export interface LoginCredentials {
  username?: string;
  password?: string;
}

export interface ChangePasswordPayload {
  oldPassword?: string;
  newPassword?: string;
}

export interface UpdateProfilePayload {
  email?: string;
  // Add other updatable profile fields here
}

export interface User {
  id: number;
  username: string;
  role: 'admin' | 'user';
  email?: string;
  createdAt: string;
}

export interface AuthResponse {
  accessToken: string;
  refreshToken: string;
  user: User;
} 