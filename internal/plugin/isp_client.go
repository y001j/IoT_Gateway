package plugin

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ISPClient ISP协议客户端
type ISPClient struct {
	address     string
	conn        net.Conn
	scanner     *bufio.Scanner
	writer      *bufio.Writer
	mu          sync.Mutex
	connected   bool
	responses   map[string]chan *ISPMessage // 响应通道映射
	responseMu  sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	dataHandler func(*ISPMessage) // 数据消息处理器
	handlerMu   sync.Mutex
}

// NewISPClient 创建ISP客户端
func NewISPClient(address string) *ISPClient {
	return &ISPClient{
		address:   address,
		responses: make(map[string]chan *ISPMessage),
	}
}

// Connect 连接到ISP服务器
func (c *ISPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// 创建可取消的上下文
	c.ctx, c.cancel = context.WithCancel(ctx)

	// 建立TCP连接
	conn, err := net.DialTimeout("tcp", c.address, 10*time.Second)
	if err != nil {
		return fmt.Errorf("连接ISP服务器失败: %w", err)
	}

	c.conn = conn
	c.scanner = bufio.NewScanner(conn)
	c.writer = bufio.NewWriter(conn)
	c.connected = true

	log.Info().
		Str("address", c.address).
		Msg("ISP客户端连接成功")

	// 启动消息接收协程
	go c.receiveLoop()

	return nil
}

// Disconnect 断开连接
func (c *ISPClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false

	// 取消上下文
	if c.cancel != nil {
		c.cancel()
	}

	// 关闭连接
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// 清理响应通道
	c.responseMu.Lock()
	for id, ch := range c.responses {
		close(ch)
		delete(c.responses, id)
	}
	c.responseMu.Unlock()

	log.Info().
		Str("address", c.address).
		Msg("ISP客户端已断开连接")

	return nil
}

// IsConnected 检查连接状态
func (c *ISPClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// SendMessage 发送消息
func (c *ISPClient) SendMessage(msg *ISPMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return fmt.Errorf("ISP客户端未连接")
	}

	// 序列化消息
	data, err := msg.ToJSON()
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	// 发送消息（每行一个JSON）
	if _, err := c.writer.Write(data); err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}
	if _, err := c.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("发送换行符失败: %w", err)
	}
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("刷新缓冲区失败: %w", err)
	}

	log.Debug().
		Str("type", msg.Type).
		Str("id", msg.ID).
		Msg("发送ISP消息")

	return nil
}

// SendRequest 发送请求并等待响应
func (c *ISPClient) SendRequest(msg *ISPMessage, timeout time.Duration) (*ISPMessage, error) {
	if msg.ID == "" {
		return nil, fmt.Errorf("请求消息必须有ID")
	}

	// 创建响应通道
	respCh := make(chan *ISPMessage, 1)
	c.responseMu.Lock()
	c.responses[msg.ID] = respCh
	c.responseMu.Unlock()

	// 确保清理响应通道
	defer func() {
		c.responseMu.Lock()
		delete(c.responses, msg.ID)
		c.responseMu.Unlock()
		close(respCh)
	}()

	// 发送消息
	if err := c.SendMessage(msg); err != nil {
		return nil, err
	}

	// 等待响应
	select {
	case resp := <-respCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("等待响应超时")
	case <-c.ctx.Done():
		return nil, fmt.Errorf("客户端已关闭")
	}
}

// SetDataHandler 设置数据消息处理器
func (c *ISPClient) SetDataHandler(handler func(*ISPMessage)) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.dataHandler = handler
}

// Subscribe 订阅数据消息
func (c *ISPClient) Subscribe(handler func(*ISPMessage)) error {
	if !c.IsConnected() {
		return fmt.Errorf("ISP客户端未连接")
	}

	c.SetDataHandler(handler)
	return nil
}

// receiveLoop 消息接收循环
func (c *ISPClient) receiveLoop() {
	defer func() {
		log.Info().Msg("ISP客户端接收循环退出")
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// 检查连接状态
			if !c.IsConnected() {
				return
			}

			// 设置读取超时 - 增加到10秒以适应数据采集间隔
			if err := c.conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
				log.Error().Err(err).Msg("设置读取超时失败")
				continue
			}

			// 读取一行消息
			if !c.scanner.Scan() {
				if err := c.scanner.Err(); err != nil {
					// 检查是否是超时错误
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						// 超时是正常的，继续循环（不记录日志避免过多输出）
						continue
					}
					log.Error().Err(err).Msg("读取消息失败")
				}
				continue
			}

			line := c.scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			// 解析消息
			msg, err := FromJSON(line)
			if err != nil {
				log.Error().Err(err).Str("data", string(line)).Msg("解析ISP消息失败")
				continue
			}

			// 只记录非数据和心跳消息，减少日志噪声
			if msg.Type != MessageTypeData && msg.Type != MessageTypeHeartbeat {
				log.Debug().
					Str("type", msg.Type).
					Str("id", msg.ID).
					Msg("收到ISP消息")
			}

			// 处理消息
			c.handleMessage(msg)
		}
	}
}

// handleMessage 处理接收到的消息
func (c *ISPClient) handleMessage(msg *ISPMessage) {
	switch msg.Type {
	case MessageTypeResponse:
		// 处理响应消息
		if msg.ID != "" {
			c.responseMu.Lock()
			if ch, exists := c.responses[msg.ID]; exists {
				select {
				case ch <- msg:
				default:
					log.Warn().Str("id", msg.ID).Msg("响应通道已满")
				}
			}
			c.responseMu.Unlock()
		}

	case MessageTypeData:
		// 处理数据消息（减少日志输出）
		c.handlerMu.Lock()
		handler := c.dataHandler
		c.handlerMu.Unlock()

		if handler != nil {
			go handler(msg)
		}

	case MessageTypeHeartbeat:
		// 处理心跳消息 - 发送心跳响应以保持连接活跃（静默处理）
		heartbeatResponse := NewHeartbeatMessage()
		if err := c.SendMessage(heartbeatResponse); err != nil {
			log.Error().Err(err).Msg("发送心跳响应失败")
		}

	default:
		log.Warn().Str("type", msg.Type).Msg("未知消息类型")
	}
}

// GetDataChannel 获取数据通道
func (c *ISPClient) GetDataChannel() <-chan *ISPMessage {
	dataCh := make(chan *ISPMessage, 100)

	// 启动数据消息过滤协程
	go func() {
		defer close(dataCh)

		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				time.Sleep(100 * time.Millisecond)
				// 实际的数据消息处理会在receiveLoop中进行
			}
		}
	}()

	return dataCh
}
