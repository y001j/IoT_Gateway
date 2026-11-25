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
	reader      *bufio.Reader
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
	c.reader = bufio.NewReaderSize(conn, 64*1024)
	c.writer = bufio.NewWriter(conn)
	c.connected = true

	log.Info().
		Str("address", c.address).
		Msg("ISP客户端连接成功")

	// 启动消息接收协程（传入本次连接的 reader/conn，避免与重连产生竞争）
	go c.receiveLoop(c.ctx, conn, c.reader)

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
		_ = c.conn.Close()
		c.conn = nil
	}

	// 清理等待中的请求（不关闭通道：通道由 SendRequest 自己管理，
	// 以前这里和 SendRequest 的 defer 都会 close，断开时有请求在等待就会 panic；
	// 等待中的请求会通过 ctx.Done() 返回）
	c.responseMu.Lock()
	for id := range c.responses {
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

	// 确保清理响应通道（不 close：handleMessage 可能正持有该通道）
	defer func() {
		c.responseMu.Lock()
		delete(c.responses, msg.ID)
		c.responseMu.Unlock()
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
	case <-c.doneChan():
		return nil, fmt.Errorf("客户端已关闭")
	}
}

// doneChan 返回当前连接的取消信号
func (c *ISPClient) doneChan() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return c.ctx.Done()
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

// receiveLoop 消息接收循环（每行一个 JSON 消息）。
//
// 以前使用 bufio.Scanner + 10 秒读超时：Scanner 的错误是粘滞的，第一次超时（数据间隔超过
// 10 秒）或连接断开（EOF）后 Scan() 会立即返回 false，循环 continue 形成 100% CPU 的空转，
// 并且再也收不到任何消息；超过 64KB 的消息同样会让接收永久停止。
// 现在使用阻塞读取，连接关闭或出错时标记为断开并退出，由上层负责重连。
func (c *ISPClient) receiveLoop(ctx context.Context, conn net.Conn, reader *bufio.Reader) {
	defer log.Debug().Str("address", c.address).Msg("ISP客户端接收循环退出")

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if msg, perr := FromJSON(trimLine(line)); perr != nil {
				log.Error().Err(perr).Int("bytes", len(line)).Msg("解析ISP消息失败")
			} else {
				if msg.Type != MessageTypeData && msg.Type != MessageTypeHeartbeat {
					log.Debug().Str("type", msg.Type).Str("id", msg.ID).Msg("收到ISP消息")
				}
				c.handleMessage(msg)
			}
		}
		if err != nil {
			select {
			case <-ctx.Done():
				// 主动断开
			default:
				log.Warn().Err(err).Str("address", c.address).Msg("ISP连接已断开")
				c.markDisconnected(conn)
			}
			return
		}
	}
}

// markDisconnected 连接异常断开时更新状态（仅当仍是同一个连接时）
func (c *ISPClient) markDisconnected(conn net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != conn {
		return
	}
	c.connected = false
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.conn.Close()
	c.conn = nil
}

func trimLine(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
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

		// 同步调用，保证数据消息的顺序（以前每条消息起一个协程，顺序无法保证）。
		// 处理器必须是非阻塞的（ISPAdapterProxy 使用非阻塞发送）。
		if handler != nil {
			handler(msg)
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
