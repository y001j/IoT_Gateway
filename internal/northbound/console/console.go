package console

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

func init() {
	// 注册控制台sink工厂
	northbound.Register("console", func() northbound.Sink {
		return NewConsoleSink()
	})
}

// NewConsoleSink 创建一个新的控制台连接器
func NewConsoleSink() *ConsoleSink {
	return &ConsoleSink{
		BaseSink: northbound.NewBaseSink("console"),
	}
}

// ConsoleSink 实现了将数据点输出到控制台的sink
type ConsoleSink struct {
	*northbound.BaseSink
	buffer []model.Point
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Init 初始化控制台sink
func (s *ConsoleSink) Init(cfg json.RawMessage) error {
	// 使用标准化配置解析
	_, err := s.ParseStandardConfig(cfg)
	if err != nil {
		return fmt.Errorf("解析控制台sink配置失败: %w", err)
	}

	// 初始化缓冲区
	s.buffer = make([]model.Point, 0, s.GetBatchSize())

	log.Info().
		Str("name", s.Name()).
		Int("batch_size", s.GetBatchSize()).
		Int("buffer_size", s.GetBufferSize()).
		Msg("控制台sink初始化完成")

	return nil
}

// Start 启动控制台sink
func (s *ConsoleSink) Start(ctx context.Context) error {
	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 启动后台刷新协程
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.flush(); err != nil {
					s.HandleError(err, "刷新缓冲区")
				}
			case <-s.ctx.Done():
				// 确保最后一次刷新
				if err := s.flush(); err != nil {
					s.HandleError(err, "最终刷新缓冲区")
				}
				return
			}
		}
	}()

	log.Info().Str("name", s.Name()).Msg("控制台sink已启动")
	return nil
}

// Publish 发布数据点到控制台
func (s *ConsoleSink) Publish(batch []model.Point) error {
	if len(batch) == 0 {
		return nil
	}

	// 记录发布操作开始时间
	publishStart := time.Now()
	
	// 使用BaseSink的SafePublishBatch方法，自动处理统计
	return s.SafePublishBatch(batch, func(batch []model.Point) error {
		s.mu.Lock()
		defer s.mu.Unlock()

		// 添加到缓冲区
		s.buffer = append(s.buffer, batch...)

		// 如果缓冲区超过批处理大小，立即刷新
		if len(s.buffer) >= 100 { // 简化的批处理大小
			return s.flushLocked()
		}

		return nil
	}, publishStart)
}

// Stop 停止控制台sink
func (s *ConsoleSink) Stop() error {
	s.SetRunning(false)
	
	if s.cancel != nil {
		s.cancel()
	}

	// 等待后台协程完成
	s.wg.Wait()

	log.Info().Str("name", s.Name()).Msg("控制台sink已停止")
	return nil
}

// Healthy 检查连接器健康状态
func (s *ConsoleSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("控制台连接器未运行")
	}
	return nil
}

// flush 刷新缓冲区中的数据点到控制台
func (s *ConsoleSink) flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

// flushLocked 在已获取锁的情况下刷新缓冲区
func (s *ConsoleSink) flushLocked() error {
	if len(s.buffer) == 0 {
		return nil
	}

	// 打印所有数据点到控制台
	fmt.Print("\n===== 数据点批次开始 =====\n")
	for _, point := range s.buffer {
		// 格式化时间
		timeStr := point.Timestamp.Format("2006-01-02 15:04:05.000")

		// 格式化标签
		tagsStr := ""
		for k, v := range point.Tags {
			tagsStr += fmt.Sprintf("%s=%s ", k, v)
		}

		// 打印数据点
		fmt.Printf("[%s] %s.%s = %v (%s) %s\n",
			timeStr,
			point.DeviceID,
			point.Key,
			point.Value,
			point.Type,
			tagsStr)
	}
	fmt.Println("===== 数据点批次结束 =====")

	// 清空缓冲区
	s.buffer = s.buffer[:0]
	return nil
}
