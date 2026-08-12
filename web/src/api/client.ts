import type { ApiError } from './types';
import { HttpStatus } from './types';

export const API_BASE_URL = '.'; // API 请求固定使用当前站点，由 Vite 开发代理转发。

/**
 * 获取认证 Store（延迟导入以避免循环依赖）
 */
let getAuthStore: (() => { token: string | null; logout: () => void }) | null = null;

export function setAuthStoreGetter(getter: () => { token: string | null; logout: () => void }) {
    getAuthStore = getter;
}

/**
 * 全局错误处理
 */
const handleError = (error: ApiError) => {
    console.error('API Error:', error);

    // 401 未授权，调用 store 的 logout
    if (error.code === HttpStatus.UNAUTHORIZED) {
        if (getAuthStore) {
            const store = getAuthStore();
            store.logout();
        }
    }
};

export interface RequestOptions {
    params?: Record<string, string | number | boolean>;
    /** 显式 Authorization 覆盖（如 API Key 认证场景） */
    auth?: string;
    /** 响应解析方式；blob 用于文件下载 */
    responseType?: 'json' | 'text' | 'blob';
    /** 原始请求体（FormData / Blob 等）；设置后不自动 JSON 序列化，也不设置 Content-Type */
    rawBody?: BodyInit;
    timeoutMs?: number;
}

/**
 * 处理响应
 */
async function handleResponse<T>(response: Response, responseType: 'json' | 'text' | 'blob'): Promise<T> {
    if (responseType === 'blob') {
        if (!response.ok) {
            const text = await response.text();
            const error: ApiError = {
                code: response.status,
                message: text || response.statusText,
            };
            handleError(error);
            throw error;
        }
        return {
            blob: await response.blob(),
            filename: parseFilename(response.headers.get('content-disposition')),
        } as T;
    }

    const contentType = response.headers.get('content-type');
    const isJson = contentType?.includes('application/json');

    let data: unknown;
    if (isJson) {
        data = await response.json();
    } else {
        data = await response.text();
    }

    if (!response.ok) {
        const error: ApiError = {
            code: response.status,
            message: (data && typeof data === 'object' && 'message' in data && typeof data.message === 'string')
                ? data.message
                : (typeof data === 'string' ? data : response.statusText),
        };

        handleError(error);
        throw error;
    }

    // 标准 ApiResponse 格式：返回 data 字段
    if (data && typeof data === 'object' && 'data' in data) {
        return data.data as T;
    }
    // resp.Success(c, nil) 只返回 {code,message} 无 data 键：解包为 null，避免类型谎言
    if (data && typeof data === 'object' && 'code' in data) {
        return null as T;
    }

    return data as T;
}

/**
 * 发送请求
 */
async function request<T>(method: string, path: string, options: RequestOptions = {}): Promise<T> {
    const { params, auth, responseType = 'json', rawBody, timeoutMs = 120_000 } = options;

    // 构建 URL
    const searchParams = params ? new URLSearchParams(
        Object.entries(params).map(([k, v]) => [k, String(v)])
    ).toString() : '';
    const url = `${API_BASE_URL}${path}${searchParams ? `?${searchParams}` : ''}`;

    // 构建请求头
    const headers = new Headers();

    // 只有 JSON body 时设置 Content-Type（FormData 由浏览器自动设置）
    if (rawBody && !(rawBody instanceof FormData)) {
        headers.set('Content-Type', 'application/json');
    }

    // 添加 Authorization - 从 zustand store 获取 token；可被显式 auth 覆盖（如 API Key 认证）
    if (auth !== undefined) {
        headers.set('Authorization', auth);
    } else if (typeof window !== 'undefined' && getAuthStore) {
        const store = getAuthStore();
        if (store.token) {
            headers.set('Authorization', `Bearer ${store.token}`);
        }
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);

    try {
        const response = await fetch(url.toString(), {
            method,
            headers,
            body: rawBody,
            signal: controller.signal,
        });
        return await handleResponse<T>(response, responseType);
    } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') {
            const error: ApiError = { code: 0, message: 'Request timeout' };
            handleError(error);
            throw error;
        }
        throw err;
    } finally {
        clearTimeout(timer);
    }
}

/**
 * 下载文件（blob 响应）
 */
export interface BlobDownload {
    blob: Blob;
    /** Content-Disposition 解析出的文件名，可能为 null */
    filename: string | null;
}

function parseFilename(contentDisposition: string | null): string | null {
    if (!contentDisposition) return null;
    const match = contentDisposition.match(/filename="([^"]+)"/i);
    return match?.[1] ?? null;
}

export const apiClient = {
    /**
     * GET 请求
     */
    get: <T>(path: string, params?: Record<string, string | number | boolean>): Promise<T> =>
        request<T>('GET', path, { params }),

    /**
     * POST 请求
     */
    post: <T>(path: string, data?: unknown, params?: Record<string, string | number | boolean>): Promise<T> =>
        request<T>('POST', path, {
            params,
            rawBody: data ? JSON.stringify(data) : undefined,
        }),

    /**
     * PUT 请求
     */
    put: <T>(path: string, data?: unknown, params?: Record<string, string | number | boolean>): Promise<T> =>
        request<T>('PUT', path, {
            params,
            rawBody: data ? JSON.stringify(data) : undefined,
        }),

    /**
     * DELETE 请求
     */
    delete: <T>(path: string, params?: Record<string, string | number | boolean>): Promise<T> =>
        request<T>('DELETE', path, { params }),

    /**
     * PATCH 请求
     */
    patch: <T>(path: string, data?: unknown, params?: Record<string, string | number | boolean>): Promise<T> =>
        request<T>('PATCH', path, {
            params,
            rawBody: data ? JSON.stringify(data) : undefined,
        }),

    /**
     * POST 表单（multipart/form-data）
     */
    postForm: <T>(path: string, form: FormData): Promise<T> =>
        request<T>('POST', path, { rawBody: form }),

    /**
     * 下载文件（blob 响应）
     */
    getBlob: (path: string, params?: Record<string, string | number | boolean>): Promise<BlobDownload> =>
        request<BlobDownload>('GET', path, { params, responseType: 'blob' }),
};
