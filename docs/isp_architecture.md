# ISP协议架构设计

## 系统架构概览

ISP (IoT Sidecar Protocol) 协议采用客户端-服务器架构，Gateway作为客户端，Sidecar作为服务器。

## 组件架构图

```mermaid
graph TB
    subgraph "IoT Gateway"
        A[Plugin Manager] --> B[ISP Adapter Proxy]
        B --> C[ISP Client]
        C --> D[TCP Connection]
    end
    
    subgraph "Modbus Sidecar"
        E[ISP Server] --> F[Message Router]
        F --> G[Modbus Client]
        G --> H[Modbus Device/Simulator]
    end
    
    subgraph "Northbound"
        I[MQTT Connector]
        J[NATS Connector]
        K[InfluxDB Connector]
    end
    
    D -.->|TCP:50052| E
    B --> I
    B --> J
    B --> K
    
    style A fill:#e1f5fe
    style E fill:#f3e5f5
    style H fill:#fff3e0
```

## 数据流程图

```mermaid
sequenceDiagram
    participant G as Gateway
    participant S as Sidecar
    participant M as Modbus Device
    participant C as Cloud (MQTT/NATS)
    
    Note over G,S: 1. 连接建立阶段
    G->>S: TCP连接 (端口50052)
    S-->>G: 连接确认
    
    Note over G,S: 2. 配置阶段
    G->>S: CONFIG消息
    Note over S: 解析配置<br/>初始化Modbus客户端
    S-->>G: RESPONSE (success)
    
    Note over G,S: 3. 数据采集阶段
    loop 每2秒
        S->>M: 读取Modbus寄存器
        M-->>S: 寄存器数据
        Note over S: 解析数据<br/>转换为数据点
        S->>G: DATA消息 (批量数据点)
        Note over G: 转换为内部格式
        G->>C: 发布到云端
    end
    
    Note over G,S: 4. 状态监控 (可选)
    G->>S: STATUS查询
    S-->>G: RESPONSE (状态信息)
    
    Note over G,S: 5. 心跳保活
    G<-->S: HEARTBEAT消息
```

## 消息处理流程

```mermaid
flowchart TD
    A[接收TCP消息] --> B{解析JSON}
    B -->|成功| C{消息类型}
    B -->|失败| D[记录错误日志]
    
    C -->|CONFIG| E[处理配置]
    C -->|DATA| F[处理数据]
    C -->|STATUS| G[查询状态]
    C -->|HEARTBEAT| H[更新心跳]
    C -->|RESPONSE| I[处理响应]
    
    E --> J{配置验证}
    J -->|通过| K[应用配置]
    J -->|失败| L[返回错误]
    K --> M[返回成功响应]
    
    F --> N[解析数据点]
    N --> O[数据类型转换]
    O --> P[发送到Northbound]
    
    G --> Q[收集状态信息]
    Q --> R[返回状态响应]
    
    style A fill:#e8f5e8
    style C fill:#fff2cc
    style E fill:#ffe6cc
    style F fill:#e1f5fe
```

## 网络协议栈

```mermaid
graph TB
    subgraph "应用层"
        A[ISP Protocol<br/>JSON Messages]
    end
    
    subgraph "表示层"
        B[UTF-8 Encoding<br/>Line-delimited JSON]
    end
    
    subgraph "传输层"
        C[TCP Protocol<br/>Port 50052]
    end
    
    subgraph "网络层"
        D[IP Protocol<br/>IPv4/IPv6]
    end
    
    subgraph "数据链路层"
        E[Ethernet/WiFi]
    end
    
    A --> B
    B --> C
    C --> D
    D --> E
    
    style A fill:#e1f5fe
    style C fill:#f3e5f5
```

## 错误处理机制

```mermaid
flowchart TD
    A[网络错误] --> B{错误类型}
    B -->|连接断开| C[自动重连]
    B -->|超时| D[重试机制]
    B -->|格式错误| E[记录日志]
    
    C --> F{重连次数}
    F -->|< 3次| G[立即重连]
    F -->|>= 3次| H[指数退避]
    
    D --> I{重试次数}
    I -->|< 5次| J[重新发送]
    I -->|>= 5次| K[放弃请求]
    
    G --> L[建立连接]
    H --> M[等待后重连]
    J --> N[发送消息]
    
    style A fill:#ffebee
    style C fill:#e8f5e8
    style D fill:#fff3e0
```

## 性能监控指标

```mermaid
graph LR
    subgraph "连接指标"
        A[连接建立时间]
        B[连接稳定性]
        C[重连次数]
    end
    
    subgraph "传输指标"
        D[消息延迟]
        E[吞吐量]
        F[错误率]
    end
    
    subgraph "业务指标"
        G[数据点数量]
        H[采集频率]
        I[数据完整性]
    end
    
    A --> D
    B --> E
    C --> F
    D --> G
    E --> H
    F --> I
    
    style A fill:#e1f5fe
    style D fill:#f3e5f5
    style G fill:#e8f5e8
```

## 扩展架构

```mermaid
graph TB
    subgraph "当前实现"
        A[Modbus Sidecar<br/>ISP Server]
    end
    
    subgraph "未来扩展"
        B[OPC-UA Sidecar<br/>ISP Server]
        C[MQTT Sidecar<br/>ISP Server]
        D[HTTP Sidecar<br/>ISP Server]
    end
    
    subgraph "Gateway"
        E[ISP Client<br/>统一接口]
    end
    
    A --> E
    B --> E
    C --> E
    D --> E
    
    E --> F[Plugin Manager]
    F --> G[Northbound Connectors]
    
    style A fill:#e1f5fe
    style B fill:#f3e5f5
    style C fill:#fff3e0
    style D fill:#e8f5e8
```

## 部署架构

```mermaid
graph TB
    subgraph "开发环境"
        A[Gateway Process]
        B[Sidecar Process]
        C[Modbus Simulator]
    end
    
    subgraph "生产环境"
        D[Gateway Container]
        E[Sidecar Container]
        F[Real Modbus Device]
    end
    
    subgraph "云端服务"
        G[MQTT Broker]
        H[NATS Server]
        I[InfluxDB]
    end
    
    A <--> B
    B <--> C
    D <--> E
    E <--> F
    
    A --> G
    A --> H
    A --> I
    D --> G
    D --> H
    D --> I
    
    style A fill:#e1f5fe
    style D fill:#f3e5f5
    style G fill:#fff3e0
```

## 关键设计决策

### 1. 为什么选择TCP而不是UDP？
- **可靠性**: IoT数据传输需要保证可靠性
- **顺序**: 数据点需要按时间顺序处理
- **连接状态**: 需要知道连接是否正常

### 2. 为什么选择JSON而不是二进制协议？
- **可读性**: 便于调试和问题排查
- **扩展性**: 易于添加新字段
- **兼容性**: 跨语言支持良好

### 3. 为什么使用Line-delimited JSON？
- **流式处理**: 支持连续的消息流
- **边界清晰**: 每行一个完整消息
- **解析简单**: 标准的换行符分隔

### 4. 批量传输的优势
- **效率**: 减少网络往返次数
- **吞吐量**: 提高数据传输速度
- **资源利用**: 降低CPU和网络开销

## 安全考虑

```mermaid
graph TD
    A[网络安全] --> B[TLS加密]
    A --> C[IP白名单]
    A --> D[端口防火墙]
    
    E[数据安全] --> F[消息校验]
    E --> G[数据验证]
    E --> H[异常过滤]
    
    I[访问控制] --> J[认证机制]
    I --> K[授权检查]
    I --> L[审计日志]
    
    style A fill:#ffebee
    style E fill:#e8f5e8
    style I fill:#e1f5fe
```

## 总结

ISP协议通过简洁的设计和清晰的架构，成功替换了复杂的gRPC协议，为IoT Gateway提供了高效、可靠、易维护的Sidecar通信解决方案。其模块化的设计使得系统具有良好的扩展性，能够支持未来更多类型的IoT设备接入。 