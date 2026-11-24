package plugin

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

// sinkDispatcher 为单个 Sink 提供独立的有界队列和写入协程。
//
// 以前所有 Sink 的 Publish() 都在插件管理器的唯一数据协程里串行调用，
// 任何一个慢 Sink（例如逐点往返的 Redis、同步等待 ACK 的 JetStream）都会
// 阻塞整个采集管道。现在每个 Sink 各自消费自己的队列：慢 Sink 只会让
// 自己的队列积压，队列满时丢弃该 Sink 的批次并计数，不影响其他 Sink 和采集。
type sinkDispatcher struct {
	name  string
	sink  northbound.Sink
	queue chan []model.Point

	stopOnce sync.Once
	stopCh   chan struct{}
	done     chan struct{}

	// 使用 atomic 类型保证在 32 位 ARM 上 64 位计数器的对齐
	sentBatches    atomic.Uint64
	sentPoints     atomic.Uint64
	droppedBatches atomic.Uint64
	droppedPoints  atomic.Uint64
	failedBatches  atomic.Uint64

	lastDropLog atomic.Int64 // unix nano，用于限制丢弃日志频率
}

// SinkDispatchStats Sink 分发统计
type SinkDispatchStats struct {
	Name           string `json:"name"`
	QueueLen       int    `json:"queue_len"`
	QueueCap       int    `json:"queue_cap"`
	SentBatches    uint64 `json:"sent_batches"`
	SentPoints     uint64 `json:"sent_points"`
	DroppedBatches uint64 `json:"dropped_batches"`
	DroppedPoints  uint64 `json:"dropped_points"`
	FailedBatches  uint64 `json:"failed_batches"`
}

func newSinkDispatcher(name string, sink northbound.Sink, queueSize int) *sinkDispatcher {
	if queueSize <= 0 {
		queueSize = 256
	}
	d := &sinkDispatcher{
		name:   name,
		sink:   sink,
		queue:  make(chan []model.Point, queueSize),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	go d.run()
	return d
}

// enqueue 非阻塞入队。batch 必须是该 Sink 独占的副本。
func (d *sinkDispatcher) enqueue(batch []model.Point) bool {
	select {
	case <-d.stopCh:
		return false
	default:
	}
	select {
	case d.queue <- batch:
		return true
	default:
		d.droppedBatches.Add(1)
		d.droppedPoints.Add(uint64(len(batch)))
		now := time.Now().UnixNano()
		last := d.lastDropLog.Load()
		if now-last > int64(10*time.Second) && d.lastDropLog.CompareAndSwap(last, now) {
			log.Warn().
				Str("sink", d.name).
				Uint64("dropped_points_total", d.droppedPoints.Load()).
				Msg("连接器队列已满，丢弃数据批次（该连接器写入过慢）")
		}
		return false
	}
}

func (d *sinkDispatcher) run() {
	defer close(d.done)
	for {
		select {
		case batch := <-d.queue:
			d.publish(batch)
		case <-d.stopCh:
			// 尽量把已入队的数据写完
			for {
				select {
				case batch := <-d.queue:
					d.publish(batch)
				default:
					return
				}
			}
		}
	}
}

func (d *sinkDispatcher) publish(batch []model.Point) {
	defer func() {
		if r := recover(); r != nil {
			d.failedBatches.Add(1)
			log.Error().Interface("panic", r).Str("sink", d.name).Msg("连接器发布时发生panic")
		}
	}()
	if err := d.sink.Publish(batch); err != nil {
		d.failedBatches.Add(1)
		log.Debug().Err(err).Str("sink", d.name).Int("count", len(batch)).Msg("发送数据到连接器失败")
		return
	}
	d.sentBatches.Add(1)
	d.sentPoints.Add(uint64(len(batch)))
}

// stop 停止分发器并等待队列排空（最多 timeout）
func (d *sinkDispatcher) stop(timeout time.Duration) {
	d.stopOnce.Do(func() { close(d.stopCh) })
	select {
	case <-d.done:
	case <-time.After(timeout):
		log.Warn().Str("sink", d.name).Msg("等待连接器队列排空超时")
	}
}

func (d *sinkDispatcher) stats() SinkDispatchStats {
	return SinkDispatchStats{
		Name:           d.name,
		QueueLen:       len(d.queue),
		QueueCap:       cap(d.queue),
		SentBatches:    d.sentBatches.Load(),
		SentPoints:     d.sentPoints.Load(),
		DroppedBatches: d.droppedBatches.Load(),
		DroppedPoints:  d.droppedPoints.Load(),
		FailedBatches:  d.failedBatches.Load(),
	}
}

// copyBatchForSink 为单个 Sink 复制一份批次。
// Sink 可能会修改数据点（例如 BaseSink.AddTags 添加标签），也可能异步持有数据，
// 所以每个 Sink 必须拿到独立的切片和独立的标签容器，避免互相污染。
func copyBatchForSink(points []model.Point) []model.Point {
	out := make([]model.Point, len(points))
	for i := range points {
		out[i] = points[i].Clone()
	}
	return out
}
