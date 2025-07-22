# ISP协议快速参考

## 基本信息
- **协议名称**: IoT Sidecar Protocol (ISP)
- **传输层**: TCP
- **默认端口**: 50052
- **消息格式**: Line-delimited JSON
- **字符编码**: UTF-8

## 消息类型速览

| 类型 | 方向 | 用途 | 是否需要响应 |
|------|------|------|-------------|
| CONFIG | Gateway → Sidecar | 发送配置 | 是 |
| DATA | Sidecar → Gateway | 发送数据 | 否 |
| STATUS | Gateway → Sidecar | 查询状态 | 是 |
| HEARTBEAT | 双向 | 保持连接 | 否 |
| RESPONSE | 双向 | 回复请求 | 否 |

## 消息结构模板

### CONFIG消息
```json
{
  "type": "CONFIG",
  "id": "unique-id",
  "timestamp": 1751200000,
  "payload": {
    "device_id": "设备ID",
    "host": "Modbus主机",
    "port": 502,
    "slave_id": 1,
    "registers": [...]
  }
}
```

### DATA消息
```json
{
  "type": "DATA",
  "timestamp": 1751200000,
  "payload": {
    "device_id": "设备ID",
    "points": [
      {
        "key": "数据点名称",
        "value": 数值,
        "data_type": "数据类型",
        "timestamp": 1751200000
      }
    ]
  }
}
```

### RESPONSE消息
```json
{
  "type": "RESPONSE",
  "id": "对应请求ID",
  "timestamp": 1751200000,
  "payload": {
    "status": "success|error",
    "message": "描述信息"
  }
}
```

## 数据类型

| 类型 | 范围 | 用途 |
|------|------|------|
| bool | true/false | 开关状态 |
| int16 | -32768~32767 | 小整数 |
| uint16 | 0~65535 | 无符号小整数 |
| int32 | -2147483648~2147483647 | 大整数 |
| uint32 | 0~4294967295 | 无符号大整数 |
| float32 | IEEE 754 | 浮点数 |

## 文件结构

### Gateway端
```
internal/plugin/
├── isp_protocol.go      # 协议定义
├── isp_client.go        # 客户端实现
└── isp_adapter_proxy.go # 适配器代理
```

### Sidecar端
```
plugins/modbus-sidecar/
├── isp_server.go        # 服务器实现
├── main.go             # 主程序
└── modbus-sidecar.exe  # 可执行文件
```

## 配置文件

### 插件配置 (plugins/modbus-sidecar-isp.json)
```json
{
  "name": "modbus-sensor",
  "version": "1.0.0",
  "type": "adapter",
  "mode": "isp-sidecar",
  "entry": "./plugins/modbus-sidecar/modbus-sidecar.exe",
  "isp_port": 50052
}
```

## 启动顺序

1. **启动Modbus模拟器**
   ```bash
   python tools/modbus_simulator.py
   ```

2. **启动Sidecar**
   ```bash
   ./plugins/modbus-sidecar/modbus-sidecar.exe
   ```

3. **启动Gateway**
   ```bash
   ./iot-gateway.exe
   ```

## 常用命令

### 编译
```bash
# Gateway
go build -o iot-gateway.exe ./cmd/gateway

# Sidecar
cd plugins/modbus-sidecar
go build -o modbus-sidecar.exe
```

### 测试连接
```bash
# 检查端口监听
netstat -an | findstr 50052

# 检查进程
tasklist | findstr modbus-sidecar
```

## 故障排除

### 连接问题
- [ ] Sidecar是否启动？
- [ ] 端口50052是否被占用？
- [ ] 防火墙是否阻止连接？

### 数据问题
- [ ] Modbus模拟器是否运行？
- [ ] 寄存器配置是否正确？
- [ ] 数据类型是否匹配？

### 日志关键字
- `ISP客户端连接成功` - 连接建立
- `发送CONFIG消息` - 配置发送
- `收到DATA消息` - 数据接收
- `连接错误` - 连接问题

## 性能指标

- **连接建立**: < 100ms
- **数据延迟**: < 10ms
- **采集频率**: 2秒/次
- **批量大小**: 不限制
- **内存占用**: < 50MB

## 扩展点

### 添加新消息类型
1. 在`isp_protocol.go`中定义结构体
2. 在客户端/服务器中添加处理逻辑
3. 更新消息路由

### 添加新数据类型
1. 更新`DataType`枚举
2. 添加类型转换逻辑
3. 更新文档

## 版本兼容性

- **v1.0.0**: 基础功能
- **向后兼容**: 支持旧版本消息格式
- **协议升级**: 通过版本字段协商 