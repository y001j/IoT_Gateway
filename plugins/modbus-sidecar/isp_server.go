package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/goburrow/modbus"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/plugin"
)

// ModbusLongConnection 表示一个长连接（单连接多设备）
type ModbusLongConnection struct {
	client    modbus.Client
	handler   modbus.ClientHandler
	lastUsed  time.Time
	mu        sync.Mutex
	connected bool
	config    *plugin.ConfigPayload
}

// ISPServer ISP服务器
type ISPServer struct {
	address        string
	listener       net.Listener
	clients        map[string]*ISPClientConn
	clientsMu      sync.RWMutex
	running        bool
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
	modbusConf     *plugin.ConfigPayload
	modbusClient   modbus.Client
	dataTimer      *time.Timer
	heartbeatTimer *time.Timer           // 心跳定时器
	longConn       *ModbusLongConnection // 改为单一长连接
	
	// 指标收集
	startTime           time.Time
	dataPointsCollected int64
	errorsCount         int64
	lastError           string
	responseTimes       []float64
	avgResponseTime     float64
	maxResponseTimes    int
	metricsMu           sync.RWMutex
}

// ISPClientConn ISP客户端连接
type ISPClientConn struct {
	conn    net.Conn
	scanner *bufio.Scanner
	writer  *bufio.Writer
	id      string
	server  *ISPServer
}

// 使用标准ISP协议定义
type Register = plugin.RegisterConfig
type ISPModbusConfig = plugin.ConfigPayload

// 使用标准ISP消息类型常量
const (
	MessageTypeConfig    = plugin.MessageTypeConfig
	MessageTypeData      = plugin.MessageTypeData
	MessageTypeResponse  = plugin.MessageTypeResponse
	MessageTypeStatus    = plugin.MessageTypeStatus
	MessageTypeHeartbeat = plugin.MessageTypeHeartbeat
	MessageTypeMetrics   = plugin.MessageTypeMetrics
)

// 使用标准ISP协议定义
type ISPMessage = plugin.ISPMessage
type DataPoint = plugin.DataPoint

// NewModbusLongConnection 创建长连接
func NewModbusLongConnection(config *plugin.ConfigPayload) (*ModbusLongConnection, error) {
	var handler modbus.ClientHandler

	switch config.Mode {
	case "tcp":
		handler = modbus.NewTCPClientHandler(config.Address)
		handler.(*modbus.TCPClientHandler).Timeout = time.Duration(config.TimeoutMS) * time.Millisecond
	case "rtu":
		handler = modbus.NewRTUClientHandler(config.Address)
		handler.(*modbus.RTUClientHandler).Timeout = time.Duration(config.TimeoutMS) * time.Millisecond
	default:
		return nil, fmt.Errorf("不支持的Modbus模式: %s", config.Mode)
	}

	// 建立连接
	var err error
	switch config.Mode {
	case "tcp":
		err = handler.(*modbus.TCPClientHandler).Connect()
	case "rtu":
		err = handler.(*modbus.RTUClientHandler).Connect()
	}
	if err != nil {
		return nil, fmt.Errorf("连接Modbus设备失败: %w", err)
	}

	client := modbus.NewClient(handler)

	conn := &ModbusLongConnection{
		client:    client,
		handler:   handler,
		lastUsed:  time.Now(),
		connected: true,
		config:    config,
	}

	log.Info().
		Str("mode", config.Mode).
		Str("address", config.Address).
		Msg("创建Modbus长连接成功")

	return conn, nil
}

// ReadRegister 使用长连接读取寄存器（线程安全）
func (conn *ModbusLongConnection) ReadRegister(reg *Register) (interface{}, error) {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	// 检查连接状态
	if !conn.connected {
		if err := conn.reconnect(); err != nil {
			return nil, fmt.Errorf("重连失败: %w", err)
		}
	}

	// 设置从站地址
	switch conn.config.Mode {
	case "tcp":
		conn.handler.(*modbus.TCPClientHandler).SlaveId = reg.DeviceID
	case "rtu":
		conn.handler.(*modbus.RTUClientHandler).SlaveId = reg.DeviceID
	}

	var results []byte
	var err error

	switch reg.Function {
	case 1: // 读取线圈
		results, err = conn.client.ReadCoils(reg.Address, reg.Quantity)
	case 2: // 读取离散输入
		results, err = conn.client.ReadDiscreteInputs(reg.Address, reg.Quantity)
	case 3: // 读取保持寄存器
		results, err = conn.client.ReadHoldingRegisters(reg.Address, reg.Quantity)
	case 4: // 读取输入寄存器
		results, err = conn.client.ReadInputRegisters(reg.Address, reg.Quantity)
	default:
		return nil, fmt.Errorf("不支持的功能码: %d", reg.Function)
	}

	if err != nil {
		// 标记连接为断开状态，下次使用时会重连
		conn.connected = false
		return nil, fmt.Errorf("读取寄存器失败: %w", err)
	}

	// 更新最后使用时间
	conn.lastUsed = time.Now()

	// 解析数据
	return conn.parseRegisterValue(results, reg)
}

// reconnect 重新连接
func (conn *ModbusLongConnection) reconnect() error {
	// 先关闭旧连接
	conn.Close()

	// 重新建立连接
	var err error
	switch conn.config.Mode {
	case "tcp":
		err = conn.handler.(*modbus.TCPClientHandler).Connect()
	case "rtu":
		err = conn.handler.(*modbus.RTUClientHandler).Connect()
	}
	if err != nil {
		return fmt.Errorf("重连失败: %w", err)
	}

	conn.connected = true
	log.Info().
		Str("mode", conn.config.Mode).
		Str("address", conn.config.Address).
		Msg("Modbus长连接重连成功")

	return nil
}

// Close 关闭连接
func (conn *ModbusLongConnection) Close() {
	if conn.handler != nil {
		switch conn.config.Mode {
		case "tcp":
			conn.handler.(*modbus.TCPClientHandler).Close()
		case "rtu":
			conn.handler.(*modbus.RTUClientHandler).Close()
		}
	}
	conn.connected = false
}

// parseRegisterValue 解析寄存器值
func (conn *ModbusLongConnection) parseRegisterValue(data []byte, reg *Register) (interface{}, error) {
	switch reg.Type {
	case "bool":
		if len(data) > 0 {
			return data[0] != 0, nil
		}
		return false, nil

	case "int16":
		if len(data) >= 2 {
			value := int16(data[0])<<8 | int16(data[1])
			return int(float64(value) * reg.Scale), nil
		}
		return 0, fmt.Errorf("数据长度不足")

	case "uint16":
		if len(data) >= 2 {
			value := uint16(data[0])<<8 | uint16(data[1])
			return int(float64(value) * reg.Scale), nil
		}
		return 0, fmt.Errorf("数据长度不足")

	case "int32":
		if len(data) >= 4 {
			value := int32(data[0])<<24 | int32(data[1])<<16 | int32(data[2])<<8 | int32(data[3])
			return int(float64(value) * reg.Scale), nil
		}
		return 0, fmt.Errorf("数据长度不足")

	case "uint32":
		if len(data) >= 4 {
			value := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
			return int(float64(value) * reg.Scale), nil
		}
		return 0, fmt.Errorf("数据长度不足")

	case "float", "float32":
		if len(data) >= 2 {
			value := uint16(data[0])<<8 | uint16(data[1])
			return float64(value) * reg.Scale, nil
		}
		return 0.0, fmt.Errorf("数据长度不足")

	default:
		// 默认为uint16
		if len(data) >= 2 {
			value := uint16(data[0])<<8 | uint16(data[1])
			return float64(value) * reg.Scale, nil
		}
		return 0.0, fmt.Errorf("数据长度不足")
	}
}

// NewISPServer 创建ISP服务器
func NewISPServer(address string) *ISPServer {
	return &ISPServer{
		address:          address,
		clients:          make(map[string]*ISPClientConn),
		startTime:        time.Now(),
		maxResponseTimes: 100,
		responseTimes:    make([]float64, 0, 100),
	}
}

// Start 启动ISP服务器
func (s *ISPServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("ISP服务器已经在运行")
	}

	// 创建可取消的上下文
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 监听TCP端口
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("监听端口失败: %w", err)
	}

	s.listener = listener
	s.running = true

	log.Info().
		Str("address", s.address).
		Msg("ISP服务器启动成功")

	// 启动接受连接的协程
	go s.acceptLoop()

	return nil
}

// Stop 停止ISP服务器
func (s *ISPServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	// 取消上下文
	if s.cancel != nil {
		s.cancel()
	}

	// 关闭监听器
	if s.listener != nil {
		s.listener.Close()
	}

	// 停止数据定时器
	if s.dataTimer != nil {
		s.dataTimer.Stop()
	}

	// 停止心跳定时器
	if s.heartbeatTimer != nil {
		s.heartbeatTimer.Stop()
	}

	// 关闭长连接
	if s.longConn != nil {
		s.longConn.Close()
	}

	// 关闭所有客户端连接
	s.clientsMu.Lock()
	for _, client := range s.clients {
		client.conn.Close()
	}
	s.clients = make(map[string]*ISPClientConn)
	s.clientsMu.Unlock()

	log.Info().
		Str("address", s.address).
		Msg("ISP服务器已停止")

	return nil
}

// acceptLoop 接受连接循环
func (s *ISPServer) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				if s.running {
					log.Error().Err(err).Msg("接受连接失败")
				}
				continue
			}

			// 创建客户端连接
			clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
			client := &ISPClientConn{
				conn:    conn,
				scanner: bufio.NewScanner(conn),
				writer:  bufio.NewWriter(conn),
				id:      clientID,
				server:  s,
			}

			// 添加到客户端列表
			s.clientsMu.Lock()
			s.clients[clientID] = client
			s.clientsMu.Unlock()

			log.Info().
				Str("client_id", clientID).
				Str("remote_addr", conn.RemoteAddr().String()).
				Msg("新客户端连接")

			// 启动客户端处理协程
			go client.handleConnection()
		}
	}
}

// handleConnection 处理客户端连接
func (c *ISPClientConn) handleConnection() {
	defer func() {
		c.conn.Close()

		// 从客户端列表中移除
		c.server.clientsMu.Lock()
		delete(c.server.clients, c.id)
		c.server.clientsMu.Unlock()

		log.Info().
			Str("client_id", c.id).
			Msg("客户端连接已关闭")
	}()

	for {
		select {
		case <-c.server.ctx.Done():
			return
		default:
			// 设置读取超时为60秒，给心跳机制足够时间
			if err := c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
				log.Error().Err(err).Msg("设置读取超时失败")
				return
			}

			// 读取一行消息
			if !c.scanner.Scan() {
				if err := c.scanner.Err(); err != nil {
					log.Error().Err(err).Msg("读取消息失败")
				}
				return
			}

			line := c.scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			// 解析消息
			var msg ISPMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				log.Error().Err(err).Str("data", string(line)).Msg("解析ISP消息失败")
				continue
			}

			log.Debug().
				Str("client_id", c.id).
				Str("type", msg.Type).
				Str("id", msg.ID).
				Msg("收到ISP消息")

			// 处理消息
			c.handleMessage(&msg)
		}
	}
}

// handleMessage 处理消息
func (c *ISPClientConn) handleMessage(msg *plugin.ISPMessage) {
	switch msg.Type {
	case MessageTypeConfig:
		c.handleConfigMessage(msg)
	case MessageTypeStatus:
		c.handleStatusMessage(msg)
	case MessageTypeHeartbeat:
		c.handleHeartbeatMessage(msg)
	case MessageTypeMetrics:
		c.handleMetricsMessage(msg)
	default:
		log.Warn().
			Str("client_id", c.id).
			Str("type", msg.Type).
			Msg("未知消息类型")
	}
}

// handleConfigMessage 处理配置消息
func (c *ISPClientConn) handleConfigMessage(msg *plugin.ISPMessage) {
	log.Info().
		Str("client_id", c.id).
		Msg("🔵 [Sidecar调试] 收到配置消息")

	var config plugin.ConfigPayload
	if err := json.Unmarshal(msg.Payload, &config); err != nil {
		c.sendErrorResponse(msg.ID, fmt.Sprintf("解析配置失败: %v", err))
		return
	}

	log.Info().
		Str("client_id", c.id).
		Str("mode", config.Mode).
		Str("address", config.Address).
		Int("timeout_ms", config.TimeoutMS).
		Int("interval_ms", config.IntervalMS).
		Int("registers_count", len(config.Registers)).
		Msg("🔵 [Sidecar调试] 配置解析成功")

	// 保存配置
	c.server.modbusConf = &config

	// 初始化Modbus客户端
	if err := c.server.initModbusClient(&config); err != nil {
		c.sendErrorResponse(msg.ID, fmt.Sprintf("初始化Modbus客户端失败: %v", err))
		return
	}

	// 启动数据采集
	log.Info().
		Str("client_id", c.id).
		Msg("🔵 [Sidecar调试] 准备启动数据采集")
	c.server.startDataCollection()

	// 启动心跳机制
	log.Info().
		Str("client_id", c.id).
		Msg("🔵 [Sidecar调试] 准备启动心跳机制")
	c.server.startHeartbeat()

	// 发送成功响应
	c.sendSuccessResponse(msg.ID, "配置成功")

	log.Info().
		Str("client_id", c.id).
		Str("mode", config.Mode).
		Str("address", config.Address).
		Int("registers", len(config.Registers)).
		Msg("🔵 [Sidecar调试] Modbus配置成功")
}

// initModbusClient 初始化Modbus客户端（使用长连接）
func (s *ISPServer) initModbusClient(config *plugin.ConfigPayload) error {
	// 创建长连接
	conn, err := NewModbusLongConnection(config)
	if err != nil {
		return fmt.Errorf("创建Modbus长连接失败: %w", err)
	}

	s.longConn = conn

	log.Info().
		Str("mode", config.Mode).
		Str("address", config.Address).
		Msg("Modbus长连接初始化成功")

	return nil
}

// startDataCollection 启动数据采集
func (s *ISPServer) startDataCollection() {
	if s.modbusConf == nil {
		return
	}

	// 停止之前的定时器
	if s.dataTimer != nil {
		s.dataTimer.Stop()
	}

	// 创建新的定时器
	s.dataTimer = time.NewTimer(time.Duration(s.modbusConf.IntervalMS) * time.Millisecond)

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.dataTimer.C:
				// 采集数据
				s.collectData()

				// 重置定时器
				s.dataTimer.Reset(time.Duration(s.modbusConf.IntervalMS) * time.Millisecond)
			}
		}
	}()

	log.Info().
		Int("interval_ms", s.modbusConf.IntervalMS).
		Msg("数据采集已启动")
}

// startHeartbeat 启动心跳机制
func (s *ISPServer) startHeartbeat() {
	// 停止之前的心跳定时器
	if s.heartbeatTimer != nil {
		s.heartbeatTimer.Stop()
	}

	// 创建心跳定时器，每15秒发送一次心跳
	s.heartbeatTimer = time.NewTimer(15 * time.Second)

	go func() {
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-s.heartbeatTimer.C:
				// 发送心跳消息
				s.sendHeartbeat()

				// 重置定时器
				s.heartbeatTimer.Reset(15 * time.Second)
			}
		}
	}()

	log.Info().Msg("心跳机制已启动")
}

// sendHeartbeat 发送心跳消息到所有客户端
func (s *ISPServer) sendHeartbeat() {
	msg := &plugin.ISPMessage{
		Type:      MessageTypeHeartbeat,
		Timestamp: time.Now().UnixNano(),
	}

	// 发送到所有客户端
	s.clientsMu.RLock()
	clientCount := len(s.clients)
	for _, client := range s.clients {
		if err := client.sendMessage(msg); err != nil {
			log.Error().
				Err(err).
				Str("client_id", client.id).
				Msg("发送心跳消息失败")
		}
	}
	s.clientsMu.RUnlock()

	if clientCount > 0 {
		log.Debug().
			Int("clients", clientCount).
			Msg("心跳消息已发送")
	}
}

// collectData 采集数据
func (s *ISPServer) collectData() {
	log.Info().
		Msg("🔵 [Sidecar调试] 开始数据采集")

	if s.modbusConf == nil || s.longConn == nil {
		log.Error().
			Bool("has_config", s.modbusConf != nil).
			Bool("has_conn", s.longConn != nil).
			Msg("🔵 [Sidecar调试] 数据采集条件不满足")
		return
	}

	var points []plugin.DataPoint
	now := time.Now().UnixNano()
	collectStart := time.Now()

	log.Info().
		Int("registers_count", len(s.modbusConf.Registers)).
		Msg("🔵 [Sidecar调试] 准备读取寄存器")

	for _, reg := range s.modbusConf.Registers {
		regStart := time.Now()
		value, err := s.readRegister(&reg)
		if err != nil {
			s.incrementErrors()
			s.setLastError(err.Error())
			log.Error().
				Err(err).
				Str("key", reg.Key).
				Uint16("address", reg.Address).
				Msg("读取寄存器失败")
			continue
		}

		// 记录成功的响应时间
		s.addResponseTime(float64(time.Since(regStart).Nanoseconds()) / 1000000.0)

		point := plugin.DataPoint{
			Key:       reg.Key,
			Source:    "modbus-sidecar",
			Timestamp: now,
			Value:     value,
			Type:      reg.Type,
			Quality:   1, // 1表示正常
			Tags:      reg.Tags,
		}

		points = append(points, point)
	}

	if len(points) > 0 {
		s.incrementDataPoints(int64(len(points)))
		s.broadcastData(points)
		log.Debug().
			Int("points", len(points)).
			Float64("collect_time_ms", float64(time.Since(collectStart).Nanoseconds())/1000000.0).
			Msg("数据采集完成")
	}
}

// readRegister 读取寄存器（使用长连接）
func (s *ISPServer) readRegister(reg *Register) (interface{}, error) {
	return s.longConn.ReadRegister(reg)
}

// broadcastData 广播数据到所有客户端
func (s *ISPServer) broadcastData(points []plugin.DataPoint) {
	// 创建数据消息
	payload, err := json.Marshal(plugin.DataPayload{
		Points: points,
	})
	if err != nil {
		log.Error().Err(err).Msg("序列化数据消息失败")
		return
	}

	msg := &plugin.ISPMessage{
		Type:      MessageTypeData,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}

	// 发送到所有客户端
	s.clientsMu.RLock()
	for _, client := range s.clients {
		if err := client.sendMessage(msg); err != nil {
			log.Error().
				Err(err).
				Str("client_id", client.id).
				Msg("发送数据消息失败")
		}
	}
	s.clientsMu.RUnlock()

	log.Debug().
		Int("points", len(points)).
		Int("clients", len(s.clients)).
		Msg("数据已广播")
}

// handleStatusMessage 处理状态查询消息
func (c *ISPClientConn) handleStatusMessage(msg *plugin.ISPMessage) {
	status := map[string]interface{}{
		"name":      "modbus-sidecar",
		"running":   c.server.running,
		"connected": c.server.modbusClient != nil,
		"health":    "healthy",
	}

	if c.server.modbusConf != nil {
		status["config"] = c.server.modbusConf
	}

	c.sendSuccessResponse(msg.ID, status)
}

// handleHeartbeatMessage 处理心跳消息
func (c *ISPClientConn) handleHeartbeatMessage(msg *plugin.ISPMessage) {
	// 简单响应心跳
	log.Debug().
		Str("client_id", c.id).
		Msg("收到心跳消息")
}

// handleMetricsMessage 处理指标请求消息
func (c *ISPClientConn) handleMetricsMessage(msg *plugin.ISPMessage) {
	// 获取服务器指标
	metrics := c.server.getMetrics()
	
	// 发送指标响应
	c.sendSuccessResponse(msg.ID, metrics)
	
	log.Debug().
		Str("client_id", c.id).
		Int64("data_points", metrics.DataPointsCollected).
		Int64("errors", metrics.ErrorsCount).
		Float64("avg_response_time", metrics.AverageResponseTime).
		Msg("发送指标响应")
}

// sendMessage 发送消息
func (c *ISPClientConn) sendMessage(msg *plugin.ISPMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	if _, err := c.writer.Write(data); err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}
	if _, err := c.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("发送换行符失败: %w", err)
	}
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("刷新缓冲区失败: %w", err)
	}

	return nil
}

// sendSuccessResponse 发送成功响应
func (c *ISPClientConn) sendSuccessResponse(id string, data interface{}) {
	response := map[string]interface{}{
		"success": true,
		"data":    data,
	}

	payload, _ := json.Marshal(response)
	msg := &plugin.ISPMessage{
		Type:      MessageTypeResponse,
		ID:        id,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}

	c.sendMessage(msg)
}

// sendErrorResponse 发送错误响应
func (c *ISPClientConn) sendErrorResponse(id string, errMsg string) {
	response := map[string]interface{}{
		"success": false,
		"error":   errMsg,
	}

	payload, _ := json.Marshal(response)
	msg := &plugin.ISPMessage{
		Type:      MessageTypeResponse,
		ID:        id,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}

	c.sendMessage(msg)
}

// 指标收集辅助方法
func (s *ISPServer) incrementDataPoints(count int64) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.dataPointsCollected += count
}

func (s *ISPServer) incrementErrors() {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.errorsCount++
}

func (s *ISPServer) setLastError(errMsg string) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.lastError = errMsg
}

func (s *ISPServer) addResponseTime(responseTimeMs float64) {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	
	// 添加新的响应时间
	s.responseTimes = append(s.responseTimes, responseTimeMs)
	
	// 如果超过最大记录数，移除最旧的记录
	if len(s.responseTimes) > s.maxResponseTimes {
		s.responseTimes = s.responseTimes[1:]
	}
	
	// 重新计算平均响应时间
	if len(s.responseTimes) > 0 {
		var sum float64
		for _, rt := range s.responseTimes {
			sum += rt
		}
		s.avgResponseTime = sum / float64(len(s.responseTimes))
	}
}

func (s *ISPServer) getMetrics() plugin.MetricsPayload {
	s.metricsMu.RLock()
	defer s.metricsMu.RUnlock()
	
	uptime := int64(time.Since(s.startTime).Seconds())
	
	return plugin.MetricsPayload{
		DataPointsCollected: s.dataPointsCollected,
		ErrorsCount:         s.errorsCount,
		ConnectionUptime:    uptime,
		LastError:           s.lastError,
		AverageResponseTime: s.avgResponseTime,
		StartTime:           s.startTime.UnixNano(),
		LastDataTime:        time.Now().UnixNano(),
	}
}
