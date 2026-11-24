package rules

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// OptimizedWorkerPool 规则任务工作池。
//
// 设计要点：
//   - 按 hash(device_id, key) 把任务固定分配到某个 worker，同一数据序列的点
//     总是由同一个 worker 按到达顺序处理。以前按轮询/最空闲分发，同一设备的
//     连续点会被并行处理而乱序，影响滑动窗口、变化率、连续计数等依赖顺序的逻辑。
//   - 任务到达后立即处理，不再攒批等待（以前最多额外等待 10ms）。
//   - 队列满时在 SubmitTimeout 内阻塞等待（对上游形成背压），超时仍满才丢弃，
//     丢弃数量计入 DroppedTasks 并可通过 GetStats 观测；不再出现
//     "提交成功但随后在分发器里被静默丢弃" 的情况。
type OptimizedWorkerPool struct {
	// 64-bit fields first for ARM32 alignment
	totalTasks     int64
	completedTasks int64
	failedTasks    int64
	droppedTasks   int64
	avgLatency     int64 // 纳秒
	lastDropLog    int64

	numWorkers    int
	maxQueueSize  int
	submitTimeout time.Duration

	workerQueues []chan RuleTask

	workerStats []WorkerStats
	statsLock   sync.RWMutex

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once
	stopped  atomic.Bool

	// process 处理单个任务；默认调用 service.processRuleTaskInternal，测试时可替换
	process func(task RuleTask) error
}

// WorkerStats Worker统计信息
type WorkerStats struct {
	// 64-bit fields first for ARM32 alignment
	TasksProcessed int64 `json:"tasks_processed"`
	TasksFailed    int64 `json:"tasks_failed"`
	// Other fields
	WorkerID     int           `json:"worker_id"`
	AvgLatency   time.Duration `json:"avg_latency"`
	QueueLength  int           `json:"queue_length"`
	IsIdle       bool          `json:"is_idle"`
	LastTaskTime time.Time     `json:"last_task_time"`
}

// WorkerPoolConfig 工作池配置
type WorkerPoolConfig struct {
	NumWorkers int `json:"num_workers"`
	QueueSize  int `json:"queue_size"`
	// SubmitTimeout 队列满时最多等待多久（默认 100ms）；<0 表示不等待
	SubmitTimeout time.Duration `json:"submit_timeout"`

	// 以下字段为兼容旧配置保留，已不再使用（任务不再攒批）
	BatchSize    int           `json:"batch_size"`
	BatchTimeout time.Duration `json:"batch_timeout"`
}

// NewOptimizedWorkerPool 创建工作池
func NewOptimizedWorkerPool(config WorkerPoolConfig, service *RuleEngineService) *OptimizedWorkerPool {
	if config.NumWorkers <= 0 {
		config.NumWorkers = runtime.NumCPU() * 2
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 1000
	}
	if config.SubmitTimeout == 0 {
		config.SubmitTimeout = 100 * time.Millisecond
	}
	perWorker := config.QueueSize / config.NumWorkers
	if perWorker < 16 {
		perWorker = 16
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &OptimizedWorkerPool{
		numWorkers:    config.NumWorkers,
		maxQueueSize:  perWorker * config.NumWorkers,
		submitTimeout: config.SubmitTimeout,
		workerQueues:  make([]chan RuleTask, config.NumWorkers),
		workerStats:   make([]WorkerStats, config.NumWorkers),
		ctx:           ctx,
		cancel:        cancel,
	}
	for i := 0; i < config.NumWorkers; i++ {
		pool.workerQueues[i] = make(chan RuleTask, perWorker)
		pool.workerStats[i].WorkerID = i
		pool.workerStats[i].IsIdle = true
	}
	if service != nil {
		pool.process = func(task RuleTask) error {
			return service.processRuleTaskInternal(task.Rule, task.Point)
		}
	}
	return pool
}

// Start 启动工作池
func (p *OptimizedWorkerPool) Start() error {
	log.Info().Int("workers", p.numWorkers).Int("queue_size", p.maxQueueSize).Msg("启动规则引擎工作池")
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	return nil
}

// Stop 停止工作池：不再接收新任务，处理完队列中剩余任务后退出
func (p *OptimizedWorkerPool) Stop() error {
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		p.cancel()
		p.wg.Wait()
		log.Info().
			Int64("total_tasks", atomic.LoadInt64(&p.totalTasks)).
			Int64("completed_tasks", atomic.LoadInt64(&p.completedTasks)).
			Int64("failed_tasks", atomic.LoadInt64(&p.failedTasks)).
			Int64("dropped_tasks", atomic.LoadInt64(&p.droppedTasks)).
			Msg("规则引擎工作池已停止")
	})
	return nil
}

// shardFor 根据数据序列选择 worker（FNV-1a）
func (p *OptimizedWorkerPool) shardFor(task RuleTask) int {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	h := uint32(offset32)
	s := task.Point.DeviceID
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	h ^= 0xff
	h *= prime32
	s = task.Point.Key
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return int(h % uint32(p.numWorkers))
}

// SubmitTask 提交任务。返回 false 表示任务被丢弃（工作池已停止或队列持续满）。
func (p *OptimizedWorkerPool) SubmitTask(task RuleTask) bool {
	if p.stopped.Load() {
		return false
	}
	q := p.workerQueues[p.shardFor(task)]

	select {
	case q <- task:
		atomic.AddInt64(&p.totalTasks, 1)
		return true
	default:
	}

	if p.submitTimeout > 0 {
		timer := time.NewTimer(p.submitTimeout)
		defer timer.Stop()
		select {
		case q <- task:
			atomic.AddInt64(&p.totalTasks, 1)
			return true
		case <-timer.C:
		case <-p.ctx.Done():
		}
	}

	dropped := atomic.AddInt64(&p.droppedTasks, 1)
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&p.lastDropLog)
	if now-last > int64(10*time.Second) && atomic.CompareAndSwapInt64(&p.lastDropLog, last, now) {
		ruleID := ""
		if task.Rule != nil {
			ruleID = task.Rule.ID
		}
		log.Warn().Str("rule_id", ruleID).Str("device_id", task.Point.DeviceID).
			Int64("dropped_total", dropped).Msg("规则引擎工作池队列已满，任务被丢弃")
	}
	return false
}

func (p *OptimizedWorkerPool) worker(workerID int) {
	defer p.wg.Done()
	queue := p.workerQueues[workerID]
	for {
		select {
		case task := <-queue:
			p.runTask(workerID, task)
		case <-p.ctx.Done():
			// 处理剩余任务后退出
			for {
				select {
				case task := <-queue:
					p.runTask(workerID, task)
				default:
					return
				}
			}
		}
	}
}

func (p *OptimizedWorkerPool) runTask(workerID int, task RuleTask) {
	start := time.Now()
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errPanic(r)
				log.Error().Interface("panic", r).Int("worker_id", workerID).Msg("规则处理发生panic")
			}
		}()
		if p.process != nil {
			err = p.process(task)
		}
	}()
	d := time.Since(start)

	if err != nil {
		atomic.AddInt64(&p.failedTasks, 1)
	} else {
		atomic.AddInt64(&p.completedTasks, 1)
	}
	old := atomic.LoadInt64(&p.avgLatency)
	atomic.StoreInt64(&p.avgLatency, (old*7+d.Nanoseconds()*3)/10)

	p.statsLock.Lock()
	ws := &p.workerStats[workerID]
	if err != nil {
		ws.TasksFailed++
	} else {
		ws.TasksProcessed++
	}
	ws.LastTaskTime = time.Now()
	ws.QueueLength = len(p.workerQueues[workerID])
	ws.IsIdle = ws.QueueLength == 0
	if ws.AvgLatency == 0 {
		ws.AvgLatency = d
	} else {
		ws.AvgLatency = time.Duration(float64(ws.AvgLatency)*0.7 + float64(d)*0.3)
	}
	p.statsLock.Unlock()
}

type panicError struct{ v interface{} }

func (e panicError) Error() string { return "panic in rule processing" }

func errPanic(v interface{}) error { return panicError{v: v} }

// GetStats 获取工作池统计信息
func (p *OptimizedWorkerPool) GetStats() WorkerPoolStats {
	p.statsLock.RLock()
	defer p.statsLock.RUnlock()

	stats := WorkerPoolStats{
		NumWorkers:     p.numWorkers,
		TotalTasks:     atomic.LoadInt64(&p.totalTasks),
		CompletedTasks: atomic.LoadInt64(&p.completedTasks),
		FailedTasks:    atomic.LoadInt64(&p.failedTasks),
		DroppedTasks:   atomic.LoadInt64(&p.droppedTasks),
		AvgLatency:     time.Duration(atomic.LoadInt64(&p.avgLatency)),
		WorkerStats:    make([]WorkerStats, p.numWorkers),
	}
	copy(stats.WorkerStats, p.workerStats)
	for i := 0; i < p.numWorkers; i++ {
		ql := len(p.workerQueues[i])
		stats.WorkerStats[i].QueueLength = ql
		stats.TotalQueueLength += ql
		if ql == 0 {
			stats.IdleWorkers++
		}
	}
	if stats.TotalTasks > 0 {
		stats.SuccessRate = float64(stats.CompletedTasks) / float64(stats.TotalTasks)
	}
	return stats
}

// WorkerPoolStats 工作池统计信息
type WorkerPoolStats struct {
	NumWorkers       int           `json:"num_workers"`
	TotalTasks       int64         `json:"total_tasks"`
	CompletedTasks   int64         `json:"completed_tasks"`
	FailedTasks      int64         `json:"failed_tasks"`
	DroppedTasks     int64         `json:"dropped_tasks"`
	SuccessRate      float64       `json:"success_rate"`
	AvgLatency       time.Duration `json:"avg_latency"`
	IdleWorkers      int           `json:"idle_workers"`
	TotalQueueLength int           `json:"total_queue_length"`
	WorkerStats      []WorkerStats `json:"worker_stats"`
}
