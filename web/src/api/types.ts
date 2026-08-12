/**
 * API 响应基础类型
 */
export interface ApiResponse<T = unknown> {
    code?: number;
    message?: string;
    data?: T;
}

/**
 * API 错误响应
 */
export interface ApiError {
    code: number;
    message: string;
}

/**
 * HTTP 状态码常量
 */
export const HttpStatus = {
    OK: 200,
    CREATED: 201,
    NO_CONTENT: 204,
    BAD_REQUEST: 400,
    UNAUTHORIZED: 401,
    FORBIDDEN: 403,
    NOT_FOUND: 404,
    CONFLICT: 409,
    INTERNAL_SERVER_ERROR: 500,
} as const;

export type HttpStatusCode = typeof HttpStatus[keyof typeof HttpStatus];
