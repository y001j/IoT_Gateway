import axios, { AxiosError, InternalAxiosRequestConfig, AxiosResponse } from 'axios';
import { useAuthStore } from '../store/authStore';

const api = axios.create({
  baseURL: '/api/v1',
  headers: {
    'Content-Type': 'application/json',
  },
  timeout: 30000, // 30秒超时
});

// Request interceptor to add the auth token to headers
api.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    const authState = useAuthStore.getState();
    const { accessToken, isAuthenticated, isInitialized } = authState;
    
    console.log('📤 发送API请求:', config.method?.toUpperCase(), config.url);
    console.log('🔍 认证状态:', {
      isAuthenticated,
      isInitialized,
      hasAccessToken: !!accessToken,
      accessTokenPrefix: accessToken ? `${accessToken.substring(0, 20)}...` : 'null'
    });
    
    if (accessToken) {
      config.headers.Authorization = `Bearer ${accessToken}`;
      console.log('✅ 已添加Authorization header');
    } else {
      console.log('❌ 没有accessToken，未添加Authorization header');
    }
    return config;
  },
  (error: AxiosError) => {
    console.error('❌ API请求拦截器错误:', error);
    return Promise.reject(error);
  }
);

// Response interceptor to handle token refresh
let isRefreshing = false;
let failedQueue: { resolve: (value: unknown) => void; reject: (reason?: Error) => void; }[] = [];

const processQueue = (error: Error | null, token: string | null = null) => {
  failedQueue.forEach(prom => {
    if (error) {
      prom.reject(error);
    } else {
      prom.resolve(token);
    }
  });
  
  failedQueue = [];
};

api.interceptors.response.use(
  (response: AxiosResponse) => {
    console.log('✅ API响应成功:', response.config.url, response.status);
    return response;
  },
  async (error: AxiosError) => {
    console.error('❌ API请求错误:', {
      url: error.config?.url,
      status: error.response?.status,
      statusText: error.response?.statusText,
      message: error.message,
      code: error.code,
      response: error.response?.data
    });
    
    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean };

    if (!originalRequest) {
      console.error('❌ 缺少原始请求配置');
      return Promise.reject(error);
    }

    // 网络错误处理
    if (error.code === 'ECONNREFUSED' || error.message.includes('Network Error')) {
      console.warn('🌐 网络连接错误，后端服务可能未启动');
      const networkError = new Error('网络连接失败，请检查后端服务是否运行');
      networkError.name = 'NetworkError';
      return Promise.reject(networkError);
    }

    // 400 Bad Request 特殊处理
    if (error.response?.status === 400) {
      const errorData = error.response.data as any;
      if (errorData?.message && errorData.message.includes('RefreshToken')) {
        console.warn('⚠️ Refresh token 验证失败，需要重新登录');
        const { logout } = useAuthStore.getState();
        logout();
        const authError = new Error('认证已过期，请重新登录');
        authError.name = 'AuthExpiredError';
        return Promise.reject(authError);
      }
    }

    if (error.response?.status === 401 && !originalRequest._retry) {
      if (isRefreshing) {
        return new Promise(function(resolve, reject) {
          failedQueue.push({ resolve, reject });
        }).then(token => {
          originalRequest.headers['Authorization'] = 'Bearer ' + token;
          return axios(originalRequest);
        }).catch(err => {
          return Promise.reject(err);
        })
      }

      originalRequest._retry = true;
      isRefreshing = true;

      const { refreshToken, logout, setTokens } = useAuthStore.getState();
      if (!refreshToken) {
        logout();
        isRefreshing = false;
        // Optionally redirect to login page
        // window.location.href = '/login';
        return Promise.reject(error);
      }

      try {
        // 后端期望字段名为 refresh_token（小写下划线）
        const { data } = await axios.post('/api/v1/auth/refresh', { refresh_token: refreshToken });
        const { token: newAccessToken, refresh_token: newRefreshToken } = data.data || data;
        
        // 如果没有新refresh token，使用access token
        const newRefresh = newRefreshToken || newAccessToken;
        
        setTokens(newAccessToken, newRefresh);
        if (api.defaults.headers.common)
            api.defaults.headers.common['Authorization'] = 'Bearer ' + newAccessToken;
        originalRequest.headers['Authorization'] = 'Bearer ' + newAccessToken;
        
        processQueue(null, newAccessToken);
        return api(originalRequest);
      } catch (refreshError) {
        console.error('❌ Token 刷新失败:', refreshError);
        const error = refreshError instanceof Error ? refreshError : new Error(String(refreshError));
        processQueue(error, null);
        logout();
        
        // 创建更友好的错误消息
        const authError = new Error('会话已过期，请重新登录');
        authError.name = 'AuthExpiredError';
        return Promise.reject(authError);
      } finally {
        isRefreshing = false;
      }
    }

    // 其他HTTP错误的友好处理
    if (error.response?.status === 403) {
      const permissionError = new Error('权限不足，无法执行此操作');
      permissionError.name = 'PermissionError';
      return Promise.reject(permissionError);
    }

    if (error.response?.status === 404) {
      const notFoundError = new Error('请求的资源不存在');
      notFoundError.name = 'NotFoundError';
      return Promise.reject(notFoundError);
    }

    if (error.response?.status >= 500) {
      const serverError = new Error('服务器内部错误，请稍后重试');
      serverError.name = 'ServerError';
      return Promise.reject(serverError);
    }

    return Promise.reject(error);
  }
);

export default api; 