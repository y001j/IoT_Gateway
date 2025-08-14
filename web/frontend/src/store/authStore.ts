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
    console.log('🔐 [BEFORE] 设置认证令牌开始', {
      accessToken: accessToken ? `${accessToken.substring(0, 20)}...` : 'null',
      refreshToken: refreshToken ? `${refreshToken.substring(0, 20)}...` : 'null',
      currentState: get()
    });
    
    // 使用函数形式更新状态，确保persist能正确捕获
    set((state) => {
      console.log('🔄 [DURING SET] 状态更新中，当前state:', state);
      const newState = {
        ...state,
        accessToken, 
        refreshToken, 
        isAuthenticated: !!accessToken,
        isInitialized: true
      };
      console.log('🔄 [DURING SET] 即将返回新状态:', newState);
      return newState;
    });
    
    // 强制触发persist保存
    setTimeout(() => {
      const currentState = get();
      console.log('💾 强制检查persist状态:', currentState);
      
      // 手动保存到localStorage作为备份
      const persistData = {
        accessToken: currentState.accessToken,
        refreshToken: currentState.refreshToken,
        user: currentState.user,
        isAuthenticated: currentState.isAuthenticated,
      };
      try {
        localStorage.setItem('auth-storage-backup', JSON.stringify(persistData));
        console.log('💾 手动备份保存成功:', persistData);
      } catch (error) {
        console.error('💾 手动备份保存失败:', error);
      }
    }, 100);
    
    // 同步更新WebSocket服务的token
    if (accessToken) {
      import('../services/websocketService').then(({ webSocketService }) => {
        webSocketService.setToken(accessToken);
        console.log('🔗 WebSocket令牌已同步更新');
      }).catch((error) => {
        console.warn('⚠️ WebSocket令牌同步失败:', error);
      });
    }
    
    // 验证状态是否正确设置
    const afterState = get();
    console.log('🔐 [AFTER] 设置认证令牌完成', {
      afterState,
      isTokenSet: !!afterState.accessToken,
      isAuthenticated: afterState.isAuthenticated,
      isInitialized: afterState.isInitialized
    });
  },
  setUser: (user) => {
    console.log('👤 设置用户信息:', user);
    set((state) => ({
      ...state,
      user
    }));
  },
  logout: () => {
    console.log('🚪 用户登出，清除认证状态');
    set((state) => ({ 
      ...state,
      accessToken: null, 
      refreshToken: null, 
      user: null, 
      isAuthenticated: false,
      isInitialized: true  // 保持初始化状态，避免无限循环
    }));
    // 清除localStorage中的认证信息
    localStorage.removeItem('auth-storage');
    localStorage.removeItem('auth-storage-backup');
  },
  initialize: async () => {
    console.log('🚀 初始化认证状态...');
    
    const { accessToken, refreshToken } = get();
    
    console.log('📋 存储的认证信息:');
    console.log('- accessToken:', accessToken ? `${accessToken.substring(0, 30)}...` : 'null');
    console.log('- refreshToken:', refreshToken ? `${refreshToken.substring(0, 30)}...` : 'null');
    
    if (!accessToken) {
      console.log('❌ 没有找到存储的访问令牌');
      set((state) => ({ ...state, isAuthenticated: false, isInitialized: true }));
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
        set((state) => ({ 
          ...state,
          isAuthenticated: true, 
          isInitialized: true,
          user: userData 
        }));
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
        set((state) => ({ 
          ...state,
          isAuthenticated: true, 
          isInitialized: true 
        }));
        return;
      }
      
      // 只有在明确的认证错误时才清除认证信息
      console.error('认证失败，清除存储的认证信息');
      set((state) => ({ 
        ...state,
        accessToken: null, 
        refreshToken: null, 
        user: null, 
        isAuthenticated: false, 
        isInitialized: true 
      }));
      localStorage.removeItem('auth-storage');
    }
  },
});


// 尝试刷新token的辅助函数
const attemptTokenRefresh = async (refreshToken: string, set: any) => {
  try {
    console.log('🔄 尝试刷新访问令牌...');
    // 后端期望字段名为 refresh_token（小写下划线）
    const response = await fetch('/api/v1/auth/refresh', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ refresh_token: refreshToken }),
    });

    if (response.ok) {
      const result = await response.json();
      const { token: newAccessToken, refresh_token: newRefreshToken, user } = result.data || result;
      
      // 如果没有新refresh token，使用access token
      const newRefresh = newRefreshToken || newAccessToken;
      
      console.log('✅ 令牌刷新成功');
      set((state) => ({ 
        ...state,
        accessToken: newAccessToken, 
        refreshToken: newRefresh,
        user: user,
        isAuthenticated: true, 
        isInitialized: true 
      }));
    } else {
      throw new Error(`刷新令牌失败: ${response.status}`);
    }
  } catch (error) {
    console.error('❌ 刷新令牌失败:', error);
    
    // 检查是否是网络错误
    if (error instanceof TypeError && error.message.includes('fetch')) {
      console.warn('🌐 刷新令牌时网络错误 - 保留现有认证状态');
      set((state) => ({ 
        ...state,
        isAuthenticated: true, 
        isInitialized: true 
      }));
      return;
    }
    
    // 只有在明确的认证错误时才清除认证信息
    console.error('刷新令牌失败，清除所有认证信息');
    set((state) => ({ 
      ...state,
      accessToken: null, 
      refreshToken: null, 
      user: null, 
      isAuthenticated: false, 
      isInitialized: true 
    }));
    localStorage.removeItem('auth-storage');
  }
};

export const useAuthStore = create<AuthState>()(
  persist(
    authStoreCreator,
    {
      name: 'auth-storage', // unique name
      storage: createJSONStorage(() => localStorage),
      // 持久化所有字段，除了isInitialized
      partialize: (state) => {
        console.log('💾 Persist partialize 调用，当前状态:', state);
        const result = {
          accessToken: state.accessToken,
          refreshToken: state.refreshToken,
          user: state.user,
          isAuthenticated: state.isAuthenticated,
        };
        console.log('💾 Persist 保存的数据:', result);
        
        // 立即手动验证存储
        setTimeout(() => {
          const stored = localStorage.getItem('auth-storage');
          console.log('💾 验证localStorage实际存储:', stored);
        }, 50);
        
        return result;
      },
      // 添加事件监听器
      onRehydrateStorage: () => {
        console.log('🏺 开始从localStorage恢复状态...');
        return (state, error) => {
          if (error) {
            console.error('🏺 从localStorage恢复状态失败:', error);
          } else {
            console.log('🏺 从localStorage恢复状态成功:', state);
            
            // 如果persist失败，尝试从备份恢复
            if (!state?.accessToken) {
              try {
                const backup = localStorage.getItem('auth-storage-backup');
                if (backup) {
                  const backupData = JSON.parse(backup);
                  console.log('🔄 尝试从备份恢复数据:', backupData);
                  if (backupData.accessToken) {
                    // 手动设置状态
                    useAuthStore.setState((prevState) => ({ ...prevState, ...backupData }));
                    console.log('✅ 从备份恢复成功');
                  }
                }
              } catch (e) {
                console.error('❌ 备份恢复失败:', e);
              }
            }
          }
        };
      },
      // 强制同步写入
      serialize: (state) => {
        const serialized = JSON.stringify(state);
        console.log('💾 Serialize 调用:', serialized);
        return serialized;
      },
      deserialize: (str) => {
        console.log('💾 Deserialize 调用:', str);
        return JSON.parse(str);
      },
    }
  )
); 