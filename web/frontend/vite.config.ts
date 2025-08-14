import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': new URL('./src', import.meta.url).pathname,
    },
  },
  optimizeDeps: {
    include: ['react', 'react-dom', 'antd', 'echarts', 'monaco-editor'],
  },
  server: {
    port: 3000,
    host: true,
    proxy: {
      '/api': {
        target: process.env.WEB_API_URL || 'http://localhost:8081',  // Web API服务端口
        changeOrigin: true,
        secure: false,
        configure: (proxy, options) => {
          proxy.on('error', (err, req, res) => {
            console.log('API代理错误:', err);
            console.log('尝试连接到:', process.env.WEB_API_URL || 'http://localhost:8081');
          });
          proxy.on('proxyReq', (proxyReq, req, res) => {
            console.log('API代理请求:', req.method, req.url, '-> ' + (process.env.WEB_API_URL || 'http://localhost:8081') + req.url);
          });
          proxy.on('proxyRes', (proxyRes, req, res) => {
            console.log('API代理响应:', proxyRes.statusCode, req.url);
          });
        },
        // 移除 WebSocket 支持，防止代理层疯狂重连
        // ws: true,  
      },
      '/health': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        secure: false,
      },
      '/metrics': {
        target: process.env.GATEWAY_URL || 'http://localhost:8080',  // 根据配置文件更新：网关主服务的端口
        changeOrigin: true,
        secure: false,
        configure: (proxy, options) => {
          proxy.on('error', (err, req, res) => {
            console.log('Metrics代理错误:', err);
            console.log('尝试连接到:', process.env.GATEWAY_URL || 'http://localhost:8080');
          });
          proxy.on('proxyReq', (proxyReq, req, res) => {
            console.log('Metrics代理请求:', req.method, req.url, '-> ' + (process.env.GATEWAY_URL || 'http://localhost:8080') + req.url);
          });
        },
      },
      // 完全移除 WebSocket 代理配置，让前端直接连接
      // '/ws': {
      //   target: 'ws://localhost:8081',    // WebSocket服务的端口
      //   ws: true,
      //   changeOrigin: true,
      // },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom'],
          antd: ['antd'],
          charts: ['echarts'],
        },
      },
    },
    // 增加 chunk 大小警告限制
    chunkSizeWarningLimit: 1000,
  },
  css: {
    modules: {
      localsConvention: 'camelCase',
    },
    preprocessorOptions: {
      less: {
        javascriptEnabled: true,
      },
    },
  },
}) 