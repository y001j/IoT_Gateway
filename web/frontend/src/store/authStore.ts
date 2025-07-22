import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';
import { StateCreator } from 'zustand';

interface User {
  id: number;
  username: string;
  role: string;
  email?: string;
  createdAt?: string;
}

interface AuthState {
  accessToken: string | null;
  refreshToken: string | null;
  user: User | null;
  isAuthenticated: boolean;
  isInitialized: boolean;
  setTokens: (accessToken: string, refreshToken: string) => void;
  setUser: (user: User | null) => void;
  logout: () => void;
  initialize: () => Promise<void>;
}

const authStoreCreator: StateCreator<AuthState, [], [['zustand/persist', AuthState]]> = (set, get) => ({
  accessToken: null,
  refreshToken: null,
  user: null,
  isAuthenticated: false,
  isInitialized: false,
  setTokens: (accessToken, refreshToken) => {
    console.log('🔐 设置认证令牌');
    set({ accessToken, refreshToken, isAuthenticated: !!accessToken });
  },
  setUser: (user) => {
    console.log('👤 设置用户信息:', user);
    set({ user });
  },
  logout: () => {
    console.log('🚪 用户登出，清除认证状态');
    set({ accessToken: null, refreshToken: null, user: null, isAuthenticated: false });
    // 清除localStorage中的认证信息
    localStorage.removeItem('auth-storage');
  },
  initialize: async () => {
    console.log('🚀 初始化认证状态...');
    
    const { accessToken, refreshToken } = get();
    
    console.log('📋 存储的认证信息:');
    console.log('- accessToken:', accessToken ? `${accessToken.substring(0, 30)}...` : 'null');
    console.log('- refreshToken:', refreshToken ? `${refreshToken.substring(0, 30)}...` : 'null');
    
    if (!accessToken) {
      console.log('❌ 没有找到存储的访问令牌');
      set({ isAuthenticated: false, isInitialized: true });
      return;
    }

    if (!refreshToken) {
      console.log('⚠️ 没有找到刷新令牌，但有访问令牌，尝试验证访问令牌');
    }

    try {
      console.log('🔍 验证token有效性...');
      console.log('🌐 API URL:', '/api/v1/auth/profile');
      
      // 验证token有效性 - 调用一个需要认证的轻量级接口
      const response = await fetch('/api/v1/auth/profile', {
        method: 'GET',
        headers: {
          'Authorization': `Bearer ${accessToken}`,
          'Content-Type': 'application/json',
        },
      });

      console.log('📡 API响应:', {
        status: response.status,
        statusText: response.statusText,
        ok: response.ok,
        url: response.url
      });

      if (response.ok) {
        const result = await response.json();
        const userData = result.data || result;
        console.log('✅ 令牌有效，用户已认证:', userData);
        set({ 
          isAuthenticated: true, 
          isInitialized: true,
          user: userData 
        });
      } else if (response.status === 401) {
        // Token过期，尝试使用refreshToken刷新
        if (refreshToken) {
          console.log('🔄 访问令牌过期，尝试刷新...');
          await attemptTokenRefresh(refreshToken, set);
        } else {
          console.log('❌ 访问令牌过期但没有刷新令牌');
          throw new Error('访问令牌过期且没有刷新令牌');
        }
      } else {
        const errorText = await response.text();
        console.error('❌ API调用失败:', {
          status: response.status,
          statusText: response.statusText,
          errorText: errorText
        });
        throw new Error(`验证失败: ${response.status} - ${errorText}`);
      }
    } catch (error) {
      console.error('❌ 认证验证异常:', error);
      
      // 检查是否是网络错误
      if (error instanceof TypeError && error.message.includes('fetch')) {
        console.warn('🌐 网络连接错误 - 后端服务可能未启动，但保留认证状态');
        console.warn('如果后端服务恢复，认证状态将自动恢复');
        
        // 网络错误时，保留认证状态但标记为未初始化，让用户可以继续使用
        set({ 
          isAuthenticated: true, 
          isInitialized: true 
        });
        return;
      }
      
      // 只有在明确的认证错误时才清除认证信息
      console.error('认证失败，清除存储的认证信息');
      set({ 
        accessToken: null, 
        refreshToken: null, 
        user: null, 
        isAuthenticated: false, 
        isInitialized: true 
      });
      localStorage.removeItem('auth-storage');
    }
  },
});


// 尝试刷新token的辅助函数
const attemptTokenRefresh = async (refreshToken: string, set: any) => {
  try {
    console.log('🔄 尝试刷新访问令牌...');
    const response = await fetch('/api/v1/auth/refresh', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ refreshToken }),
    });

    if (response.ok) {
      const result = await response.json();
      const { token: newAccessToken, refresh_token: newRefreshToken, user } = result.data || result;
      
      console.log('✅ 令牌刷新成功');
      set({ 
        accessToken: newAccessToken, 
        refreshToken: newRefreshToken,
        user: user,
        isAuthenticated: true, 
        isInitialized: true 
      });
    } else {
      throw new Error(`刷新令牌失败: ${response.status}`);
    }
  } catch (error) {
    console.error('❌ 刷新令牌失败:', error);
    
    // 检查是否是网络错误
    if (error instanceof TypeError && error.message.includes('fetch')) {
      console.warn('🌐 刷新令牌时网络错误 - 保留现有认证状态');
      set({ 
        isAuthenticated: true, 
        isInitialized: true 
      });
      return;
    }
    
    // 只有在明确的认证错误时才清除认证信息
    console.error('刷新令牌失败，清除所有认证信息');
    set({ 
      accessToken: null, 
      refreshToken: null, 
      user: null, 
      isAuthenticated: false, 
      isInitialized: true 
    });
    localStorage.removeItem('auth-storage');
  }
};

export const useAuthStore = create<AuthState>()(
  persist(
    authStoreCreator,
    {
      name: 'auth-storage', // unique name
      storage: createJSONStorage(() => localStorage),
      // 只持久化特定字段，不持久化isInitialized
      partialize: (state) => ({
        accessToken: state.accessToken,
        refreshToken: state.refreshToken,
        user: state.user,
        isAuthenticated: state.isAuthenticated,
      }),
    }
  )
); 