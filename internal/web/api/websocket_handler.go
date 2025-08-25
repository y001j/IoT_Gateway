package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// SmartThrottler 智能限流器
type SmartThrottler struct {
	// 64-bit fields first for ARM32 alignment
	enabled          int64              // 是否启用推送 (0=暂停, 1=启用)
	pushCount       int64                    // 推送计数
	droppedCount    int64                    // 丢弃计数
	// Other fields
	globalRate       time.Duration      // 全局推送间隔
	sensorRates      map[string]time.Duration // 每个传感器的推送间隔
	lastPushTimes    map[string]time.Time     // 每个传感器的最后推送时间
	mutex           sync.RWMutex
	messageBuffer   chan ThrottledMessage    // 消息缓冲区
	statsMutex      sync.RWMutex
}

// ThrottledMessage 限流消息
type ThrottledMessage struct {
	MessageType string
	Data        interface{}
	SensorID    string
	Timestamp   time.Time
	Priority    int // 0=低, 1=中, 2=高
}

// WebSocketHandler handles WebSocket connections for real-time communication
type WebSocketHandler struct {
	services     *services.Services
	upgrader     websocket.Upgrader
	clients      map[string]*WebSocketClient
	mutex        sync.RWMutex
	natsConn     *nats.Conn
	// 智能限流器
	throttler    *SmartThrottler
	// 配置参数
	config          *models.WebSocketConfig
	connectionStats map[string]int // 连接统计
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	// 64-bit fields first for ARM32 alignment
	messageCount    int64     // 消息计数（改为int64支持原子操作）
	pushEnabled     int64     // 是否启用推送 (0=暂停, 1=启用)
	// Other fields
	ID       string
	Conn     *websocket.Conn
	UserID   string
	Username string
	Role     string
	Send     chan []byte
	Hub      *WebSocketHandler
	// 客户端管理字段
	lastActivity    time.Time // 最后活动时间
	createdAt       time.Time // 创建时间
	// 客户端推送控制
	subscriptions   map[string]bool // 订阅的数据类型
	subsMutex       sync.RWMutex    // 订阅锁
}

// WebSocketMessage represents a WebSocket message
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
	Source    string      `json:"source,omitempty"`
}

// RealtimeSystemData represents real-time system data
type RealtimeSystemData struct {
	SystemStatus  interface{} `json:"system_status,omitempty"`
	SystemMetrics interface{} `json:"system_metrics,omitempty"`
	PluginStatus  interface{} `json:"plugin_status,omitempty"`
	AlertData     interface{} `json:"alert_data,omitempty"`
}

// NewSmartThrottler 创建智能限流器
func NewSmartThrottler() *SmartThrottler {
	return &SmartThrottler{
		enabled:       1, // 默认启用
		globalRate:    50 * time.Millisecond, // 全局最小间隔50ms（每秒20条）
		sensorRates:   make(map[string]time.Duration),
		lastPushTimes: make(map[string]time.Time),
		messageBuffer: make(chan ThrottledMessage, 1000), // 缓冲区1000条消息
		pushCount:     0,
		droppedCount:  0,
	}
}

// SetGlobalEnabled 设置全局推送开关
func (t *SmartThrottler) SetGlobalEnabled(enabled bool) {
	if enabled {
		atomic.StoreInt64(&t.enabled, 1)
	} else {
		atomic.StoreInt64(&t.enabled, 0)
	}
}

// IsGlobalEnabled 检查全局推送是否启用
func (t *SmartThrottler) IsGlobalEnabled() bool {
	return atomic.LoadInt64(&t.enabled) == 1
}

// SetSensorRate 设置特定传感器的推送频率
func (t *SmartThrottler) SetSensorRate(sensorID string, rate time.Duration) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.sensorRates[sensorID] = rate
}

// ShouldPush 检查是否应该推送消息
func (t *SmartThrottler) ShouldPush(sensorID string, priority int) bool {
	// 检查全局开关
	if !t.IsGlobalEnabled() {
		atomic.AddInt64(&t.droppedCount, 1)
		return false
	}
	
	t.mutex.Lock()
	defer t.mutex.Unlock()
	
	now := time.Now()
	
	// 高优先级消息总是通过
	if priority >= 2 {
		t.lastPushTimes[sensorID] = now
		atomic.AddInt64(&t.pushCount, 1)
		return true
	}
	
	// 检查传感器特定频率限制
	if rate, exists := t.sensorRates[sensorID]; exists {
		if lastTime, hasLast := t.lastPushTimes[sensorID]; hasLast {
			if now.Sub(lastTime) < rate {
				atomic.AddInt64(&t.droppedCount, 1)
				return false
			}
		}
	} else {
		// 检查全局频率限制
		if lastTime, hasLast := t.lastPushTimes["_global"]; hasLast {
			if now.Sub(lastTime) < t.globalRate {
				atomic.AddInt64(&t.droppedCount, 1)
				return false
			}
		}
		t.lastPushTimes["_global"] = now
	}
	
	t.lastPushTimes[sensorID] = now
	atomic.AddInt64(&t.pushCount, 1)
	return true
}

// GetStats 获取限流统计信息
func (t *SmartThrottler) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":         t.IsGlobalEnabled(),
		"push_count":      atomic.LoadInt64(&t.pushCount),
		"dropped_count":   atomic.LoadInt64(&t.droppedCount),
		"buffer_size":     len(t.messageBuffer),
		"buffer_capacity": cap(t.messageBuffer),
	}
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(services *services.Services, natsConn *nats.Conn) *WebSocketHandler {
	// 获取WebSocket配置
	config := getWebSocketConfig(services)
	
	// 创建智能限流器（不配置传感器特定频率，使用通用策略）
	throttler := NewSmartThrottler()
	
	handler := &WebSocketHandler{
		services: services,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// 在生产环境中，应该检查 Origin
				return true
			},
			ReadBufferSize:  config.ReadBufferSize,
			WriteBufferSize: config.WriteBufferSize,
		},
		clients:         make(map[string]*WebSocketClient),
		natsConn:        natsConn,
		throttler:       throttler,
		config:          config,
		connectionStats: make(map[string]int),
	}

	
	// 启动 NATS 数据订阅
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("panic", r).Msg("WebSocket处理器NATS订阅Goroutine崩溃")
			}
		}()
		handler.subscribeToNATSData()
	}()

	// 启动系统数据推送
	go handler.pushSystemData()

	// 启动连接清理任务
	go handler.connectionCleanup()

	return handler
}

// getWebSocketConfig 获取WebSocket配置，如果配置不存在则返回默认值
func getWebSocketConfig(services *services.Services) *models.WebSocketConfig {
	// 尝试从系统配置获取WebSocket配置
	if systemConfig, err := services.System.GetConfig(); err == nil {
		wsConfig := &systemConfig.WebUI.WebSocket
		
		// 设置默认值
		if wsConfig.MaxClients <= 0 {
			wsConfig.MaxClients = 50
		}
		if wsConfig.MessageRate <= 0 {
			wsConfig.MessageRate = 10
		}
		if wsConfig.ReadBufferSize <= 0 {
			wsConfig.ReadBufferSize = 2048
		}
		if wsConfig.WriteBufferSize <= 0 {
			wsConfig.WriteBufferSize = 2048
		}
		if wsConfig.CleanupInterval <= 0 {
			wsConfig.CleanupInterval = 30
		}
		if wsConfig.InactivityTimeout <= 0 {
			wsConfig.InactivityTimeout = 300
		}
		if wsConfig.PingInterval <= 0 {
			wsConfig.PingInterval = 54
		}
		if wsConfig.PongTimeout <= 0 {
			wsConfig.PongTimeout = 60
		}
		
		log.Info().
			Int("max_clients", wsConfig.MaxClients).
			Int("message_rate", wsConfig.MessageRate).
			Int("read_buffer_size", wsConfig.ReadBufferSize).
			Int("write_buffer_size", wsConfig.WriteBufferSize).
			Msg("WebSocket配置已加载")
		
		return wsConfig
	}
	
	// 使用默认配置
	log.Warn().Msg("无法获取WebSocket配置，使用默认值")
	return &models.WebSocketConfig{
		MaxClients:         50,
		MessageRate:        10,
		ReadBufferSize:     2048,
		WriteBufferSize:    2048,
		CleanupInterval:    30,
		InactivityTimeout:  300,
		PingInterval:       54,
		PongTimeout:        60,
	}
}

// HandleWebSocket handles WebSocket connection upgrade
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// 获取用户信息（从JWT中间件设置）
	userInterface, exists := c.Get("user")
	if !exists {
		log.Error().Msg("未找到用户信息")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	user, ok := userInterface.(*models.User)
	if !ok {
		log.Error().Msg("用户信息格式错误")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "内部错误"})
		return
	}

	// 升级HTTP连接为WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("WebSocket升级失败")
		return
	}

	// 创建客户端
	client := &WebSocketClient{
		ID:            generateClientID(),
		Conn:          conn,
		UserID:        strconv.Itoa(user.ID),
		Username:      user.Username,
		Role:          user.Role,
		Send:          make(chan []byte, 256),
		Hub:           h,
		pushEnabled:   1, // 默认启用推送
		subscriptions: make(map[string]bool),
	}
	
	// 默认订阅所有数据类型
	client.subscriptions["iot_data"] = true
	client.subscriptions["rule_event"] = true
	client.subscriptions["system_event"] = true

	// 注册客户端
	h.registerClient(client)

	// 发送欢迎消息和初始数据
	go func() {
		time.Sleep(100 * time.Millisecond) // 等待连接稳定
		h.sendWelcomeMessage(client)
		h.sendInitialData(client)
	}()

	// 启动客户端的读写协程
	go client.writePump()
	go client.readPump()

	log.Info().
		Str("client_id", client.ID).
		Str("username", client.Username).
		Msg("WebSocket客户端连接成功")
}

// registerClient registers a new WebSocket client with connection limits
func (h *WebSocketHandler) registerClient(client *WebSocketClient) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	// 检查连接数限制
	if len(h.clients) >= h.config.MaxClients {
		log.Warn().Int("current_clients", len(h.clients)).Int("max_clients", h.config.MaxClients).Msg("达到最大连接数限制，拒绝新连接")
		client.Conn.Close()
		return
	}

	client.createdAt = time.Now()
	client.lastActivity = time.Now()
	h.clients[client.ID] = client

	// 更新连接统计
	h.connectionStats[client.UserID]++

	log.Info().
		Str("client_id", client.ID).
		Str("username", client.Username).
		Int("total_clients", len(h.clients)).
		Msg("新WebSocket客户端已注册")
}

// unregisterClient removes a WebSocket client
func (h *WebSocketHandler) unregisterClient(client *WebSocketClient) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, exists := h.clients[client.ID]; exists {
		delete(h.clients, client.ID)
		close(client.Send)

		// 更新连接统计
		if h.connectionStats[client.UserID] > 0 {
			h.connectionStats[client.UserID]--
		}

		log.Info().
			Str("client_id", client.ID).
			Str("username", client.Username).
			Int("total_clients", len(h.clients)).
			Msg("WebSocket客户端已注销")
	}
}

// broadcastMessage sends a message to all connected clients with rate limiting
func (h *WebSocketHandler) broadcastMessage(messageType string, data interface{}) {
	message := WebSocketMessage{
		Type:      messageType,
		Timestamp: time.Now(),
		Data:      data,
		Source:    "system",
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Msg("序列化WebSocket消息失败")
		return
	}

	h.mutex.RLock()
	clients := make([]*WebSocketClient, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.mutex.RUnlock()

	// 异步发送消息，避免阻塞
	go func() {
		for _, client := range clients {
			// 检查客户端状态
			if client.Conn == nil {
				continue
			}

			// 速率限制：检查客户端消息频率
			if atomic.LoadInt64(&client.messageCount) > int64(h.config.MessageRate) {
				log.Warn().Str("client_id", client.ID).Int64("message_count", atomic.LoadInt64(&client.messageCount)).Int("message_rate", h.config.MessageRate).Msg("客户端消息频率过高，跳过发送")
				continue
			}

			select {
			case client.Send <- messageBytes:
				atomic.AddInt64(&client.messageCount, 1)
				client.lastActivity = time.Now()
			default:
				// 发送缓冲区已满，记录警告但不立即断开连接
				log.Warn().Str("client_id", client.ID).Msg("客户端发送缓冲区已满，跳过消息")
			}
		}
	}()
}

// sendToClient sends a message to a specific client
func (h *WebSocketHandler) sendToClient(clientID string, messageType string, data interface{}) {
	message := WebSocketMessage{
		Type:      messageType,
		Timestamp: time.Now(),
		Data:      data,
		Source:    "system",
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Msg("序列化WebSocket消息失败")
		return
	}

	h.mutex.RLock()
	client, exists := h.clients[clientID]
	h.mutex.RUnlock()

	if exists {
		select {
		case client.Send <- messageBytes:
		default:
			// 发送失败
			log.Warn().Str("client_id", clientID).Msg("向客户端发送消息失败")
		}
	}
}

// sendWelcomeMessage sends welcome message to newly connected client
func (h *WebSocketHandler) sendWelcomeMessage(client *WebSocketClient) {
	welcomeData := map[string]interface{}{
		"message":     "欢迎连接到IoT网关实时数据服务",
		"client_id":   client.ID,
		"username":    client.Username,
		"server_time": time.Now(),
	}

	h.sendToClient(client.ID, "welcome", welcomeData)
}

// sendInitialData sends initial system data to newly connected client
func (h *WebSocketHandler) sendInitialData(client *WebSocketClient) {
	// 发送系统状态
	if status, err := h.services.System.GetStatus(); err == nil {
		h.sendToClient(client.ID, "system_status", status)
	}

	// 发送系统指标
	if metrics, err := h.services.System.GetMetrics(); err == nil {
		h.sendToClient(client.ID, "system_metrics", metrics)
	}

	// 发送插件状态
	req := &models.PluginListRequest{
		Page:     1,
		PageSize: 100,
		Type:     "",
		Status:   "",
		Search:   "",
	}
	if plugins, _, err := h.services.Plugin.GetPlugins(req); err == nil {
		h.sendToClient(client.ID, "plugin_status", plugins)
	}

	// 发送健康检查信息
	if health, err := h.services.System.GetHealth(); err == nil {
		h.sendToClient(client.ID, "health_check", health)
	}
}

// subscribeToNATSData subscribes to NATS data streams with enhanced rate limiting
func (h *WebSocketHandler) subscribeToNATSData() {
	if h.natsConn == nil {
		log.Warn().Msg("NATS连接为空，无法订阅实时数据")
		return
	}

	
	// 检查NATS连接状态
	if h.natsConn.Status() != nats.CONNECTED {
		log.Error().Str("status", h.natsConn.Status().String()).Msg("NATS连接状态异常")
		return
	}
	

	// 订阅IoT数据流，使用智能限流器
	_, err := h.natsConn.Subscribe("iot.data.>", func(msg *nats.Msg) {
		log.Debug().Str("subject", msg.Subject).Msg("WebSocket处理器收到IoT数据消息")
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			// 检查是否有客户端连接
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			if clientCount == 0 {
				return // 没有客户端，跳过处理
			}

			// 提取传感器ID
			sensorID := "unknown"
			if deviceID, ok := data["device_id"].(string); ok {
				sensorID = deviceID
			}

			// 设置消息优先级
			priority := 1 // 默认普通优先级
			if strings.Contains(msg.Subject, "alert") || strings.Contains(msg.Subject, "alarm") {
				priority = 2 // 高优先级
			}

			// 使用智能限流器检查是否应该推送
			if h.throttler.ShouldPush(sensorID, priority) {
				// 增强数据格式，包含类型信息和派生值
				enhancedData := h.enhanceIoTData(data)
				h.smartBroadcastMessage("iot_data", map[string]interface{}{
					"subject": msg.Subject,
					"data":    enhancedData,
				})
			}
		}
	})
	
	if err != nil {
		log.Error().Err(err).Msg("订阅IoT数据流失败")
		return
	}
	

	// 订阅规则引擎事件
	h.natsConn.Subscribe("iot.rules.>", func(msg *nats.Msg) {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			if clientCount > 0 {
				// 规则事件优先级较低，使用全局限流
				if h.throttler.ShouldPush("rule_engine", 0) {
					h.smartBroadcastMessage("rule_event", map[string]interface{}{
						"subject": msg.Subject,
						"data":    data,
					})
				}
			}
		}
	})

	// 订阅系统事件
	h.natsConn.Subscribe("iot.system.>", func(msg *nats.Msg) {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			if clientCount > 0 {
				h.broadcastMessage("system_event", map[string]interface{}{
					"subject": msg.Subject,
					"data":    data,
				})
			}
		}
	})

}

// smartBroadcastMessage 智能广播消息，支持客户端订阅管理和推送控制
func (h *WebSocketHandler) smartBroadcastMessage(messageType string, data interface{}) {
	message := WebSocketMessage{
		Type:      messageType,
		Timestamp: time.Now(),
		Data:      data,
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Error().Err(err).Msg("消息序列化失败")
		return
	}

	h.mutex.RLock()
	clients := make([]*WebSocketClient, 0, len(h.clients))
	for _, client := range h.clients {
		clients = append(clients, client)
	}
	h.mutex.RUnlock()

	for _, client := range clients {
		// 检查客户端推送是否启用
		if atomic.LoadInt64(&client.pushEnabled) == 0 {
			continue // 客户端暂停推送
		}

		// 检查客户端是否订阅了此类型的数据
		client.subsMutex.RLock()
		shouldSend := false
		if len(client.subscriptions) == 0 {
			// 没有订阅，默认发送（向后兼容）
			shouldSend = true
		} else {
			// 检查是否订阅了此类型
			for sub := range client.subscriptions {
				if sub == "all" || sub == messageType {
					shouldSend = true
					break
				}
				// 检查IoT数据的模式匹配
				if messageType == "iot_data" && (sub == "iot.data.*" || strings.HasPrefix(sub, "iot.data.")) {
					shouldSend = true
					break
				}
			}
		}
		client.subsMutex.RUnlock()
		
		if !shouldSend {
			continue
		}

		// 非阻塞发送
		select {
		case client.Send <- messageBytes:
			// 更新客户端统计
			atomic.AddInt64(&client.messageCount, 1)
			client.lastActivity = time.Now()
		default:
			// 客户端缓冲区满，记录并跳过
			log.Warn().
				Str("client_id", client.ID).
				Str("message_type", messageType).
				Msg("客户端缓冲区满，跳过消息")
		}
	}
}

// pushSystemData periodically pushes system data to all clients
func (h *WebSocketHandler) pushSystemData() {
	ticker := time.NewTicker(10 * time.Second) // 每10秒推送一次
	defer ticker.Stop()

	for range ticker.C {
		h.mutex.RLock()
		clientCount := len(h.clients)
		h.mutex.RUnlock()

		if clientCount == 0 {
			continue // 没有客户端连接，跳过
		}

		// 获取并推送系统状态
		if status, err := h.services.System.GetStatus(); err == nil {
			h.broadcastMessage("system_status_update", status)
		}

		// 获取并推送系统指标
		if metrics, err := h.services.System.GetMetrics(); err == nil {
			h.broadcastMessage("system_metrics_update", metrics)
		}
	}
}

// connectionCleanup periodically cleans up inactive connections
func (h *WebSocketHandler) connectionCleanup() {
	ticker := time.NewTicker(time.Duration(h.config.CleanupInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.cleanupInactiveClients()
			h.resetMessageCounters()
		}
	}
}

// cleanupInactiveClients removes inactive or stale connections
func (h *WebSocketHandler) cleanupInactiveClients() {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	now := time.Now()
	inactiveClients := make([]*WebSocketClient, 0)

	for _, client := range h.clients {
		// 检查客户端是否超时
		if now.Sub(client.lastActivity) > time.Duration(h.config.InactivityTimeout)*time.Second {
			inactiveClients = append(inactiveClients, client)
		}
	}

	// 清理不活跃的客户端
	for _, client := range inactiveClients {
		log.Info().Str("client_id", client.ID).Msg("清理不活跃的WebSocket连接")
		client.Conn.Close()
		delete(h.clients, client.ID)
		close(client.Send)

		if h.connectionStats[client.UserID] > 0 {
			h.connectionStats[client.UserID]--
		}
	}

	if len(inactiveClients) > 0 {
		log.Info().Int("cleaned_clients", len(inactiveClients)).Int("remaining_clients", len(h.clients)).Msg("已清理不活跃连接")
	}
}

// resetMessageCounters resets message counters for rate limiting
func (h *WebSocketHandler) resetMessageCounters() {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for _, client := range h.clients {
		atomic.StoreInt64(&client.messageCount, 0)
	}
}

// readPump handles reading from the WebSocket connection
func (c *WebSocketClient) readPump() {
	defer func() {
		c.Hub.unregisterClient(c)
		c.Conn.Close()
	}()

	// 设置读取限制和超时 - 增加到8KB
	c.Conn.SetReadLimit(8192)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		c.lastActivity = time.Now() // 更新活动时间
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("client_id", c.ID).Msg("WebSocket读取错误")
			}
			break
		}

		c.lastActivity = time.Now() // 更新活动时间

		// 处理客户端消息
		c.handleClientMessage(message)
	}
}

// writePump handles writing messages to WebSocket client with improved error handling
func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(15 * time.Second)) // 增加写入超时
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 直接发送单条消息，避免批量发送导致的数据损坏
			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Error().Err(err).Str("client_id", c.ID).Msg("写入WebSocket消息失败")
				return
			}

			c.lastActivity = time.Now() // 更新活动时间

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Error().Err(err).Str("client_id", c.ID).Msg("发送心跳失败")
				return
			}
		}
	}
}

// handleClientMessage handles messages received from WebSocket client
func (c *WebSocketClient) handleClientMessage(message []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Error().Err(err).Str("client_id", c.ID).Msg("解析客户端消息失败")
		return
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		log.Warn().Str("client_id", c.ID).Msg("客户端消息缺少type字段")
		return
	}

	log.Debug().
		Str("client_id", c.ID).
		Str("message_type", msgType).
		Msg("收到客户端消息")

	// 根据消息类型处理
	switch msgType {
	case "ping":
		c.Hub.sendToClient(c.ID, "pong", map[string]interface{}{
			"timestamp": time.Now(),
		})
	case "subscribe":
		// 处理订阅请求
		c.handleSubscribeRequest(msg)
	case "unsubscribe":
		// 处理取消订阅请求
		c.handleUnsubscribeRequest(msg)
	default:
		log.Warn().
			Str("client_id", c.ID).
			Str("message_type", msgType).
			Msg("未知的客户端消息类型")
	}
}

// handleSubscribeRequest handles subscription requests from client
func (c *WebSocketClient) handleSubscribeRequest(msg map[string]interface{}) {
	log.Info().
		Str("client_id", c.ID).
		Interface("request", msg).
		Msg("处理订阅请求")
	
	// 解析订阅请求
	data, ok := msg["data"].(map[string]interface{})
	if !ok {
		log.Warn().Str("client_id", c.ID).Msg("订阅请求格式错误")
		return
	}
	
	channel, ok := data["channel"].(string)
	if !ok {
		log.Warn().Str("client_id", c.ID).Msg("订阅请求缺少channel字段")
		return
	}
	
	// 添加到客户端订阅列表
	c.subsMutex.Lock()
	if c.subscriptions == nil {
		c.subscriptions = make(map[string]bool)
	}
	c.subscriptions[channel] = true
	c.subsMutex.Unlock()
	
	// 启用客户端推送
	atomic.StoreInt64(&c.pushEnabled, 1)
	
	log.Info().
		Str("client_id", c.ID).
		Str("channel", channel).
		Msg("成功添加订阅")
	
	// 发送订阅确认
	c.Hub.sendToClient(c.ID, "subscription_confirmed", map[string]interface{}{
		"channel": channel,
		"status":  "active",
		"timestamp": time.Now(),
	})
}

// handleUnsubscribeRequest handles unsubscription requests from client
func (c *WebSocketClient) handleUnsubscribeRequest(msg map[string]interface{}) {
	// 实现取消订阅逻辑
	log.Info().
		Str("client_id", c.ID).
		Interface("request", msg).
		Msg("处理取消订阅请求")
}

// generateClientID generates a unique client ID
func generateClientID() string {
	return "client_" + time.Now().Format("20060102150405") + "_" + randomString(6)
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// enhanceIoTData 增强IoT数据，添加类型信息和派生值
func (h *WebSocketHandler) enhanceIoTData(data map[string]interface{}) map[string]interface{} {
	enhanced := make(map[string]interface{})
	
	// 复制原始数据
	for k, v := range data {
		enhanced[k] = v
	}
	
	// 确定数据类型并计算派生值
	dataType := h.detectDataType(data)
	enhanced["data_type"] = dataType
	
	// 根据数据类型计算派生值
	switch dataType {
	case "location":
		if derivedValues := h.calculateLocationDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "vector3d":
		if derivedValues := h.calculateVector3DDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "color":
		if derivedValues := h.calculateColorDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "vector":
		if derivedValues := h.calculateVectorDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "array":
		if derivedValues := h.calculateArrayDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "matrix":
		if derivedValues := h.calculateMatrixDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "timeseries":
		if derivedValues := h.calculateTimeSeriesDerivedValues(data); derivedValues != nil {
			enhanced["derived_values"] = derivedValues
		}
	case "float", "int":
		// 对于简单数值类型，添加显示值
		if value, ok := data["value"]; ok {
			enhanced["derived_values"] = map[string]interface{}{
				"display_value": value,
				"summary":       fmt.Sprintf("%.2f", toFloat64(value)),
			}
		}
	}
	
	return enhanced
}

// detectDataType 检测数据类型
func (h *WebSocketHandler) detectDataType(data map[string]interface{}) string {
	// 检查type字段
	if dataType, ok := data["type"].(string); ok {
		return dataType
	}
	
	// 根据value结构推断类型
	value, ok := data["value"]
	if !ok {
		return "unknown"
	}
	
	switch v := value.(type) {
	case map[string]interface{}:
		// 检查复合数据类型的特征字段
		if _, hasLat := v["latitude"]; hasLat {
			if _, hasLng := v["longitude"]; hasLng {
				return "location"
			}
		}
		if _, hasX := v["x"]; hasX {
			if _, hasY := v["y"]; hasY {
				if _, hasZ := v["z"]; hasZ {
					return "vector3d"
				}
			}
		}
		if _, hasR := v["r"]; hasR {
			if _, hasG := v["g"]; hasG {
				if _, hasB := v["b"]; hasB {
					return "color"
				}
			}
		}
		if _, hasValues := v["values"]; hasValues {
			if _, hasDim := v["dimension"]; hasDim {
				return "vector"
			}
			if _, hasRows := v["rows"]; hasRows {
				if _, hasCols := v["cols"]; hasCols {
					return "matrix"
				}
			}
		}
		if _, hasTimestamps := v["timestamps"]; hasTimestamps {
			return "timeseries"
		}
		return "object"
	case []interface{}:
		return "array"
	case float64, int, int64, float32:
		return "float"
	case bool:
		return "bool"
	case string:
		return "string"
	default:
		return "unknown"
	}
}

// calculateLocationDerivedValues 计算GPS位置派生值
func (h *WebSocketHandler) calculateLocationDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	lat := toFloat64(value["latitude"])
	lng := toFloat64(value["longitude"])
	
	derived := map[string]interface{}{
		"display_value": fmt.Sprintf("%.6f,%.6f", lat, lng),
		"summary":      fmt.Sprintf("GPS (%.4f, %.4f)", lat, lng),
		"coordinate":   fmt.Sprintf("%.6f,%.6f", lat, lng),
	}
	
	if speed := toFloat64(value["speed"]); speed > 0 {
		derived["speed_kmh"] = speed
		derived["moving"] = speed > 1.0
	}
	
	if altitude := toFloat64(value["altitude"]); altitude != 0 {
		derived["altitude_m"] = altitude
	}
	
	return derived
}

// calculateVector3DDerivedValues 计算3D向量派生值
func (h *WebSocketHandler) calculateVector3DDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	x := toFloat64(value["x"])
	y := toFloat64(value["y"])
	z := toFloat64(value["z"])
	
	magnitude := math.Sqrt(x*x + y*y + z*z)
	
	return map[string]interface{}{
		"display_value": magnitude,
		"summary":      fmt.Sprintf("Vector3D (|%.2f|)", magnitude),
		"magnitude":    magnitude,
		"components":   fmt.Sprintf("(%.2f, %.2f, %.2f)", x, y, z),
	}
}

// calculateColorDerivedValues 计算颜色派生值
func (h *WebSocketHandler) calculateColorDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	r := int(toFloat64(value["r"]))
	g := int(toFloat64(value["g"]))
	b := int(toFloat64(value["b"]))
	
	hexColor := fmt.Sprintf("#%02X%02X%02X", r, g, b)
	
	return map[string]interface{}{
		"display_value": hexColor,
		"summary":      fmt.Sprintf("Color %s", hexColor),
		"hex":          hexColor,
		"rgb":          fmt.Sprintf("rgb(%d, %d, %d)", r, g, b),
	}
}

// calculateVectorDerivedValues 计算通用向量派生值
func (h *WebSocketHandler) calculateVectorDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	valuesInterface, ok := value["values"].([]interface{})
	if !ok {
		return nil
	}
	
	values := make([]float64, len(valuesInterface))
	for i, v := range valuesInterface {
		values[i] = toFloat64(v)
	}
	
	// 计算模长
	sumSquares := 0.0
	for _, v := range values {
		sumSquares += v * v
	}
	magnitude := math.Sqrt(sumSquares)
	
	dimension := len(values)
	
	return map[string]interface{}{
		"display_value": magnitude,
		"summary":      fmt.Sprintf("Vector%dD (|%.2f|)", dimension, magnitude),
		"magnitude":    magnitude,
		"dimension":    dimension,
	}
}

// calculateArrayDerivedValues 计算数组派生值
func (h *WebSocketHandler) calculateArrayDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	valuesInterface, ok := value["values"].([]interface{})
	if !ok {
		return nil
	}
	
	size := len(valuesInterface)
	nullCount := 0
	numericCount := 0
	var sum float64
	
	for _, v := range valuesInterface {
		if v == nil {
			nullCount++
		} else if _, ok := v.(float64); ok {
			numericCount++
			sum += toFloat64(v)
		} else if _, ok := v.(int); ok {
			numericCount++
			sum += toFloat64(v)
		}
	}
	
	derived := map[string]interface{}{
		"display_value": size,
		"summary":      fmt.Sprintf("Array[%d]", size),
		"size":         size,
		"null_count":   nullCount,
	}
	
	if numericCount > 0 {
		derived["numeric_count"] = numericCount
		derived["average"] = sum / float64(numericCount)
	}
	
	return derived
}

// calculateMatrixDerivedValues 计算矩阵派生值
func (h *WebSocketHandler) calculateMatrixDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	rows := int(toFloat64(value["rows"]))
	cols := int(toFloat64(value["cols"]))
	
	return map[string]interface{}{
		"display_value": fmt.Sprintf("%dx%d", rows, cols),
		"summary":      fmt.Sprintf("Matrix %dx%d", rows, cols),
		"rows":         rows,
		"cols":         cols,
		"size":         rows * cols,
		"is_square":    rows == cols,
	}
}

// calculateTimeSeriesDerivedValues 计算时间序列派生值
func (h *WebSocketHandler) calculateTimeSeriesDerivedValues(data map[string]interface{}) map[string]interface{} {
	value, ok := data["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	
	valuesInterface, ok := value["values"].([]interface{})
	if !ok {
		return nil
	}
	
	length := len(valuesInterface)
	if length == 0 {
		return map[string]interface{}{
			"display_value": 0,
			"summary":      "TimeSeries (empty)",
			"length":       0,
		}
	}
	
	// 计算最新值
	latestValue := toFloat64(valuesInterface[length-1])
	
	derived := map[string]interface{}{
		"display_value": latestValue,
		"summary":      fmt.Sprintf("TimeSeries[%d] (%.2f)", length, latestValue),
		"length":       length,
		"latest_value": latestValue,
	}
	
	// 计算简单统计
	if length > 1 {
		firstValue := toFloat64(valuesInterface[0])
		change := latestValue - firstValue
		derived["total_change"] = change
		derived["trend"] = "stable"
		if change > 0.01 {
			derived["trend"] = "increasing"
		} else if change < -0.01 {
			derived["trend"] = "decreasing"
		}
	}
	
	return derived
}

// toFloat64 辅助函数，安全地转换为float64
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0.0
}

// SetGlobalPushEnabled 设置全局推送开关API
func (h *WebSocketHandler) SetGlobalPushEnabled(c *gin.Context) {
	var request struct {
		Enabled bool `json:"enabled"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}
	
	h.throttler.SetGlobalEnabled(request.Enabled)
	
	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("全局推送已%s", map[bool]string{true: "启用", false: "暂停"}[request.Enabled]),
		"enabled": request.Enabled,
		"stats":   h.throttler.GetStats(),
	})
}

// SetClientPushEnabled 设置指定客户端的推送开关API
func (h *WebSocketHandler) SetClientPushEnabled(c *gin.Context) {
	clientID := c.Param("clientId")
	var request struct {
		Enabled bool `json:"enabled"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}
	
	h.mutex.RLock()
	client, exists := h.clients[clientID]
	h.mutex.RUnlock()
	
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "客户端不存在"})
		return
	}
	
	if request.Enabled {
		atomic.StoreInt64(&client.pushEnabled, 1)
	} else {
		atomic.StoreInt64(&client.pushEnabled, 0)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message":   fmt.Sprintf("客户端%s推送已%s", clientID, map[bool]string{true: "启用", false: "暂停"}[request.Enabled]),
		"client_id": clientID,
		"enabled":   request.Enabled,
	})
}

// SetClientSubscriptions 设置客户端订阅API
func (h *WebSocketHandler) SetClientSubscriptions(c *gin.Context) {
	clientID := c.Param("clientId")
	var request struct {
		Subscriptions []string `json:"subscriptions"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求参数"})
		return
	}
	
	h.mutex.RLock()
	client, exists := h.clients[clientID]
	h.mutex.RUnlock()
	
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "客户端不存在"})
		return
	}
	
	client.subsMutex.Lock()
	client.subscriptions = make(map[string]bool)
	for _, sub := range request.Subscriptions {
		client.subscriptions[sub] = true
	}
	client.subsMutex.Unlock()
	
	c.JSON(http.StatusOK, gin.H{
		"message":       fmt.Sprintf("客户端%s订阅已更新", clientID),
		"client_id":     clientID,
		"subscriptions": request.Subscriptions,
	})
}

// GetWebSocketStats 获取WebSocket统计信息API
func (h *WebSocketHandler) GetWebSocketStats(c *gin.Context) {
	h.mutex.RLock()
	clientStats := make([]map[string]interface{}, 0, len(h.clients))
	for _, client := range h.clients {
		client.subsMutex.RLock()
		subscriptions := make([]string, 0, len(client.subscriptions))
		for sub := range client.subscriptions {
			subscriptions = append(subscriptions, sub)
		}
		client.subsMutex.RUnlock()
		
		clientStats = append(clientStats, map[string]interface{}{
			"client_id":      client.ID,
			"username":       client.Username,
			"role":           client.Role,
			"push_enabled":   atomic.LoadInt64(&client.pushEnabled) == 1,
			"message_count":  atomic.LoadInt64(&client.messageCount),
			"last_activity":  client.lastActivity,
			"created_at":     client.createdAt,
			"subscriptions":  subscriptions,
		})
	}
	h.mutex.RUnlock()
	
	c.JSON(http.StatusOK, gin.H{
		"throttler_stats": h.throttler.GetStats(),
		"client_count":    len(clientStats),
		"clients":         clientStats,
		"config": map[string]interface{}{
			"max_clients":         h.config.MaxClients,
			"message_rate":        h.config.MessageRate,
			"read_buffer_size":    h.config.ReadBufferSize,
			"write_buffer_size":   h.config.WriteBufferSize,
			"cleanup_interval":    h.config.CleanupInterval,
			"inactivity_timeout":  h.config.InactivityTimeout,
			"ping_interval":       h.config.PingInterval,
			"pong_timeout":        h.config.PongTimeout,
		},
	})
}
