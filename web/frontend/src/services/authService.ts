import api from './api';
import { useAuthStore } from '../store/authStore';
import { LoginCredentials, ChangePasswordPayload, UpdateProfilePayload } from '../types/auth';

export const authService = {
  async login(credentials: LoginCredentials) {
    const { setTokens, setUser } = useAuthStore.getState();
    const response = await api.post('/auth/login', credentials);
    const { token, refresh_token, user } = response.data.data;
    setTokens(token, refresh_token);
    setUser(user);
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
    const response = await api.post('/auth/refresh', { refreshToken });
    const { token: newAccessToken, refresh_token: newRefreshToken } = response.data.data || response.data;
    setTokens(newAccessToken, newRefreshToken);
    return newAccessToken;
  },

  getToken() {
    const { accessToken } = useAuthStore.getState();
    return accessToken;
  },
}; 