# 网络连接故障排除指南

## 问题描述
当前端运行在Windows主机上，而后端服务运行在WSL中时，可能会遇到网络连接问题。

## 症状
- 前端显示"Failed to fetch"错误
- 轻量级指标数据无法加载
- 连接器/适配器metrics显示不正确

## 解决方案

### 1. 检查服务状态
确保后端服务正在运行：
```bash
# 在WSL中执行
netstat -tlnp | grep :8080  # 检查网关服务
netstat -tlnp | grep :8081  # 检查Web API服务
```

### 2. 测试网络连接
运行网络测试脚本：
```bash
# 在前端目录中执行
node test-network.js
```

### 3. 修改网络配置
如果localhost连接失败，编辑 `.env` 文件：

```env
# 使用WSL的IP地址（替换为实际IP）
GATEWAY_URL=http://192.168.2.71:8080
WEB_API_URL=http://192.168.2.71:8081
```

获取WSL IP地址：
```bash
# 在WSL中执行
hostname -I
```

### 4. 确保服务绑定配置
确保WSL中的服务绑定到所有接口：
- 网关服务应该监听 `0.0.0.0:8080` 而不是 `127.0.0.1:8080`
- Web API服务应该监听 `0.0.0.0:8081` 而不是 `127.0.0.1:8081`

### 5. Windows防火墙
如果使用WSL IP地址仍然无法连接，检查Windows防火墙设置：
- 允许Node.js和相关应用程序通过防火墙
- 确保WSL2网络适配器没有被阻止

### 6. 端口转发（高级）
如果其他方法都失败，可以设置端口转发：
```cmd
# 在Windows命令提示符中执行（管理员权限）
netsh interface portproxy add v4tov4 listenport=8080 listenaddress=0.0.0.0 connectport=8080 connectaddress=WSL_IP_ADDRESS
```

## 验证修复
重启前端开发服务器后，检查：
1. 浏览器控制台是否显示正确的metrics数据
2. 监控页面是否显示连接器数据
3. 网络请求是否成功

## 调试信息
前端会在控制台输出详细的网络请求信息，包括：
- 请求的URL
- 响应状态
- 接收到的数据结构

查看浏览器开发者工具的Console和Network标签页获取更多信息。