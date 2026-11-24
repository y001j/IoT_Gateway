package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

// ISPAdapterProxy ISP适配器代理
type ISPAdapterProxy struct {
	*southbound.BaseAdapter
	address    string
	client     *ISPClient
	config     *ConfigPayload // 保存配置用于重连
	mu         sync.Mutex
	cancelFunc context.CancelFunc
	dataCh     chan *ISPMessage
	outputCh   chan<- model.Point // 保存输出通道用于重连
}

// NewISPAdapterProxy 创建ISP适配器代理
func NewISPAdapterProxy(name string, address string) (*ISPAdapterProxy, error) {
	client := NewISPClient(address)

	proxy := &ISPAdapterProxy{
		BaseAdapter: southbound.NewBaseAdapter(name, "isp-sidecar"),
		address:     address,
		client:      client,
		dataCh:      make(chan *ISPMessage, 100),
	}

	proxy.SetHealthStatus("healthy", "ISP adapter proxy created")

	log.Info().
		Str("name", name).
		Str("address", address).
		Msg("创建ISP适配器代理")

	return proxy, nil
}

// Init 初始化适配器
func (p *ISPAdapterProxy) Init(cfg json.RawMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 确保配置是有效的JSON
	if !json.Valid(cfg) {
		return fmt.Errorf("无效的JSON配置")
	}

	// 连接到ISP服务器
	ctx := context.Background()
	if err := p.client.Connect(ctx); err != nil {
		return fmt.Errorf("连接ISP服务器失败: %w", err)
	}

	// 解析配置
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(cfg, &rawConfig); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	// 提取plugin_config字段作为实际的配置
	var actualConfig map[string]interface{}
	if pluginConfig, ok := rawConfig["plugin_config"].(map[string]interface{}); ok {
		actualConfig = pluginConfig
	} else {
		actualConfig = rawConfig // 如果没有plugin_config，使用原始配置
	}

	// 转换为ISP配置格式
	config := p.convertToISPConfig(actualConfig)

	// 保存配置用于重连
	p.config = &config

	// 发送配置消息
	log.Debug().
		Str("name", p.Name()).
		Int("registers_count", len(config.Registers)).
		Str("address", config.Address).
		Str("mode", config.Mode).
		Msg("🔵 [调试] 准备发送ISP配置消息")

	configMsg, err := NewConfigMessage("config-"+fmt.Sprintf("%d", time.Now().Unix()), config)
	if err != nil {
		return fmt.Errorf("创建配置消息失败: %w", err)
	}

	// 发送配置并等待响应
	response, err := p.client.SendRequest(configMsg, 30*time.Second)
	if err != nil {
		return fmt.Errorf("发送配置请求失败: %w", err)
	}

	// 解析响应
	respPayload, err := response.ParseResponsePayload()
	if err != nil {
		return fmt.Errorf("解析配置响应失败: %w", err)
	}

	if !respPayload.Success {
		return fmt.Errorf("Sidecar配置失败: %s", respPayload.Error)
	}

	log.Info().
		Str("name", p.Name()).
		Msg("ISP适配器代理初始化成功")

	return nil
}

// convertToISPConfig 将原始配置转换为ISP配置格式
func (p *ISPAdapterProxy) convertToISPConfig(rawConfig map[string]interface{}) ConfigPayload {
	config := ConfigPayload{
		Extra: make(map[string]interface{}),
	}

	// 转换基本配置 - 支持多种字段名映射
	if mode, ok := rawConfig["mode"].(string); ok {
		config.Mode = mode
	} else if protocol, ok := rawConfig["protocol"].(string); ok {
		config.Mode = protocol // protocol字段映射到Mode
	}

	if address, ok := rawConfig["address"].(string); ok {
		config.Address = address
	} else if host, ok := rawConfig["host"].(string); ok {
		// 构建完整的地址 host:port
		port := 502 // 默认Modbus端口
		if portVal, ok := rawConfig["port"].(float64); ok {
			port = int(portVal)
		}
		config.Address = fmt.Sprintf("%s:%d", host, port)
	}
	// 处理超时配置（支持多种格式）
	if timeout, ok := rawConfig["timeout_ms"].(float64); ok {
		config.TimeoutMS = int(timeout)
	} else if timeoutStr, ok := rawConfig["timeout"].(string); ok {
		if duration, err := time.ParseDuration(timeoutStr); err == nil {
			config.TimeoutMS = int(duration.Milliseconds())
		}
	}

	// 处理间隔配置（支持多种格式）
	if interval, ok := rawConfig["interval_ms"].(float64); ok {
		config.IntervalMS = int(interval)
	} else if intervalStr, ok := rawConfig["interval"].(string); ok {
		if duration, err := time.ParseDuration(intervalStr); err == nil {
			config.IntervalMS = int(duration.Milliseconds())
		}
	}

	// 转换寄存器配置
	if registers, ok := rawConfig["registers"].([]interface{}); ok {
		for _, reg := range registers {
			if regMap, ok := reg.(map[string]interface{}); ok {
				regConfig := RegisterConfig{}

				if key, ok := regMap["key"].(string); ok {
					regConfig.Key = key
				}
				if address, ok := regMap["address"].(float64); ok {
					regConfig.Address = uint16(address)
				}
				if quantity, ok := regMap["quantity"].(float64); ok {
					regConfig.Quantity = uint16(quantity)
				}
				if regType, ok := regMap["type"].(string); ok {
					// 将寄存器类型转换为功能码
					switch regType {
					case "coil":
						regConfig.Function = 1 // 读取线圈
						if regConfig.Quantity == 0 {
							regConfig.Quantity = 1
						}
					case "discrete_input":
						regConfig.Function = 2 // 读取离散输入
						if regConfig.Quantity == 0 {
							regConfig.Quantity = 1
						}
					case "holding_register":
						regConfig.Function = 3 // 读取保持寄存器
						if regConfig.Quantity == 0 {
							regConfig.Quantity = 1
						}
					case "input_register":
						regConfig.Function = 4 // 读取输入寄存器
						if regConfig.Quantity == 0 {
							regConfig.Quantity = 1
						}
					}
				}
				// 处理数据类型字段（优先使用data_type字段）
				if dataType, ok := regMap["data_type"].(string); ok {
					regConfig.Type = dataType
				} else if regType, ok := regMap["type"].(string); ok {
					regConfig.Type = regType
				}
				if function, ok := regMap["function"].(float64); ok {
					regConfig.Function = uint8(function) // 手动指定的功能码优先级更高
				}
				if scale, ok := regMap["scale"].(float64); ok {
					regConfig.Scale = scale
				}
				if deviceID, ok := regMap["device_id"].(float64); ok {
					regConfig.DeviceID = byte(deviceID)
				}
				if byteOrder, ok := regMap["byte_order"].(string); ok {
					regConfig.ByteOrder = byteOrder
				}
				if bitOffset, ok := regMap["bit_offset"].(float64); ok {
					regConfig.BitOffset = int(bitOffset)
				}

				// 转换标签
				if tags, ok := regMap["tags"].(map[string]interface{}); ok {
					regConfig.Tags = make(map[string]string)
					for k, v := range tags {
						if strVal, ok := v.(string); ok {
							regConfig.Tags[k] = strVal
						}
					}
				}

				config.Registers = append(config.Registers, regConfig)
			}
		}
	}

	return config
}

// Start 启动适配器
func (p *ISPAdapterProxy) Start(ctx context.Context, ch chan<- model.Point) error {
	p.mu.Lock()
	if p.IsRunning() {
		p.mu.Unlock()
		return fmt.Errorf("适配器已经在运行")
	}

	// 确保ISP客户端已连接
	if !p.client.IsConnected() {
		if err := p.client.Connect(ctx); err != nil {
			p.mu.Unlock()
			p.SetLastError(fmt.Errorf("连接ISP服务器失败: %w", err))
			return fmt.Errorf("连接ISP服务器失败: %w", err)
		}
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(ctx)
	p.cancelFunc = cancel
	p.SetRunning(true)
	p.outputCh = ch // 保存输出通道用于重连
	p.SetHealthStatus("healthy", "ISP adapter proxy started")
	p.mu.Unlock()

	log.Info().
		Str("name", p.Name()).
		Msg("ISP适配器代理启动成功")

	// 设置数据消息处理器
	p.client.SetDataHandler(func(msg *ISPMessage) {
		p.handleDataMessage(msg, ch)
	})

	// 启动数据接收协程
	go p.dataReceiveLoop(ctx, ch)

	return nil
}

// dataReceiveLoop 数据接收循环
func (p *ISPAdapterProxy) dataReceiveLoop(ctx context.Context, ch chan<- model.Point) {
	defer func() {
		p.mu.Lock()
		p.SetRunning(false)
		p.cancelFunc = nil
		p.mu.Unlock()
		log.Info().
			Str("name", p.Name()).
			Msg("ISP适配器代理数据接收循环退出")
	}()

	// 保持循环运行，主要用于状态监控和重连
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			// 定期检查连接状态
			p.mu.Lock()
			client := p.client
			p.mu.Unlock()
			if !client.IsConnected() {
				p.SetHealthStatus("degraded", "ISP connection lost, attempting reconnection")
				log.Warn().Str("name", p.Name()).Msg("ISP客户端连接断开，尝试重连...")

				// 尝试重连
				if err := p.reconnect(ctx); err != nil {
					p.SetLastError(err)
					p.SetHealthStatus("unhealthy", fmt.Sprintf("Reconnection failed: %v", err))
					log.Error().Err(err).Str("name", p.Name()).Msg("重连失败")
					// 等待一段时间后再次尝试（可被停止信号打断）
					select {
					case <-ctx.Done():
						return
					case <-time.After(10 * time.Second):
					}
					continue
				}

				p.SetHealthStatus("healthy", "ISP connection restored")
				log.Info().Str("name", p.Name()).Msg("ISP客户端重连成功")
			}
			// ISP适配器代理运行正常（移除频繁debug日志）
		}
	}
}

// reconnect 重连ISP客户端
func (p *ISPAdapterProxy) reconnect(ctx context.Context) error {
	// 断开现有连接
	p.mu.Lock()
	old := p.client
	p.mu.Unlock()
	old.Disconnect()

	// 创建新的ISP客户端
	newClient := NewISPClient(p.address)

	// 连接到ISP服务器
	if err := newClient.Connect(ctx); err != nil {
		return fmt.Errorf("连接ISP服务器失败: %w", err)
	}

	// 重新发送配置
	if p.config != nil {
		configMsg, err := NewConfigMessage("config-reconnect-"+fmt.Sprintf("%d", time.Now().Unix()), *p.config)
		if err != nil {
			newClient.Disconnect()
			return fmt.Errorf("创建重连配置消息失败: %w", err)
		}
		if _, err := newClient.SendRequest(configMsg, 10*time.Second); err != nil {
			newClient.Disconnect()
			return fmt.Errorf("重新发送配置失败: %w", err)
		}
	}

	// 设置数据消息处理器
	newClient.SetDataHandler(func(msg *ISPMessage) {
		if p.outputCh != nil {
			p.handleDataMessage(msg, p.outputCh)
		}
	})

	// 替换客户端（加锁：Status/GetMetrics 会并发读取 p.client）
	p.mu.Lock()
	p.client = newClient
	p.mu.Unlock()

	return nil
}

// handleDataMessage 处理数据消息
func (p *ISPAdapterProxy) handleDataMessage(msg *ISPMessage, ch chan<- model.Point) {
	log.Debug().
		Str("name", p.Name()).
		Msg("🔵 [调试] ISP适配器代理收到数据消息")

	dataPayload, err := msg.ParseDataPayload()
	if err != nil {
		p.SetLastError(fmt.Errorf("解析数据消息失败: %w", err))
		log.Error().Err(err).Msg("解析数据消息失败")
		return
	}

	log.Debug().
		Str("name", p.Name()).
		Int("points_count", len(dataPayload.Points)).
		Msg("🔵 [调试] 解析到数据点")

	for _, point := range dataPayload.Points {
		// 转换为内部数据点格式
		internalPoint := p.convertDataPoint(point)
		if internalPoint != nil {
			select {
			case ch <- *internalPoint:
				// 成功发送，更新指标
				p.IncrementDataPoints()
				log.Debug().
					Str("name", p.Name()).
					Str("key", internalPoint.Key).
					Interface("value", internalPoint.Value).
					Msg("🔵 [调试] 数据点已转发到通道")
			default:
				p.SetLastError(fmt.Errorf("数据通道已满，丢弃数据点: %s", point.Key))
				log.Warn().
					Str("name", p.Name()).
					Str("key", point.Key).
					Msg("数据通道已满，丢弃数据点")
			}
		}
	}
}

// convertDataPoint 把 sidecar 的数据点转换为网关数据点；未知数据类型返回 nil
func (p *ISPAdapterProxy) convertDataPoint(point DataPoint) *model.Point {
	ts := time.Unix(0, point.Timestamp)
	if point.Timestamp == 0 {
		// sidecar 未提供时间戳时以前会得到 1970 年的时间
		ts = time.Now()
	}
	internalPoint := &model.Point{
		Key:       point.Key,
		DeviceID:  point.Source,
		Timestamp: ts,
		Quality:   point.Quality,
		Value:     point.Value,
	}
	if len(point.Tags) > 0 {
		internalPoint.SetTagsSafe(point.Tags)
	}

	switch point.Type {
	case "bool", "boolean":
		internalPoint.Type = model.TypeBool
	case "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		internalPoint.Type = model.TypeInt
	case "float", "float32", "float64", "double":
		internalPoint.Type = model.TypeFloat
	case "string":
		internalPoint.Type = model.TypeString
	default:
		log.Warn().Str("name", p.Name()).Str("key", point.Key).Str("type", point.Type).Msg("未知数据类型")
		return nil
	}
	return internalPoint
}

// Stop 停止适配器
func (p *ISPAdapterProxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.IsRunning() {
		return nil
	}

	// 取消上下文
	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}

	p.SetRunning(false)
	p.SetHealthStatus("healthy", "ISP adapter proxy stopped")

	log.Info().
		Str("name", p.Name()).
		Msg("ISP适配器代理已停止")

	return nil
}

// Status 获取适配器状态
func (p *ISPAdapterProxy) Status() (map[string]interface{}, error) {
	p.mu.Lock()
	running := p.IsRunning()
	connected := p.client.IsConnected()
	p.mu.Unlock()

	status := map[string]interface{}{
		"name":      p.Name(),
		"running":   running,
		"connected": connected,
		"address":   p.address,
		"protocol":  "ISP",
	}

	// 如果已连接，尝试获取远程状态
	if connected {
		statusMsg := NewStatusMessage("status-" + fmt.Sprintf("%d", time.Now().Unix()))
		resp, err := p.client.SendRequest(statusMsg, 5*time.Second)
		if err != nil {
			status["remote_error"] = err.Error()
		} else {
			respPayload, err := resp.ParseResponsePayload()
			if err != nil {
				status["parse_error"] = err.Error()
			} else if respPayload.Success {
				status["remote_status"] = respPayload.Data
			}
		}
	}

	return status, nil
}

// GetMetrics 获取sidecar适配器指标
func (p *ISPAdapterProxy) GetMetrics() (interface{}, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.client.IsConnected() {
		return nil, fmt.Errorf("ISP客户端未连接")
	}

	// 发送指标请求
	metricsMsg := NewMetricsRequestMessage("metrics-" + fmt.Sprintf("%d", time.Now().Unix()))
	response, err := p.client.SendRequest(metricsMsg, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("获取sidecar指标失败: %w", err)
	}

	// 解析响应
	respPayload, err := response.ParseResponsePayload()
	if err != nil {
		return nil, fmt.Errorf("解析指标响应失败: %w", err)
	}

	if !respPayload.Success {
		return nil, fmt.Errorf("sidecar返回错误: %s", respPayload.Error)
	}

	// 将响应数据转换为指标格式
	if metricsData, ok := respPayload.Data.(map[string]interface{}); ok {
		// 构造符合BaseAdapter接口的指标结构
		metrics := struct {
			DataPointsCollected int64         `json:"data_points_collected"`
			ErrorsCount         int64         `json:"errors_count"`
			ConnectionUptime    time.Duration `json:"connection_uptime"`
			LastError           string        `json:"last_error"`
			AverageResponseTime float64       `json:"average_response_time"`
		}{}

		// 安全地转换各个字段
		if v, ok := metricsData["data_points_collected"].(float64); ok {
			metrics.DataPointsCollected = int64(v)
		}
		if v, ok := metricsData["errors_count"].(float64); ok {
			metrics.ErrorsCount = int64(v)
		}
		if v, ok := metricsData["connection_uptime"].(float64); ok {
			metrics.ConnectionUptime = time.Duration(int64(v)) * time.Second
		}
		if v, ok := metricsData["last_error"].(string); ok {
			metrics.LastError = v
		}
		if v, ok := metricsData["average_response_time"].(float64); ok {
			metrics.AverageResponseTime = v
		}

		return metrics, nil
	}

	return respPayload.Data, nil
}

// Close 关闭适配器代理
func (p *ISPAdapterProxy) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 停止运行
	if p.IsRunning() && p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
		p.SetRunning(false)
	}

	// 断开ISP客户端连接
	if p.client != nil {
		p.client.Disconnect()
	}

	p.SetHealthStatus("healthy", "ISP adapter proxy closed")

	log.Info().
		Str("name", p.Name()).
		Msg("ISP适配器代理已关闭")

	return nil
}
