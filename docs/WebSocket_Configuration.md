# WebSocket 配置说明

## 概述

IoT Gateway的WebSocket功能支持实时数据推送到前端页面。为了防止高频数据压垮客户端和服务器，系统实现了可配置的速率限制和连接管理机制。

## 配置参数

### 基本配置

在主配置文件的 `web_ui.websocket` 部分可以配置以下参数：

```yaml
web_ui:
  websocket:
    max_clients: 50        # 最大客户端连接数
    message_rate: 20       # 消息发送速率限制 (每秒)
    read_buffer_size: 4096 # 读取缓冲区大小 (bytes)
    write_buffer_size: 4096 # 写入缓冲区大小 (bytes)
    cleanup_interval: 30   # 清理间隔 (秒)
    inactivity_timeout: 300 # 不活跃超时 (秒)
    ping_interval: 54      # Ping间隔 (秒)
    pong_timeout: 60       # Pong超时 (秒)
```

### 参数说明

#### `max_clients` (默认: 50)
- **功能**: 限制同时连接的WebSocket客户端数量
- **影响**: 超过此数量的新连接将被拒绝
- **建议**: 根据服务器性能和预期用户数量调整

#### `message_rate` (默认: 10)
- **功能**: 每个客户端每秒最多接收的消息数量
- **影响**: 超过此限制的消息将被丢弃，并记录警告日志
- **建议**: 
  - 高频数据场景(如振动传感器): 20-50
  - 普通监控场景: 10-20
  - 低频监控场景: 5-10

#### `read_buffer_size` / `write_buffer_size` (默认: 2048)
- **功能**: WebSocket连接的读写缓冲区大小
- **影响**: 影响单个消息的最大大小和传输效率
- **建议**: 根据消息大小调整，大消息需要更大缓冲区

#### `cleanup_interval` (默认: 30)
- **功能**: 清理不活跃连接的间隔时间
- **影响**: 影响资源回收的及时性
- **建议**: 高频场景可适当降低，低频场景可适当增加

#### `inactivity_timeout` (默认: 300)
- **功能**: 客户端不活跃超时时间
- **影响**: 超过此时间无活动的连接将被关闭
- **建议**: 根据网络环境稳定性调整

## 常见场景配置

### 高频数据监控
适用于振动传感器、高精度测量等场景：

```yaml
websocket:
  message_rate: 50          # 允许高频消息
  max_clients: 20           # 限制连接数防止过载
  cleanup_interval: 30      # 频繁清理维护性能
  read_buffer_size: 8192    # 更大的缓冲区
  write_buffer_size: 8192
```

### 普通监控
适用于温度、湿度、压力等常规传感器：

```yaml
websocket:
  message_rate: 15          # 适中的消息频率
  max_clients: 50           # 标准连接数
  inactivity_timeout: 300   # 标准超时时间
```

### 低频监控
适用于状态检查、报警等低频场景：

```yaml
websocket:
  message_rate: 5           # 低消息频率
  max_clients: 100          # 可支持更多连接
  inactivity_timeout: 900   # 更长的超时时间
  cleanup_interval: 60      # 较长的清理间隔
```

## 故障排查

### 常见警告和解决方案

#### "客户端消息频率过高，跳过发送"
- **原因**: 客户端接收消息的频率超过了 `message_rate` 限制
- **解决**: 增加 `message_rate` 值或优化数据推送频率

#### "达到最大连接数限制，拒绝新连接"
- **原因**: 连接数达到 `max_clients` 限制
- **解决**: 增加 `max_clients` 值或检查是否有僵尸连接

#### WebSocket连接频繁断开
- **原因**: 可能是 `inactivity_timeout` 设置过短或网络不稳定
- **解决**: 增加超时时间或优化网络环境

### 性能优化建议

1. **根据数据频率调整 `message_rate`**
   - 监控日志中的速率限制警告
   - 根据实际数据频率合理设置

2. **合理设置连接数限制**
   - 考虑服务器性能和内存使用
   - 监控实际连接数和资源占用

3. **优化缓冲区大小**
   - 根据消息大小调整缓冲区
   - 避免缓冲区过大导致内存浪费

4. **定期监控连接状态**
   - 检查不活跃连接清理情况
   - 监控连接创建和销毁日志

## 配置更新

配置更新后需要重启Web服务才能生效：

```bash
# 停止当前服务
pkill -f "bin/server"

# 重新启动服务
./bin/server -config config.yaml
```

## 监控指标

系统提供以下监控指标：

- 当前连接数
- 消息发送频率
- 速率限制触发次数
- 连接创建/销毁统计
- 缓冲区使用情况

这些指标可以通过日志和WebSocket状态API获取。