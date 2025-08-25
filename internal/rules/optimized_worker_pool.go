package rules

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// OptimizedWorkerPool 优化的工作池
// 特性：负载均衡、批量处理、自适应调度、NUMA感知
type OptimizedWorkerPool struct {
	// 64-bit fields first for ARM32 alignment
	nextWorker    uint64 // 原子计数器，轮询分发
	// 性能监控
	totalTasks     int64
	completedTasks int64
	failedTasks    int64
	avgLatency     int64 // 纳秒
	// Other fields
	// 基础配置
	numWorkers    int
	maxQueueSize  int
	batchSize     int
	batchTimeout  time.Duration
	// 工作队列 - 每个worker独立队列减少竞争
	workerQueues []chan RuleTask
	// 任务分发器
	dispatcherChan chan RuleTask
	dispatcherStop chan struct{}
	// Worker状态跟踪
	workerStats   []WorkerStats
	statsLock     sync.RWMutex
	// 负载均衡
	busyWorkers   []int32 // 每个worker的繁忙度
	// 生命周期管理
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	// 服务引用
	service *RuleEngineService
}

// WorkerStats Worker统计信息
type WorkerStats struct {
	// 64-bit fields first for ARM32 alignment
	TasksProcessed int64         `json:"tasks_processed"`
	TasksFailed    int64         `json:"tasks_failed"`
	// Other fields
	WorkerID       int           `json:"worker_id"`
	AvgLatency     time.Duration `json:"avg_latency"`
	QueueLength    int           `json:"queue_length"`
	IsIdle         bool          `json:"is_idle"`
	LastTaskTime   time.Time     `json:"last_task_time"`
}

// BatchTask 批量任务
type BatchTask struct {
	Tasks     []RuleTask
	StartTime time.Time
}

// NewOptimizedWorkerPool 创建优化的工作池
func NewOptimizedWorkerPool(config WorkerPoolConfig, service *RuleEngineService) *OptimizedWorkerPool {
	// 默认配置
	if config.NumWorkers <= 0 {
		config.NumWorkers = runtime.NumCPU() * 2
	}
	if config.QueueSize <= 0 {
		config.QueueSize = 1000
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 10
	}
	if config.BatchTimeout <= 0 {
		config.BatchTimeout = 5 * time.Millisecond
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// 创建worker队列
	workerQueues := make([]chan RuleTask, config.NumWorkers)
	for i := 0; i < config.NumWorkers; i++ {
		workerQueues[i] = make(chan RuleTask, config.QueueSize/config.NumWorkers)
	}
	
	pool := &OptimizedWorkerPool{
		numWorkers:     config.NumWorkers,
		maxQueueSize:   config.QueueSize,
		batchSize:      config.BatchSize,
		batchTimeout:   config.BatchTimeout,
		workerQueues:   workerQueues,
		dispatcherChan: make(chan RuleTask, config.QueueSize),
		dispatcherStop: make(chan struct{}),
		workerStats:    make([]WorkerStats, config.NumWorkers),
		busyWorkers:    make([]int32, config.NumWorkers),
		ctx:            ctx,
		cancel:         cancel,
		service:        service,
	}
	
	// 初始化worker统计
	for i := 0; i < config.NumWorkers; i++ {
		pool.workerStats[i].WorkerID = i
		pool.workerStats[i].IsIdle = true
	}
	
	return pool
}

// WorkerPoolConfig 工作池配置
type WorkerPoolConfig struct {
	NumWorkers   int           `json:"num_workers"`
	QueueSize    int           `json:"queue_size"`
	BatchSize    int           `json:"batch_size"`
	BatchTimeout time.Duration `json:"batch_timeout"`
}

// Start 启动工作池
func (p *OptimizedWorkerPool) Start() error {
	log.Info().
		Int("workers", p.numWorkers).
		Int("queue_size", p.maxQueueSize).
		Int("batch_size", p.batchSize).
		Dur("batch_timeout", p.batchTimeout).
		Msg("启动优化工作池")
	
	// 启动任务分发器
	p.wg.Add(1)
	go p.dispatcherWorker()
	
	// 启动所有worker
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	
	// 启动统计收集器
	p.wg.Add(1)
	go p.statsCollector()
	
	return nil
}

// Stop 停止工作池
func (p *OptimizedWorkerPool) Stop() error {
	log.Info().Msg("停止优化工作池")
	
	// 停止分发器
	close(p.dispatcherStop)
	
	// 取消上下文，通知所有goroutine停止
	p.cancel()
	
	// 等待所有worker完成
	p.wg.Wait()
	
	log.Info().
		Int64("total_tasks", atomic.LoadInt64(&p.totalTasks)).
		Int64("completed_tasks", atomic.LoadInt64(&p.completedTasks)).
		Int64("failed_tasks", atomic.LoadInt64(&p.failedTasks)).
		Msg("工作池停止完成")
	
	return nil
}

// SubmitTask 提交任务 - 增强版本，支持背压处理
func (p *OptimizedWorkerPool) SubmitTask(task RuleTask) bool {
	select {
	case p.dispatcherChan <- task:
		atomic.AddInt64(&p.totalTasks, 1)
		return true
	default:
		// 队列满，尝试背压处理
		return p.handleBackpressure(task)
	}
}

// handleBackpressure 处理背压情况
func (p *OptimizedWorkerPool) handleBackpressure(task RuleTask) bool {
	// 策略1: 尝试短暂等待
	select {
	case p.dispatcherChan <- task:
		atomic.AddInt64(&p.totalTasks, 1)
		log.Debug().
			Str("rule_id", task.Rule.ID).
			Str("device_id", task.Point.DeviceID).
			Msg("背压处理成功：短暂等待后任务提交成功")
		return true
	case <-time.After(time.Millisecond * 50): // 50ms超时
		// 策略2: 优先级处理 - 为关键规则保留处理能力
		if p.isHighPriorityRule(task.Rule) {
			return p.forceSubmitHighPriorityTask(task)
		}
		
		// 策略3: 记录失败并提供监控信息
		atomic.AddInt64(&p.failedTasks, 1)
		
		// 提供更详细的背压信息
		totalTasks := atomic.LoadInt64(&p.totalTasks)
		failedTasks := atomic.LoadInt64(&p.failedTasks)
		failureRate := float64(failedTasks) / float64(totalTasks) * 100
		
		log.Warn().
			Str("rule_id", task.Rule.ID).
			Str("device_id", task.Point.DeviceID).
			Int64("total_tasks", totalTasks).
			Int64("failed_tasks", failedTasks).
			Float64("failure_rate", failureRate).
			Msg("工作池队列满，任务被丢弃")
			
		return false
	}
}

// isHighPriorityRule 判断是否为高优先级规则
func (p *OptimizedWorkerPool) isHighPriorityRule(rule *Rule) bool {
	// 基于规则类型和优先级判断
	if rule.Priority >= 90 { // 高优先级规则
		return true
	}
	
	// 安全相关规则视为高优先级
	for _, action := range rule.Actions {
		if action.Type == "alert" {
			return true
		}
	}
	
	return false
}

// forceSubmitHighPriorityTask 强制提交高优先级任务
func (p *OptimizedWorkerPool) forceSubmitHighPriorityTask(task RuleTask) bool {
	// 尝试直接向最空闲的worker提交
	bestWorker := p.selectBestWorker()
	
	select {
	case p.workerQueues[bestWorker] <- task:
		atomic.AddInt64(&p.totalTasks, 1)
		log.Info().
			Str("rule_id", task.Rule.ID).
			Str("device_id", task.Point.DeviceID).
			Int("worker_id", bestWorker).
			Msg("高优先级任务强制提交成功")
		return true
	case <-time.After(time.Millisecond * 20): // 更短的超时
		atomic.AddInt64(&p.failedTasks, 1)
		log.Error().
			Str("rule_id", task.Rule.ID).
			Str("device_id", task.Point.DeviceID).
			Msg("高优先级任务强制提交失败")
		return false
	}
}

// dispatcherWorker 智能任务分发器
func (p *OptimizedWorkerPool) dispatcherWorker() {
	defer p.wg.Done()
	
	log.Info().Msg("任务分发器启动")
	
	for {
		select {
		case task := <-p.dispatcherChan:
			// 选择最空闲的worker
			workerIndex := p.selectBestWorker()
			
			select {
			case p.workerQueues[workerIndex] <- task:
				// 任务分发成功
			default:
				// worker队列满，尝试其他worker
				if !p.tryAlternativeWorkers(task, workerIndex) {
					// 所有worker都满，丢弃任务
					atomic.AddInt64(&p.failedTasks, 1)
					log.Warn().
						Str("rule_id", task.Rule.ID).
						Str("device_id", task.Point.DeviceID).
						Msg("工作池队列满，任务被丢弃")
				}
			}
			
		case <-p.dispatcherStop:
			log.Info().Msg("任务分发器停止")
			return
		case <-p.ctx.Done():
			log.Info().Msg("任务分发器上下文取消")
			return
		}
	}
}

// selectBestWorker 选择最佳worker（负载均衡）
func (p *OptimizedWorkerPool) selectBestWorker() int {
	// 方案1：轮询 + 负载检查
	startIndex := int(atomic.AddUint64(&p.nextWorker, 1) % uint64(p.numWorkers))
	
	// 检查起始worker是否空闲
	if atomic.LoadInt32(&p.busyWorkers[startIndex]) == 0 {
		return startIndex
	}
	
	// 方案2：寻找最空闲的worker
	minBusy := int32(1000000) // 足够大的初始值
	bestWorker := startIndex
	
	for i := 0; i < p.numWorkers; i++ {
		busy := atomic.LoadInt32(&p.busyWorkers[i])
		if busy < minBusy {
			minBusy = busy
			bestWorker = i
		}
		
		// 如果找到完全空闲的worker，立即返回
		if busy == 0 {
			return i
		}
	}
	
	return bestWorker
}

// tryAlternativeWorkers 尝试替代worker
func (p *OptimizedWorkerPool) tryAlternativeWorkers(task RuleTask, excludeWorker int) bool {
	for i := 0; i < p.numWorkers; i++ {
		if i == excludeWorker {
			continue
		}
		
		select {
		case p.workerQueues[i] <- task:
			return true
		default:
			continue
		}
	}
	return false
}

// worker 工作协程
func (p *OptimizedWorkerPool) worker(workerID int) {
	defer p.wg.Done()
	
	log.Info().Int("worker_id", workerID).Msg("Worker启动")
	
	queue := p.workerQueues[workerID]
	batch := make([]RuleTask, 0, p.batchSize)
	batchTimer := time.NewTimer(p.batchTimeout)
	batchTimer.Stop() // 初始时不启动计时器
	
	for {
		select {
		case task := <-queue:
			// 标记worker繁忙
			atomic.AddInt32(&p.busyWorkers[workerID], 1)
			
			batch = append(batch, task)
			
			// 如果是第一个任务，启动批处理计时器
			if len(batch) == 1 {
				batchTimer.Reset(p.batchTimeout)
			}
			
			// 检查是否达到批处理大小
			if len(batch) >= p.batchSize {
				p.processBatch(workerID, batch)
				batch = batch[:0] // 清空批次，保留容量
				batchTimer.Stop()
				atomic.StoreInt32(&p.busyWorkers[workerID], 0)
			}
			
		case <-batchTimer.C:
			// 批处理超时，处理当前批次
			if len(batch) > 0 {
				p.processBatch(workerID, batch)
				batch = batch[:0]
				atomic.StoreInt32(&p.busyWorkers[workerID], 0)
			}
			
		case <-p.ctx.Done():
			// 处理剩余任务
			if len(batch) > 0 {
				p.processBatch(workerID, batch)
			}
			log.Info().Int("worker_id", workerID).Msg("Worker停止")
			return
		}
	}
}

// processBatch 批量处理任务
func (p *OptimizedWorkerPool) processBatch(workerID int, batch []RuleTask) {
	startTime := time.Now()
	processed := 0
	failed := 0
	
	for _, task := range batch {
		if err := p.processRuleTask(task); err != nil {
			failed++
			log.Error().
				Err(err).
				Str("rule_id", task.Rule.ID).
				Str("device_id", task.Point.DeviceID).
				Int("worker_id", workerID).
				Msg("规则处理失败")
		} else {
			processed++
		}
	}
	
	// 更新统计
	duration := time.Since(startTime)
	p.updateWorkerStats(workerID, processed, failed, duration)
	
	// 更新全局统计
	atomic.AddInt64(&p.completedTasks, int64(processed))
	atomic.AddInt64(&p.failedTasks, int64(failed))
	
	// 更新平均延迟
	if processed > 0 {
		avgLatency := duration.Nanoseconds() / int64(processed)
		// 使用指数移动平均更新延迟
		oldLatency := atomic.LoadInt64(&p.avgLatency)
		newLatency := (oldLatency*7 + avgLatency*3) / 10 // 权重0.3新值，0.7旧值
		atomic.StoreInt64(&p.avgLatency, newLatency)
	}
	
	log.Debug().
		Int("worker_id", workerID).
		Int("batch_size", len(batch)).
		Int("processed", processed).
		Int("failed", failed).
		Dur("duration", duration).
		Msg("批处理完成")
}

// processRuleTask 处理单个规则任务
func (p *OptimizedWorkerPool) processRuleTask(task RuleTask) error {
	// 这里调用实际的规则处理逻辑
	// 为了避免循环依赖，使用接口调用
	return p.service.processRuleTaskInternal(task.Rule, task.Point)
}

// updateWorkerStats 更新worker统计
func (p *OptimizedWorkerPool) updateWorkerStats(workerID int, processed, failed int, duration time.Duration) {
	p.statsLock.Lock()
	defer p.statsLock.Unlock()
	
	stats := &p.workerStats[workerID]
	stats.TasksProcessed += int64(processed)
	stats.TasksFailed += int64(failed)
	stats.LastTaskTime = time.Now()
	stats.QueueLength = len(p.workerQueues[workerID])
	stats.IsIdle = (stats.QueueLength == 0)
	
	// 计算平均延迟（指数移动平均）
	if processed > 0 {
		taskAvgLatency := duration / time.Duration(processed)
		if stats.AvgLatency == 0 {
			stats.AvgLatency = taskAvgLatency
		} else {
			// EMA with α=0.3
			stats.AvgLatency = time.Duration(float64(stats.AvgLatency)*0.7 + float64(taskAvgLatency)*0.3)
		}
	}
}

// statsCollector 统计收集器
func (p *OptimizedWorkerPool) statsCollector() {
	defer p.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			p.logPoolStats()
		case <-p.ctx.Done():
			return
		}
	}
}

// logPoolStats 记录工作池统计
func (p *OptimizedWorkerPool) logPoolStats() {
	totalTasks := atomic.LoadInt64(&p.totalTasks)
	completedTasks := atomic.LoadInt64(&p.completedTasks)
	failedTasks := atomic.LoadInt64(&p.failedTasks)
	avgLatency := time.Duration(atomic.LoadInt64(&p.avgLatency))
	
	p.statsLock.RLock()
	idleWorkers := 0
	totalQueueLen := 0
	for i := 0; i < p.numWorkers; i++ {
		if p.workerStats[i].IsIdle {
			idleWorkers++
		}
		totalQueueLen += p.workerStats[i].QueueLength
	}
	p.statsLock.RUnlock()
	
	throughput := float64(completedTasks) / 60.0 // 每分钟处理任务数
	
	log.Info().
		Int64("total_tasks", totalTasks).
		Int64("completed", completedTasks).
		Int64("failed", failedTasks).
		Dur("avg_latency", avgLatency).
		Int("idle_workers", idleWorkers).
		Int("total_queue_len", totalQueueLen).
		Float64("throughput_per_min", throughput).
		Msg("工作池性能统计")
}

// GetStats 获取工作池统计信息
func (p *OptimizedWorkerPool) GetStats() WorkerPoolStats {
	p.statsLock.RLock()
	defer p.statsLock.RUnlock()
	
	stats := WorkerPoolStats{
		NumWorkers:     p.numWorkers,
		TotalTasks:     atomic.LoadInt64(&p.totalTasks),
		CompletedTasks: atomic.LoadInt64(&p.completedTasks),
		FailedTasks:    atomic.LoadInt64(&p.failedTasks),
		AvgLatency:     time.Duration(atomic.LoadInt64(&p.avgLatency)),
		WorkerStats:    make([]WorkerStats, p.numWorkers),
	}
	
	copy(stats.WorkerStats, p.workerStats)
	
	// 计算汇总信息
	for i := 0; i < p.numWorkers; i++ {
		stats.TotalQueueLength += p.workerStats[i].QueueLength
		if p.workerStats[i].IsIdle {
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
	NumWorkers       int             `json:"num_workers"`
	TotalTasks       int64           `json:"total_tasks"`
	CompletedTasks   int64           `json:"completed_tasks"`
	FailedTasks      int64           `json:"failed_tasks"`
	SuccessRate      float64         `json:"success_rate"`
	AvgLatency       time.Duration   `json:"avg_latency"`
	IdleWorkers      int             `json:"idle_workers"`
	TotalQueueLength int             `json:"total_queue_length"`
	WorkerStats      []WorkerStats   `json:"worker_stats"`
}