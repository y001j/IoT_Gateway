package actions

import (
	"fmt"
	"hash/fnv"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// ShardedAggregateManager 分片并行聚合管理器
// 通过分片减少锁竞争，实现高并发处理能力
type ShardedAggregateManager struct {
	shards         []*AggregateManagerShard
	numShards      int32
	batchProcessor *BatchProcessor
	metrics        *PerformanceMetrics
}

// AggregateManagerShard 聚合管理器分片
type AggregateManagerShard struct {
	states         sync.Map                 // key: stateKey, value: *HighPerformanceStats
	lastCleanup    int64                    // atomic - 上次清理时间戳
	cleanupRunning int32                    // atomic - 清理是否在运行
	shardID        int32
}

// BatchProcessor 批量处理器
type BatchProcessor struct {
	batchSize       int32
	flushInterval   time.Duration
	pointChannels   []chan *BatchedPoint
	flushTicker     *time.Ticker
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

// BatchedPoint 批量处理的数据点
type BatchedPoint struct {
	Point  model.Point
	Rule   *rules.Rule
	Config *AggregateConfig
	Result chan<- *rules.ActionResult
}

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	processedCount    int64 // atomic - 处理的数据点数
	batchCount        int64 // atomic - 批次数
	averageLatency    int64 // atomic - 平均延迟(纳秒)
	peakTPS           int64 // atomic - 峰值TPS
	lastSecondCount   int64 // atomic - 上一秒处理数
	lastSecondTime    int64 // atomic - 上一秒时间戳
}

// NewShardedAggregateManager 创建分片聚合管理器
func NewShardedAggregateManager() *ShardedAggregateManager {
	numShards := runtime.NumCPU()
	if numShards < 4 {
		numShards = 4
	}
	if numShards > 64 {
		numShards = 64
	}

	sam := &ShardedAggregateManager{
		numShards: int32(numShards),
		metrics:   &PerformanceMetrics{},
	}

	// 初始化分片
	sam.shards = make([]*AggregateManagerShard, numShards)
	for i := 0; i < numShards; i++ {
		sam.shards[i] = &AggregateManagerShard{
			shardID: int32(i),
		}
	}

	// 初始化批量处理器
	sam.batchProcessor = &BatchProcessor{
		batchSize:     500,  // 默认批量大小
		flushInterval: 10 * time.Millisecond, // 默认刷新间隔
		pointChannels: make([]chan *BatchedPoint, numShards),
		stopChan:      make(chan struct{}),
	}

	// 为每个分片创建批量处理通道
	for i := 0; i < numShards; i++ {
		sam.batchProcessor.pointChannels[i] = make(chan *BatchedPoint, 1000)
		sam.startBatchProcessor(i)
	}

	// 启动性能监控
	sam.startPerformanceMonitoring()
	
	// 启动定期清理
	sam.startPeriodicCleanup()

	return sam
}

// ProcessPoint 处理数据点 - 高性能版本
func (sam *ShardedAggregateManager) ProcessPoint(rule *rules.Rule, point model.Point, config *AggregateConfig) (*rules.ActionResult, error) {
	start := time.Now()
	
	// 如果启用批量处理且是高频场景
	if sam.shouldUseBatchProcessing(config) {
		return sam.processBatched(rule, point, config)
	}
	
	// 直接处理
	return sam.processDirect(rule, point, config, start)
}

// processDirect 直接处理模式 - 低延迟
func (sam *ShardedAggregateManager) processDirect(rule *rules.Rule, point model.Point, config *AggregateConfig, start time.Time) (*rules.ActionResult, error) {
	// 计算分片ID
	stateKey := sam.generateStateKey(rule, point, config)
	shardID := sam.getShardID(stateKey)
	shard := sam.shards[shardID]
	
	// 获取或创建统计状态
	stats := sam.getOrCreateStats(shard, stateKey, config)
	
	// 添加数据点
	if point.Value != nil {
		if val, ok := point.Value.(float64); ok {
			stats.AddValue(val)
		}
	}
	
	// 计算结果
	result := sam.buildResult(stats, point, config, start)
	
	// 更新性能指标
	sam.updateMetrics(start)
	
	return result, nil
}

// processBatched 批量处理模式 - 高吞吐量
func (sam *ShardedAggregateManager) processBatched(rule *rules.Rule, point model.Point, config *AggregateConfig) (*rules.ActionResult, error) {
	resultChan := make(chan *rules.ActionResult, 1)
	
	batchPoint := &BatchedPoint{
		Point:  point,
		Rule:   rule,
		Config: config,
		Result: resultChan,
	}
	
	// 根据规则ID分片
	shardID := sam.getShardID(rule.ID)
	
	select {
	case sam.batchProcessor.pointChannels[shardID] <- batchPoint:
		// 等待批量处理结果
		select {
		case result := <-resultChan:
			return result, nil
		case <-time.After(100 * time.Millisecond): // 超时保护
			return &rules.ActionResult{
				Type:    "aggregate",
				Success: false,
				Error:   "批量处理超时",
			}, fmt.Errorf("批量处理超时")
		}
	case <-time.After(10 * time.Millisecond): // 通道满时的超时
		// 回退到直接处理
		return sam.processDirect(rule, point, config, time.Now())
	}
}

// shouldUseBatchProcessing 判断是否应该使用批量处理
func (sam *ShardedAggregateManager) shouldUseBatchProcessing(config *AggregateConfig) bool {
	// 当前TPS > 10000 时启用批量处理
	currentTPS := atomic.LoadInt64(&sam.metrics.peakTPS)
	return currentTPS > 10000 || config.WindowSize > 100
}

// getShardID 获取分片ID
func (sam *ShardedAggregateManager) getShardID(key string) int {
	hash := fnv.New32a()
	hash.Write([]byte(key))
	return int(hash.Sum32()) % int(sam.numShards)
}

// generateStateKey 生成状态键
func (sam *ShardedAggregateManager) generateStateKey(rule *rules.Rule, point model.Point, config *AggregateConfig) string {
	baseKey := fmt.Sprintf("%s:%s", rule.ID, "default")
	
	// 如果有分组字段，添加到键中
	if len(config.GroupBy) > 0 {
		for _, field := range config.GroupBy {
			switch field {
			case "device_id":
				baseKey += ":" + point.DeviceID
			case "key":
				baseKey += ":" + point.Key
			case "type":
				baseKey += ":" + string(point.Type)
			}
		}
	}
	
	return baseKey
}

// getOrCreateStats 获取或创建统计状态
func (sam *ShardedAggregateManager) getOrCreateStats(shard *AggregateManagerShard, stateKey string, config *AggregateConfig) *HighPerformanceStats {
	// 首次尝试快速获取
	if value, exists := shard.states.Load(stateKey); exists {
		return value.(*HighPerformanceStats)
	}
	
	// 创建新的统计状态
	configMap := make(map[string]interface{})
	if config.UpperLimit != nil {
		configMap["upper_limit"] = *config.UpperLimit
	}
	if config.LowerLimit != nil {
		configMap["lower_limit"] = *config.LowerLimit
	}
	if config.OutlierThreshold != nil {
		configMap["outlier_threshold"] = *config.OutlierThreshold
	}
	
	newStats := NewHighPerformanceStatsWithConfig(config.WindowSize, configMap)
	
	// 原子存储
	actual, _ := shard.states.LoadOrStore(stateKey, newStats)
	return actual.(*HighPerformanceStats)
}

// buildResult 构建结果
func (sam *ShardedAggregateManager) buildResult(stats *HighPerformanceStats, point model.Point, config *AggregateConfig, start time.Time) *rules.ActionResult {
	duration := time.Since(start)
	
	if stats.IsEmpty() {
		return &rules.ActionResult{
			Type:     "aggregate",
			Success:  false,
			Error:    "没有有效的数据点",
			Duration: duration,
		}
	}
	
	// 获取统计结果
	functions := stats.GetStats()
	
	// 构建输出数据
	outputData := map[string]interface{}{
		"aggregate_result": map[string]interface{}{
			"device_id":  point.DeviceID,
			"key":        point.Key,
			"functions":  functions,
			"count":      stats.GetCount(),
			"window":     fmt.Sprintf("window_size:%d", stats.GetWindowSize()),
			"timestamp":  time.Now(),
			"start_time": time.Now().Add(-time.Duration(stats.GetCount()) * time.Second),
			"end_time":   time.Now(),
		},
		"aggregated": true,
		"count":      stats.GetCount(),
		"state_key":  sam.generateStateKey(&rules.Rule{ID: "dummy"}, point, config),
	}
	
	return &rules.ActionResult{
		Type:     "aggregate",
		Success:  true,
		Output:   outputData,
		Duration: duration,
	}
}

// updateMetrics 更新性能指标
func (sam *ShardedAggregateManager) updateMetrics(start time.Time) {
	latency := time.Since(start).Nanoseconds()
	
	atomic.AddInt64(&sam.metrics.processedCount, 1)
	
	// 更新平均延迟 (使用指数移动平均)
	oldLatency := atomic.LoadInt64(&sam.metrics.averageLatency)
	newLatency := (oldLatency*9 + latency) / 10
	atomic.StoreInt64(&sam.metrics.averageLatency, newLatency)
}

// startBatchProcessor 启动批量处理器
func (sam *ShardedAggregateManager) startBatchProcessor(shardID int) {
	sam.batchProcessor.wg.Add(1)
	
	go func() {
		defer sam.batchProcessor.wg.Done()
		
		shard := sam.shards[shardID]
		channel := sam.batchProcessor.pointChannels[shardID]
		batch := make([]*BatchedPoint, 0, sam.batchProcessor.batchSize)
		ticker := time.NewTicker(sam.batchProcessor.flushInterval)
		defer ticker.Stop()
		
		for {
			select {
			case point := <-channel:
				batch = append(batch, point)
				
				// 批次满了或者是最后一个，立即处理
				if len(batch) >= int(sam.batchProcessor.batchSize) {
					sam.processBatch(shard, batch)
					batch = batch[:0]
				}
				
			case <-ticker.C:
				// 定时刷新
				if len(batch) > 0 {
					sam.processBatch(shard, batch)
					batch = batch[:0]
				}
				
			case <-sam.batchProcessor.stopChan:
				// 处理剩余批次
				if len(batch) > 0 {
					sam.processBatch(shard, batch)
				}
				return
			}
		}
	}()
}

// processBatch 处理一个批次
func (sam *ShardedAggregateManager) processBatch(shard *AggregateManagerShard, batch []*BatchedPoint) {
	start := time.Now()
	atomic.AddInt64(&sam.metrics.batchCount, 1)
	
	// 按状态键分组
	groups := make(map[string][]*BatchedPoint)
	for _, point := range batch {
		stateKey := sam.generateStateKey(point.Rule, point.Point, point.Config)
		groups[stateKey] = append(groups[stateKey], point)
	}
	
	// 批量处理每个分组
	for stateKey, groupPoints := range groups {
		sam.processBatchGroup(shard, stateKey, groupPoints, start)
	}
}

// processBatchGroup 处理同一状态键的批量数据
func (sam *ShardedAggregateManager) processBatchGroup(shard *AggregateManagerShard, stateKey string, points []*BatchedPoint, start time.Time) {
	if len(points) == 0 {
		return
	}
	
	// 获取配置(使用第一个点的配置)
	config := points[0].Config
	
	// 获取或创建统计状态
	stats := sam.getOrCreateStats(shard, stateKey, config)
	
	// 提取数值进行批量处理
	values := make([]float64, 0, len(points))
	for _, point := range points {
		if point.Point.Value != nil {
			if val, ok := point.Point.Value.(float64); ok {
				values = append(values, val)
			}
		}
	}
	
	// 批量添加数据
	if len(values) > 0 {
		stats.AddBatch(values)
	}
	
	// 为每个点生成结果
	for _, point := range points {
		result := sam.buildResult(stats, point.Point, point.Config, start)
		
		select {
		case point.Result <- result:
		default:
			// 结果通道已关闭或满了，记录错误
			log.Warn().Msg("批量处理结果通道发送失败")
		}
	}
}

// startPerformanceMonitoring 启动性能监控
func (sam *ShardedAggregateManager) startPerformanceMonitoring() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		
		for range ticker.C {
			currentTime := time.Now().Unix()
			lastTime := atomic.LoadInt64(&sam.metrics.lastSecondTime)
			
			if currentTime > lastTime {
				currentCount := atomic.LoadInt64(&sam.metrics.processedCount)
				lastCount := atomic.LoadInt64(&sam.metrics.lastSecondCount)
				
				tps := currentCount - lastCount
				atomic.StoreInt64(&sam.metrics.peakTPS, tps)
				atomic.StoreInt64(&sam.metrics.lastSecondCount, currentCount)
				atomic.StoreInt64(&sam.metrics.lastSecondTime, currentTime)
				
				// 记录性能指标
				if tps > 1000 {
					avgLatency := atomic.LoadInt64(&sam.metrics.averageLatency) / 1000000 // 转换为毫秒
					log.Debug().
						Int64("tps", tps).
						Int64("avg_latency_ms", avgLatency).
						Int64("total_processed", currentCount).
						Msg("聚合引擎性能指标")
				}
			}
		}
	}()
}

// startPeriodicCleanup 启动定期清理
func (sam *ShardedAggregateManager) startPeriodicCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		
		for range ticker.C {
			sam.cleanupExpiredStates()
		}
	}()
}

// cleanupExpiredStates 清理过期状态
func (sam *ShardedAggregateManager) cleanupExpiredStates() {
	now := time.Now().Unix()
	cleanupInterval := int64(300) // 5分钟
	
	for i, shard := range sam.shards {
		// 检查是否需要清理这个分片
		lastCleanup := atomic.LoadInt64(&shard.lastCleanup)
		if now-lastCleanup < cleanupInterval {
			continue
		}
		
		// 检查是否已经在清理中
		if !atomic.CompareAndSwapInt32(&shard.cleanupRunning, 0, 1) {
			continue
		}
		
		go func(shardID int, s *AggregateManagerShard) {
			defer atomic.StoreInt32(&s.cleanupRunning, 0)
			
			cleanupCount := 0
			s.states.Range(func(key, value interface{}) bool {
				stats := value.(*HighPerformanceStats)
				
				// 如果统计状态为空且超过清理时间，则删除
				if stats.IsEmpty() {
					s.states.Delete(key)
					cleanupCount++
				}
				return true
			})
			
			atomic.StoreInt64(&s.lastCleanup, now)
			
			if cleanupCount > 0 {
				log.Debug().
					Int("shard_id", shardID).
					Int("cleanup_count", cleanupCount).
					Msg("清理过期聚合状态")
			}
		}(i, shard)
	}
}

// Close 关闭管理器
func (sam *ShardedAggregateManager) Close() {
	close(sam.batchProcessor.stopChan)
	sam.batchProcessor.wg.Wait()
	
	// 关闭所有通道
	for _, ch := range sam.batchProcessor.pointChannels {
		close(ch)
	}
}

// GetMetrics 获取性能指标
func (sam *ShardedAggregateManager) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"processed_count":   atomic.LoadInt64(&sam.metrics.processedCount),
		"batch_count":       atomic.LoadInt64(&sam.metrics.batchCount),
		"average_latency_ms": atomic.LoadInt64(&sam.metrics.averageLatency) / 1000000,
		"current_tps":       atomic.LoadInt64(&sam.metrics.peakTPS),
		"num_shards":        sam.numShards,
		"batch_size":        sam.batchProcessor.batchSize,
	}
}