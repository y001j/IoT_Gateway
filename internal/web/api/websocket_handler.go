package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/web/models"
	"github.com/y001j/iot-gateway/internal/web/services"
)

// WebSocketHandler handles WebSocket connections for real-time communication
type WebSocketHandler struct {
	services     *services.Services
	upgrader     websocket.Upgrader
	clients      map[string]*WebSocketClient
	mutex        sync.RWMutex
	natsConn     *nats.Conn
	lastIOTData  time.Time  // 最后一次IoT数据推送时间
	iotDataMutex sync.Mutex // IoT数据推送互斥锁
	// 配置参数
	config          *models.WebSocketConfig
	connectionStats map[string]int // 连接统计
}

// WebSocketClient represents a connected WebSocket client
type WebSocketClient struct {
	ID       string
	Conn     *websocket.Conn
	UserID   string
	Username string
	Role     string
	Send     chan []byte
	Hub      *WebSocketHandler
	// 新增客户端管理字段
	lastActivity time.Time // 最后活动时间
	messageCount int       // 消息计数
	createdAt    time.Time // 创建时间
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

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(services *services.Services, natsConn *nats.Conn) *WebSocketHandler {
	// 获取WebSocket配置
	config := getWebSocketConfig(services)
	
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
		config:          config,
		connectionStats: make(map[string]int),
	}

	// 启动 NATS 数据订阅
	go handler.subscribeToNATSData()

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
		ID:       generateClientID(),
		Conn:     conn,
		UserID:   strconv.Itoa(user.ID),
		Username: user.Username,
		Role:     user.Role,
		Send:     make(chan []byte, 256),
		Hub:      h,
	}

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
			if client.messageCount > h.config.MessageRate {
				log.Warn().Str("client_id", client.ID).Int("message_count", client.messageCount).Int("message_rate", h.config.MessageRate).Msg("客户端消息频率过高，跳过发送")
				continue
			}

			select {
			case client.Send <- messageBytes:
				client.messageCount++
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

	// 订阅IoT数据流，使用更严格的频率限制
	h.natsConn.Subscribe("iot.data.>", func(msg *nats.Msg) {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			// 检查是否是高频振动数据
			isHighFreqData := strings.Contains(msg.Subject, "vibration_sensor_01")

			if isHighFreqData {
				// 高频数据限制：每500ms最多推送一次（降低频率）
				h.iotDataMutex.Lock()
				now := time.Now()
				if now.Sub(h.lastIOTData) < 500*time.Millisecond {
					h.iotDataMutex.Unlock()
					return // 跳过此次推送
				}
				h.lastIOTData = now
				h.iotDataMutex.Unlock()
			}

			// 只有在有客户端连接时才广播消息
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			if clientCount > 0 {
				h.broadcastMessage("iot_data", map[string]interface{}{
					"subject": msg.Subject,
					"data":    data,
				})
			}
		}
	})

	// 订阅规则引擎事件
	h.natsConn.Subscribe("iot.rules.>", func(msg *nats.Msg) {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data, &data); err == nil {
			h.mutex.RLock()
			clientCount := len(h.clients)
			h.mutex.RUnlock()

			if clientCount > 0 {
				h.broadcastMessage("rule_event", map[string]interface{}{
					"subject": msg.Subject,
					"data":    data,
				})
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

	log.Info().Msg("NATS数据流订阅已启动")
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
		client.messageCount = 0
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
	// 实现订阅逻辑，例如订阅特定的数据流
	// 这里可以根据需要扩展
	log.Info().
		Str("client_id", c.ID).
		Interface("request", msg).
		Msg("处理订阅请求")
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
