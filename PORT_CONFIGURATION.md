# IoT Gateway 端口配置说明

## 🔌 端口分配

为了避免端口冲突，IoT Gateway项目中的各个服务使用不同的端口：

### 核心服务端口

| 服务名称 | 端口 | 用途 | 配置文件 |
|---------|------|------|----------|
| **Gateway 主服务** | `8080` | IoT数据处理核心服务 | `config.yaml` → `gateway.http_port` |
| **Web API 服务** | `8081` | REST API 和管理界面后端 | `configs/web/config.yaml` → `server.addr` |
| **前端开发服务器** | `3000` | React开发服务器 (Vite) | `web/frontend/vite.config.ts` |

### 其他端口

| 服务 | 端口 | 用途 |
|------|------|------|
| NATS | `4222` | 内部消息总线 |
| MQTT | `1883` | MQTT代理 |
| Modbus | `502` | Modbus设备通信 |
| ISP Sidecar | `50052` | 插件通信协议 |

## 🔧 修改历史

### 修复端口冲突 (2025-01-XX)

**问题**: Gateway服务和Web API服务都使用8080端口，导致冲突

**解决方案**:
1. Gateway服务保持 `8080` 端口 (核心服务优先)
2. Web API服务改为 `8081` 端口
3. 前端代理配置更新为指向 `8081` 端口

**修改的文件**:
- `configs/web/config.yaml`: `server.addr` 改为 `:8081`
- `cmd/server/main.go`: 默认端口改为 `:8081`  
- `web/frontend/vite.config.ts`: 代理目标改为 `localhost:8081`

## 🚀 启动顺序建议

1. **启动Gateway服务** (端口8080):
   ```bash
   go run cmd/gateway/main.go -config config.yaml
   ```

2. **启动Web API服务** (端口8081):
   ```bash
   go run cmd/server/main.go
   ```

3. **启动前端开发服务器** (端口3000):
   ```bash
   cd web/frontend
   npm run dev
   ```

## 🌐 访问地址

- **前端管理界面**: http://localhost:3000
- **Web API文档**: http://localhost:8081/api/v1/swagger
- **Gateway健康检查**: http://localhost:8080/health

## ⚠️ 注意事项

1. **生产环境**: 建议使用反向代理(如Nginx)统一端口
2. **防火墙**: 确保相关端口在防火墙中开放
3. **配置同步**: 修改端口时需要同步更新所有相关配置文件

## 🛠️ 故障排查

### 端口占用检查
```bash
# Windows
netstat -ano | findstr :8080
netstat -ano | findstr :8081

# Linux/Mac  
lsof -i :8080
lsof -i :8081
```

### 强制终止进程
```bash
# Windows
taskkill /PID <进程ID> /F

# Linux/Mac
kill -9 <进程ID>
``` 