import api from './api';
import { useAuthStore } from '../store/authStore';
import { LoginCredentials, ChangePasswordPayload, UpdateProfilePayload } from '../types/auth';

export const authService = {
  async login(credentials: LoginCredentials) {
    console.log('🚀 AuthService.login 开始', credentials.username);
    
    const { setTokens, setUser } = useAuthStore.getState();
    console.log('📋 当前认证状态:', useAuthStore.getState());
    
    const response = await api.post('/auth/login', credentials);
    
    console.log('🔍 登录API完整响应:', response);
    console.log('🔍 响应数据结构:', response.data);
    
    const { token, refresh_token, user } = response.data.data;
    
    // 后端可能不返回refresh_token，使用token作为refresh_token
    const refreshToken = refresh_token || token;
    
    console.log('🔐 提取的认证信息详细:', {
      token: token ? `${token.substring(0, 30)}...` : 'MISSING',
      refresh_token_original: refresh_token,
      refresh_token_used: refreshToken ? `${refreshToken.substring(0, 30)}...` : 'MISSING',
      user: user,
      hasToken: !!token,
      hasRefreshToken: !!refreshToken,
      hasUser: !!user
    });
    
    console.log('📞 即将调用 setTokens...');
    setTokens(token, refreshToken);
    
    console.log('📞 即将调用 setUser...');
    setUser(user);
    
    console.log('📋 设置完成后的认证状态:', useAuthStore.getState());
    
    return response.data.data;
  },

  async logout() {
    const { logout } = useAuthStore.getState();
    try {
      // The backend might not have a persistent session to invalidate,
      // but it's good practice to call a logout endpoint if it exists.
      await api.post('/auth/logout');
    } catch (error) {
      console.error('Logout failed:', error);
      // Even if the backend call fails, we clear the frontend state.
    } finally {
      logout();
    }
  },

  async getProfile() {
    const { setUser } = useAuthStore.getState();
    const response = await api.get('/auth/profile');
    setUser(response.data.data || response.data);
    return response.data.data || response.data;
  },

  async updateProfile(payload: UpdateProfilePayload) {
    const { setUser } = useAuthStore.getState();
    const response = await api.put('/auth/profile', payload);
    setUser(response.data.data || response.data);
    return response.data.data || response.data;
  },

  async changePassword(payload: ChangePasswordPayload) {
    return await api.put('/auth/password', payload);
  },

  // This might not be needed if the interceptor handles it fully,
  // but can be useful for manual refresh logic.
  async refreshToken() {
    const { refreshToken, setTokens } = useAuthStore.getState();
    if (!refreshToken) {
      throw new Error('No refresh token available.');
    }
    // 后端期望字段名为 refresh_token（小写下划线）
    const response = await api.post('/auth/refresh', { refresh_token: refreshToken });
    const { token: newAccessToken, refresh_token: newRefreshToken } = response.data.data || response.data;
    const newRefresh = newRefreshToken || newAccessToken; // 如果没有新refresh token，使用access token
    setTokens(newAccessToken, newRefresh);
    return newAccessToken;
  },

  getToken() {
    const { accessToken } = useAuthStore.getState();
    return accessToken;
  },
}; 