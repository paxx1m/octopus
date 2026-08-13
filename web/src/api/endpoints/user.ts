import { useEffect } from 'react';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { apiClient, setAuthStoreGetter } from '../client';
import { logger } from '@/lib/logger';
import { makeMutation } from '../mutation-helpers';

/**
 * 用户登录请求
 */
export interface UserLoginRequest {
    username: string;
    password: string;
    expire: number; // token 过期时间（秒）
}

/**
 * 用户登录响应
 */
export interface UserLoginResponse {
    token: string;
    expire_at: string; // ISO 8601 格式
    must_change_password?: boolean;
}

/**
 * 用户状态响应
 */
export interface UserStatusResponse {
    username: string;
    must_change_password: boolean;
}

/**
 * 修改密码请求
 */
export interface ChangePasswordRequest {
    old_password: string;
    new_password: string;
}

/**
 * 修改用户名请求
 */
export interface ChangeUsernameRequest {
    new_username: string;
}

/**
 * 认证状态 Store
 */
interface AuthState {
    isAuthenticated: boolean;
    isLoading: boolean;
    isAPIKeyAuth: boolean;
    mustChangePassword: boolean;
    token: string | null;
    expireAt: string | null;

    // Actions
    setAuth: (token: string, expireAt: string, mustChangePassword?: boolean) => void;
    setMustChangePassword: (v: boolean) => void;
    setAPIKeyAuth: (apiKey: string) => void;
    checkAuth: () => Promise<void>;
    logout: () => void;
}

/**
 * 认证状态管理 Store（使用 zustand + persist）
 */
export const useAuthStore = create<AuthState>()(
    persist(
        (set, get) => ({
            isAuthenticated: false,
            isLoading: true,
            isAPIKeyAuth: false,
            mustChangePassword: false,
            token: null,
            expireAt: null,

            setAuth: (token: string, expireAt: string, mustChangePassword = false) => {
                set({
                    isAuthenticated: true,
                    isAPIKeyAuth: false,
                    mustChangePassword,
                    token,
                    expireAt,
                    isLoading: false
                });
            },

            setMustChangePassword: (v: boolean) => {
                set({ mustChangePassword: v });
            },

            setAPIKeyAuth: (apiKey: string) => {
                set({
                    isAuthenticated: true,
                    isAPIKeyAuth: true,
                    mustChangePassword: false,
                    token: apiKey,
                    expireAt: null,
                    isLoading: false
                });
            },

            checkAuth: async () => {
                const { token, expireAt, isAPIKeyAuth } = get();

                if (!token) {
                    set({ isAuthenticated: false, isLoading: false });
                    return;
                }

                // API Key 不检查本地过期时间
                if (!isAPIKeyAuth) {
                    if (!expireAt || Date.now() >= new Date(expireAt).getTime()) {
                        get().logout();
                        return;
                    }
                }

                try {
                    if (isAPIKeyAuth) {
                        await apiClient.get<unknown>('/api/v1/apikey/login');
                        set({ isAuthenticated: true, isLoading: false, mustChangePassword: false });
                    } else {
                        const status = await apiClient.get<UserStatusResponse>('/api/v1/user/status');
                        set({
                            isAuthenticated: true,
                            isLoading: false,
                            mustChangePassword: !!status.must_change_password,
                        });
                    }
                } catch (error) {
                    logger.error('认证验证失败:', error);
                    get().logout();
                }
            },

            logout: () => {
                set({
                    isAuthenticated: false,
                    isAPIKeyAuth: false,
                    mustChangePassword: false,
                    token: null,
                    expireAt: null,
                    isLoading: false
                });
            }
        }),
        {
            name: 'auth-storage',
            partialize: (state) => ({
                token: state.token,
                expireAt: state.expireAt,
                isAPIKeyAuth: state.isAPIKeyAuth,
            })
        }
    )
);

// 注册 auth store getter 到 apiClient
if (typeof window !== 'undefined') {
    setAuthStoreGetter(() => {
        const state = useAuthStore.getState();
        return {
            token: state.token,
            logout: state.logout
        };
    });
}

/**
 * 用户登录 Hook
 * 
 * @example
 * const login = useLogin();
 * login.mutate({ username: 'admin', password: '123456', expire: 86400 });
 * 
 * if (login.isPending) return <Loading />;
 * if (login.isError) return <Error message={login.error.message} />;
 */
export function useLogin() {
    const { setAuth } = useAuthStore();

    return makeMutation<UserLoginRequest, UserLoginResponse>({
        name: '登录',
        mutationFn: (data) => apiClient.post<UserLoginResponse>('/api/v1/user/login', data),
        onSuccess: (data) => {
            setAuth(data.token, data.expire_at, !!data.must_change_password);
        },
    });
}

/**
 * 修改密码 Hook
 * 
 * @example
 * const changePassword = useChangePassword();
 * changePassword.mutate({ oldPassword: '123', newPassword: '456' });
 */
export function useChangePassword() {
    const { setMustChangePassword } = useAuthStore();
    return makeMutation<{ oldPassword: string; newPassword: string }, string>({
        name: '密码修改',
        mutationFn: async (data) => {
            const payload: ChangePasswordRequest = {
                old_password: data.oldPassword,
                new_password: data.newPassword,
            };
            return apiClient.post<string>('/api/v1/user/change-password', payload);
        },
        onSuccess: () => {
            setMustChangePassword(false);
        },
    });
}

/**
 * 修改用户名 Hook
 * 
 * @example
 * const changeUsername = useChangeUsername();
 * changeUsername.mutate({ newUsername: 'newname' });
 */
export function useChangeUsername() {
    return makeMutation<{ newUsername: string }, string>({
        name: '用户名修改',
        mutationFn: async (data) => {
            const payload: ChangeUsernameRequest = {
                new_username: data.newUsername,
            };
            return apiClient.post<string>('/api/v1/user/change-username', payload);
        },
    });
}

/**
 * 认证状态和方法 Hook
 * 
 * @example
 * const auth = useAuth();
 * 
 * if (auth.isAuthenticated) {
 *   // 已登录
 * }
 * 
 * auth.logout(); // 登出
 */
export function useAuth() {
    const store = useAuthStore();
    const { checkAuth, isLoading } = store;

    // 只在首次挂载时检查认证状态
    useEffect(() => {
        if (isLoading) {
            checkAuth();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []); // 有意只在挂载时执行一次

    return {
        isAuthenticated: store.isAuthenticated,
        isAPIKeyAuth: store.isAPIKeyAuth,
        mustChangePassword: store.mustChangePassword,
        isLoading: store.isLoading,
        logout: store.logout,
    };
}

