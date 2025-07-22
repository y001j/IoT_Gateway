// 全局类型定义

declare module '*.less' {
  const content: Record<string, string>;
  export default content;
}

declare module '*.css' {
  const content: Record<string, string>;
  export default content;
}

// 扩展Window接口
declare global {
  interface Window {
    // WebSocket相关
    __WS_RETRY_COUNT__?: number;
    
    // 调试相关
    __IOT_GATEWAY_DEBUG__?: boolean;
  }
}

// 导出空对象以使这个文件成为模块
export {}; 