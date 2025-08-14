package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

func init() {
	// 注册连接器工厂
	northbound.Register("websocket", func() northbound.Sink {
		return NewWebSocketSink()
	})
}

// NewWebSocketSink 创建一个新的WebSocket连接器
func NewWebSocketSink() *WebSocketSink {
	return &WebSocketSink{
		BaseSink:   northbound.NewBaseSink("websocket"),
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// WebSocketSink 是一个WebSocket连接器，用于将数据点实时推送到WebSocket客户端
type WebSocketSink struct {
	*northbound.BaseSink
	server       *http.Server
	clients      map[*websocket.Conn]bool
	pointsConfig map[string]PointConfig
	broadcast    chan []byte
	register     chan *websocket.Conn
	unregister   chan *websocket.Conn
	upgrader     websocket.Upgrader
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	mu           sync.Mutex
}

// PointConfig 定义了数据点的WebSocket配置
type PointConfig struct {
	Topic       string            `json:"topic"`        // WebSocket主题
	Format      string            `json:"format"`       // 消息格式: full, value_only
	Transform   string            `json:"transform"`    // 转换函数: none, scale
	ScaleFactor float64           `json:"scale_factor"` // 缩放因子（用于scale转换）
	Tags        map[string]string `json:"tags"`         // 附加标签
}

// WebSocketConfig 是WebSocket连接器的特定参数配置
type WebSocketConfig struct {
	Address      string                 `json:"address"`          // 监听地址，如 ":8080"
	Path         string                 `json:"path"`             // WebSocket路径，如 "/ws"
	Points       map[string]PointConfig `json:"points"`           // 数据点配置
	AllowOrigins []string               `json:"allow_origins"`    // 允许的CORS源
}

// WebSocketMessage 是发送到客户端的消息格式
type WebSocketMessage struct {
	Topic     string                 `json:"topic"`
	Key       string                 `json:"key"`
	DeviceID  string                 `json:"device_id"`
	Value     interface{}            `json:"value"`
	Type      string                 `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Tags      map[string]string      `json:"tags,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// Init 初始化连接器
func (s *WebSocketSink) Init(cfg json.RawMessage) error {
	// 使用标准化配置解析
	standardConfig, err := s.ParseStandardConfig(cfg)
	if err != nil {
		return fmt.Errorf("解析WebSocket sink配置失败: %w", err)
	}

	// 解析WebSocket特定参数
	var wsConfig WebSocketConfig
	if err := json.Unmarshal(standardConfig.Params, &wsConfig); err != nil {
		return fmt.Errorf("解析WebSocket特定参数失败: %w", err)
	}

	s.pointsConfig = wsConfig.Points

	// 配置WebSocket升级器
	s.upgrader.CheckOrigin = func(r *http.Request) bool {
		// 如果未指定允许的源，则允许所有源
		if len(wsConfig.AllowOrigins) == 0 {
			return true
		}

		origin := r.Header.Get("Origin")
		for _, allowed := range wsConfig.AllowOrigins {
			if allowed == "*" || allowed == origin {
				return true
			}
		}
		return false
	}

	// 设置HTTP处理程序
	path := wsConfig.Path
	if path == "" {
		path = "/ws"
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, s.handleWebSocket)

	// 创建HTTP服务器
	address := wsConfig.Address
	if address == "" {
		address = ":8080"
	}

	s.server = &http.Server{
		Addr:    address,
		Handler: mux,
	}

	log.Info().
		Str("name", s.Name()).
		Str("address", address).
		Str("path", path).
		Int("points_config", len(s.pointsConfig)).
		Int("batch_size", s.GetBatchSize()).
		Int("buffer_size", s.GetBufferSize()).
		Msg("WebSocket连接器初始化完成")

	return nil
}

// Start 启动连接器
func (s *WebSocketSink) Start(ctx context.Context) error {
	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 启动广播协程
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.broadcastLoop()
	}()

	// 启动HTTP服务器
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		log.Info().
			Str("name", s.Name()).
			Str("address", s.server.Addr).
			Msg("WebSocket服务器启动")

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.HandleError(err, "WebSocket服务器运行")
		}
	}()

	// 监听上下文取消
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.ctx.Done()

		// 关闭HTTP服务器
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.HandleError(err, "关闭WebSocket服务器")
		}

		// 关闭所有客户端连接
		s.mu.Lock()
		for client := range s.clients {
			client.Close()
		}
		s.mu.Unlock()

		log.Info().Str("name", s.Name()).Msg("WebSocket连接器上下文取消")
	}()

	log.Info().Str("name", s.Name()).Msg("WebSocket连接器启动")
	return nil
}

// handleWebSocket 处理WebSocket连接
func (s *WebSocketSink) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.HandleError(err, "WebSocket升级")
		return
	}

	// 注册新客户端
	s.register <- conn

	log.Info().
		Str("name", s.Name()).
		Str("remote", conn.RemoteAddr().String()).
		Msg("WebSocket客户端连接")

	// 处理客户端消息
	go func() {
		defer func() {
			s.unregister <- conn
			conn.Close()
		}()

		for {
			// 读取客户端消息（目前仅用于保持连接活跃）
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					s.HandleError(err, "WebSocket读取")
				}
				break
			}
		}
	}()
}

// broadcastLoop 处理客户端注册、注销和广播消息
func (s *WebSocketSink) broadcastLoop() {
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = true
			clientCount := len(s.clients)
			s.mu.Unlock()
			log.Debug().
				Str("name", s.Name()).
				Str("remote", client.RemoteAddr().String()).
				Int("total", clientCount).
				Msg("WebSocket客户端注册")

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				clientCount := len(s.clients)
				s.mu.Unlock()
				log.Debug().
					Str("name", s.Name()).
					Str("remote", client.RemoteAddr().String()).
					Int("total", clientCount).
					Msg("WebSocket客户端注销")
			} else {
				s.mu.Unlock()
			}

		case message := <-s.broadcast:
			// 向所有客户端广播消息
			s.mu.Lock()
			for client := range s.clients {
				w, err := client.NextWriter(websocket.TextMessage)
				if err != nil {
					s.HandleError(err, "WebSocket写入器")
					client.Close()
					delete(s.clients, client)
					continue
				}
				w.Write(message)
				if err := w.Close(); err != nil {
					s.HandleError(err, "关闭WebSocket写入器")
					client.Close()
					delete(s.clients, client)
				}
			}
			s.mu.Unlock()

		case <-s.ctx.Done():
			return
		}
	}
}

// Publish 发布数据点到WebSocket客户端
func (s *WebSocketSink) Publish(batch []model.Point) error {
	if !s.IsRunning() {
		return fmt.Errorf("WebSocket连接器未启动")
	}

	if len(batch) == 0 {
		return nil
	}

	// 记录发布操作开始时间
	publishStart := time.Now()

	// 使用BaseSink的SafePublishBatch方法，自动处理统计
	return s.SafePublishBatch(batch, func(batch []model.Point) error {
		// 使用基础方法添加标签
		s.AddTags(batch)

		// 遍历数据点批次
		for _, point := range batch {
			// 构建WebSocket消息
			var topic string

			// 检查是否有针对该数据点的特殊配置
			config, hasConfig := s.pointsConfig[point.Key]
			if hasConfig && config.Topic != "" {
				// 优先使用配置的主题
				topic = config.Topic
			} else {
				// 使用默认主题格式
				topic = fmt.Sprintf("iot.data.%s.%s", point.DeviceID, point.Key)
			}

			// 创建安全的Tags副本 - 使用GetTagsCopy()
			safeTags := point.GetTagsCopy()

			// 构建消息
			msg := WebSocketMessage{
				Topic:     topic,
				Key:       point.Key,
				DeviceID:  point.DeviceID,
				Value:     s.convertValue(point, config),
				Type:      string(point.Type),
				Timestamp: point.Timestamp.UnixNano() / 1e6, // 转换为毫秒
				Tags:      safeTags,
			}

			// 序列化消息
			data, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("序列化WebSocket消息失败: %w", err)
			}

			// 发送到广播通道
			select {
			case s.broadcast <- data:
				log.Debug().
					Str("name", s.Name()).
					Str("topic", topic).
					Str("device_id", point.DeviceID).
					Str("key", point.Key).
					Msg("发布数据点到WebSocket")
			default:
				return fmt.Errorf("广播通道已满")
			}
		}

		return nil
	}, publishStart)
}

// convertValue 根据配置转换值
func (s *WebSocketSink) convertValue(point model.Point, config PointConfig) interface{} {
	// 首先根据数据类型转换值
	var value interface{}
	switch point.Type {
	case model.TypeInt:
		// 确保整数类型正确
		switch v := point.Value.(type) {
		case int:
			value = v
		case float64:
			value = int(v)
		default:
			value = 0
		}
	case model.TypeFloat:
		// 确保浮点类型正确
		switch v := point.Value.(type) {
		case float64:
			value = v
		case int:
			value = float64(v)
		default:
			value = 0.0
		}
	case model.TypeBool:
		// 确保布尔类型正确
		if v, ok := point.Value.(bool); ok {
			value = v
		} else {
			value = false
		}
	case model.TypeString:
		// 确保字符串类型正确
		if v, ok := point.Value.(string); ok {
			value = v
		} else {
			value = fmt.Sprintf("%v", point.Value)
		}
	default:
		// 默认作为字符串处理
		value = fmt.Sprintf("%v", point.Value)
	}

	// 然后根据转换函数进一步处理
	if config.Transform == "scale" && config.ScaleFactor != 0 {
		switch v := value.(type) {
		case int:
			return float64(v) * config.ScaleFactor
		case float64:
			return v * config.ScaleFactor
		}
	}

	return value
}

// Stop 停止连接器
func (s *WebSocketSink) Stop() error {
	s.SetRunning(false)

	if s.cancel != nil {
		s.cancel()
	}

	// 等待所有协程完成
	s.wg.Wait()

	log.Info().Str("name", s.Name()).Msg("WebSocket连接器停止")
	return nil
}

// Healthy 检查连接器健康状态
func (s *WebSocketSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("WebSocket连接器未运行")
	}
	if s.server == nil {
		return fmt.Errorf("WebSocket服务器未初始化")
	}
	return nil
}