// import { message } from 'antd';

export interface WebSocketMessage {
  type: string;
  data: Record<string, unknown>;
  timestamp?: number;
}

export interface WebSocketCallbacks {
  onMessage?: (message: WebSocketMessage) => void;
  onConnect?: () => void;
  onDisconnect?: () => void;
  onError?: (error: Event) => void;
}

class WebSocketService {
  private ws: WebSocket | null = null;
  private url: string = '';
  private token: string = '';
  private callbacks: WebSocketCallbacks = {};
  private reconnectInterval: number = 15000; // 进一步增加重连间隔到15秒
  private maxReconnectAttempts: number = 3;   // 进一步减少最大重连次数
  private reconnectAttempts: number = 0;
  private isConnecting: boolean = false;
  private isManuallyDisconnected: boolean = false;
  private lastDisconnectReason: string = '';
  // 新增连接控制字段
  private connectionId: string = '';
  private lastConnectAttempt: number = 0;
  private minConnectInterval: number = 5000; // 最小连接间隔5秒
  private reconnectTimer: NodeJS.Timeout | null = null;

  constructor() {
    // 优化的URL配置，支持环境变量
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const isDev = import.meta.env.DEV;
    
    if (isDev) {
      // 开发环境：优先使用环境变量，否则使用默认配置
      const wsBaseUrl = import.meta.env.VITE_WS_BASE_URL;
      if (wsBaseUrl) {
        this.url = `${wsBaseUrl}/realtime`;
      } else {
        this.url = `${protocol}//localhost:8081/api/v1/ws/realtime`;
      }
    } else {
      // 生产环境使用当前域名
      const host = window.location.host;
      this.url = `${protocol}//${host}/api/v1/ws/realtime`;
    }
    
    this.connectionId = this.generateConnectionId();
    console.log('🔧 WebSocket服务初始化', {
      url: this.url,
      connectionId: this.connectionId,
      isDev,
      wsBaseUrl: import.meta.env.VITE_WS_BASE_URL,
      debugWs: import.meta.env.VITE_DEBUG_WS
    });
  }

  /**
   * 生成唯一连接ID
   */
  private generateConnectionId(): string {
    return `ws_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  /**
   * 设置认证令牌
   */
  setToken(token: string) {
    console.log('🔑 设置WebSocket认证令牌，长度:', token ? token.length : 0);
    this.token = token;
  }

  /**
   * 设置回调函数
   */
  setCallbacks(callbacks: WebSocketCallbacks) {
    this.callbacks = { ...this.callbacks, ...callbacks };
  }

  /**
   * 测试WebSocket端点是否可用
   */
  async testWebSocketEndpoint(): Promise<boolean> {
    try {
      // 尝试访问WebSocket端点的HTTP版本（如果有的话）
      const httpUrl = this.url.replace('ws:', 'http:').replace('wss:', 'https:');
      console.log('🧪 测试WebSocket端点可达性:', httpUrl);
      
      const response = await fetch(httpUrl.split('?')[0], { 
        method: 'GET',
        headers: { 'Connection': 'upgrade' }
      });
      
      console.log('🧪 WebSocket端点测试结果:', response.status, response.statusText);
      return true; // 即使返回错误状态，至少端点是可达的
    } catch (error) {
      console.error('🧪 WebSocket端点不可达:', error);
      return false;
    }
  }

  /**
   * 连接到 WebSocket 服务器
   */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      // 执行器本身不能是 async：async 执行器中抛出的异常不会使 Promise 失败，调用方会一直等待
      const run = async () => {
      // 严格的连接控制
      const now = Date.now();
      
      if (this.isConnecting) {
        console.log('⏸️ WebSocket连接正在进行中，跳过重复连接请求');
        resolve();
        return;
      }
      
      if (this.isConnected()) {
        console.log('✅ WebSocket已连接，跳过连接请求');
        resolve();
        return;
      }

      // 检查连接间隔限制
      if (now - this.lastConnectAttempt < this.minConnectInterval) {
        const waitTime = this.minConnectInterval - (now - this.lastConnectAttempt);
        console.log(`⏰ 连接间隔限制，需等待 ${waitTime}ms`);
        setTimeout(() => {
          this.connect().then(resolve).catch(reject);
        }, waitTime);
        return;
      }

      if (!this.token) {
        const error = new Error('认证令牌未设置');
        console.error('❌ WebSocket连接失败:', error.message);
        reject(error);
        return;
      }

      // 清除之前的重连定时器
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer);
        this.reconnectTimer = null;
      }

      this.isConnecting = true;
      this.isManuallyDisconnected = false;
      this.lastConnectAttempt = now;
      
      console.log(`🔌 开始建立WebSocket连接... [${this.connectionId}]`);
      
      // 先测试端点是否可达
      const isEndpointReachable = await this.testWebSocketEndpoint();
      if (!isEndpointReachable) {
        const error = new Error(`WebSocket端点不可达: ${this.url}`);
        console.error('❌ WebSocket端点测试失败');
        this.isConnecting = false;
        reject(error);
        return;
      }

      try {
        // 在 URL 中传递 JWT 令牌（通过查询参数）
        const urlWithToken = `${this.url}?token=${encodeURIComponent(this.token)}`;
        console.log('🌐 连接WebSocket URL:', urlWithToken.replace(/token=[^&]+/, 'token=***'));
        
        this.ws = new WebSocket(urlWithToken);

        this.ws.onopen = () => {
          console.log(`✅ WebSocket连接已建立 [${this.connectionId}]`);
          this.isConnecting = false;
          this.reconnectAttempts = 0;
          this.lastDisconnectReason = '';
          this.callbacks.onConnect?.();
          resolve();
        };

        this.ws.onmessage = (event) => {
          try {
            const message: WebSocketMessage = JSON.parse(event.data);
            console.log(`📨 收到WebSocket消息: ${message.type} [${this.connectionId}]`);
            this.handleMessage(message);
          } catch (error) {
            console.error(`❌ 解析WebSocket消息失败 [${this.connectionId}]:`, error, '原始数据:', event.data);
          }
        };

        this.ws.onclose = (event) => {
          this.lastDisconnectReason = `Code: ${event.code}, Reason: ${event.reason || '未知'}`;
          console.log(`🔌 WebSocket连接已关闭 [${this.connectionId}]:`, this.lastDisconnectReason);
          this.isConnecting = false;
          this.ws = null;
          this.callbacks.onDisconnect?.();

          // 如果不是手动断开，且没有达到最大重连次数，尝试重连
          if (!this.isManuallyDisconnected && this.reconnectAttempts < this.maxReconnectAttempts) {
            const nextReconnectInterval = this.reconnectInterval * Math.pow(2, this.reconnectAttempts); // 指数退避
            console.log(`⏰ 将在${nextReconnectInterval/1000}秒后尝试重连 (${this.reconnectAttempts + 1}/${this.maxReconnectAttempts}) [${this.connectionId}]`);
            
            this.reconnectTimer = setTimeout(() => {
              if (!this.isManuallyDisconnected && !this.isConnected()) {
                this.reconnectAttempts++;
                console.log(`🔄 尝试重连WebSocket (${this.reconnectAttempts}/${this.maxReconnectAttempts}) [${this.connectionId}]`);
                this.connect().catch(error => {
                  console.error(`❌ 重连失败 [${this.connectionId}]:`, error);
                });
              }
            }, nextReconnectInterval);
          } else if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error(`❌ 已达到最大重连次数，停止重连 [${this.connectionId}]`);
          }
        };

        this.ws.onerror = (error) => {
          console.error(`❌ WebSocket连接错误 [${this.connectionId}]:`, {
            error,
            url: this.url,
            token: this.token ? `${this.token.substring(0, 20)}...` : 'null',
            readyState: this.ws?.readyState,
            timestamp: new Date().toISOString()
          });
          this.isConnecting = false;
          this.callbacks.onError?.(error);
          reject(new Error(`WebSocket连接失败: 无法连接到 ${this.url}`));
        };

      } catch (error) {
        console.error(`❌ 创建WebSocket连接失败 [${this.connectionId}]:`, error);
        this.isConnecting = false;
        reject(error);
      }
    };
      run().catch(reject);
    });
  }

  /**
   * 断开 WebSocket 连接
   */
  disconnect() {
    console.log(`🔌 手动断开WebSocket连接 [${this.connectionId}]`);
    this.isManuallyDisconnected = true;
    
    // 清除重连定时器
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    
    if (this.ws) {
      this.ws.close(1000, '用户主动断开');
      this.ws = null;
    }
    this.isConnecting = false;
  }

  /**
   * 重置重连计数器
   */
  resetReconnectAttempts() {
    console.log(`🔄 重置重连计数器 [${this.connectionId}]`);
    this.reconnectAttempts = 0;
  }

  /**
   * 检查是否已连接
   */
  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  /**
   * 发送消息
   */
  send(type: string, data: Record<string, unknown>) {
    if (!this.isConnected()) {
      console.warn(`⚠️ WebSocket未连接，无法发送消息 [${this.connectionId}]:`, type);
      return;
    }

    const message = {
      type,
      data,
      timestamp: Date.now(),
    };

    try {
      this.ws!.send(JSON.stringify(message));
      console.log(`📤 发送WebSocket消息: ${type} [${this.connectionId}]`);
    } catch (error) {
      console.error(`❌ 发送WebSocket消息失败 [${this.connectionId}]:`, error);
    }
  }

  /**
   * 发送心跳
   */
  ping() {
    this.send('ping', {});
  }

  /**
   * 订阅主题
   */
  subscribe(topics: string[]) {
    this.send('subscribe', { topics });
  }

  /**
   * 取消订阅主题
   */
  unsubscribe(topics: string[]) {
    this.send('unsubscribe', { topics });
  }

  /**
   * 处理接收到的消息
   */
  private handleMessage(message: WebSocketMessage) {
    // 处理心跳响应
    if (message.type === 'pong') {
      console.log(`💓 收到心跳响应 [${this.connectionId}]`);
      return;
    }

    // 处理欢迎消息
    if (message.type === 'welcome') {
      console.log(`👋 收到欢迎消息 [${this.connectionId}]:`, message.data);
      return;
    }

    // 处理初始数据
    if (message.type === 'initial_data') {
      console.log(`📊 收到初始数据 [${this.connectionId}]`);
    }

    // 调用用户回调
    if (this.callbacks.onMessage) {
      this.callbacks.onMessage(message);
    }
  }

  /**
   * 获取连接状态
   */
  getConnectionState(): string {
    if (this.isConnecting) return 'CONNECTING';
    if (this.isConnected()) return 'CONNECTED';
    if (this.isManuallyDisconnected) return 'DISCONNECTED';
    return 'DISCONNECTED';
  }

  /**
   * 获取连接信息
   */
  getConnectionInfo(): { id: string; state: string; attempts: number } {
    return {
      id: this.connectionId,
      state: this.getConnectionState(),
      attempts: this.reconnectAttempts,
    };
  }

  /**
   * 获取上次断开原因
   */
  getLastDisconnectReason(): string {
    return this.lastDisconnectReason;
  }

  /**
   * 获取重连信息
   */
  getReconnectInfo(): { attempts: number; maxAttempts: number; interval: number } {
    return {
      attempts: this.reconnectAttempts,
      maxAttempts: this.maxReconnectAttempts,
      interval: this.reconnectInterval,
    };
  }
}

// 导出类和单例实例
export { WebSocketService };
export const webSocketService = new WebSocketService();