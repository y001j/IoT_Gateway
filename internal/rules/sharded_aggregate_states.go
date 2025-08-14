package rules

import (
	"hash/fnv"
	"sync"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// ShardedAggregateStates 分片聚合状态管理器
// 使用分片锁替代全局锁，大幅提升并发性能
type ShardedAggregateStates struct {
	shards    []aggregateShard
	shardMask uint64
	numShards int
}

// aggregateShard 聚合状态分片
type aggregateShard struct {
	mu     sync.RWMutex
	states map[string]*AggregateState
}

// NewShardedAggregateStates 创建分片聚合状态管理器
func NewShardedAggregateStates(numShards int) *ShardedAggregateStates {
	// 确保分片数量为2的幂次，便于位运算优化
	if numShards <= 0 {
		numShards = 16 // 默认16个分片
	}
	
	// 调整为2的幂次
	actualShards := 1
	for actualShards < numShards {
		actualShards <<= 1
	}
	
	shards := make([]aggregateShard, actualShards)
	for i := range shards {
		shards[i].states = make(map[string]*AggregateState)
	}
	
	return &ShardedAggregateStates{
		shards:    shards,
		shardMask: uint64(actualShards - 1),
		numShards: actualShards,
	}
}

// getShard 根据键获取对应的分片
func (s *ShardedAggregateStates) getShard(key string) *aggregateShard {
	// 使用FNV-1a哈希算法，性能优秀且分布均匀
	h := fnv.New64a()
	h.Write([]byte(key))
	hash := h.Sum64()
	
	// 使用位运算快速取模
	shardIndex := hash & s.shardMask
	return &s.shards[shardIndex]
}

// GetOrCreateState 获取或创建聚合状态（线程安全）
func (s *ShardedAggregateStates) GetOrCreateState(stateKey string, windowSize int) *AggregateState {
	shard := s.getShard(stateKey)
	
	// 先尝试读锁获取
	shard.mu.RLock()
	if state, exists := shard.states[stateKey]; exists {
		shard.mu.RUnlock()
		return state
	}
	shard.mu.RUnlock()
	
	// 需要创建，使用写锁
	shard.mu.Lock()
	defer shard.mu.Unlock()
	
	// 双重检查，避免并发创建
	if state, exists := shard.states[stateKey]; exists {
		return state
	}
	
	// 创建新状态
	state := &AggregateState{
		Buffer:     make([]model.Point, 0, windowSize),
		WindowSize: windowSize,
		LastUpdate: time.Now(),
	}
	shard.states[stateKey] = state
	return state
}

// UpdateState 更新聚合状态（线程安全）
func (s *ShardedAggregateStates) UpdateState(stateKey string, point model.Point, windowSize int) (*AggregateState, bool) {
	shard := s.getShard(stateKey)
	
	shard.mu.Lock()
	defer shard.mu.Unlock()
	
	// 获取或创建状态
	state, exists := shard.states[stateKey]
	if !exists {
		state = &AggregateState{
			Buffer:     make([]model.Point, 0, windowSize),
			WindowSize: windowSize,
			LastUpdate: time.Now(),
		}
		shard.states[stateKey] = state
	}
	
	// 添加数据点
	state.Buffer = append(state.Buffer, point)
	state.LastUpdate = time.Now()
	
	// 检查是否达到窗口大小
	windowReady := len(state.Buffer) >= windowSize
	if windowReady {
		// 注意：这里不清空缓冲区，由调用者处理
		// 这样可以避免在锁内进行复杂计算
	}
	
	return state, windowReady
}

// ClearStateBuffer 清空状态缓冲区（用于滑动窗口）
func (s *ShardedAggregateStates) ClearStateBuffer(stateKey string) {
	shard := s.getShard(stateKey)
	
	shard.mu.Lock()
	defer shard.mu.Unlock()
	
	if state, exists := shard.states[stateKey]; exists {
		state.Buffer = state.Buffer[:0] // 保留容量，避免重分配
	}
}

// CleanExpiredStates 清理过期状态
func (s *ShardedAggregateStates) CleanExpiredStates(expireDuration time.Duration) int {
	expireTime := time.Now().Add(-expireDuration)
	totalCleaned := 0
	
	// 并行清理各个分片
	var wg sync.WaitGroup
	cleanedCounts := make([]int, s.numShards)
	
	for i := 0; i < s.numShards; i++ {
		wg.Add(1)
		go func(shardIndex int) {
			defer wg.Done()
			
			shard := &s.shards[shardIndex]
			shard.mu.Lock()
			defer shard.mu.Unlock()
			
			cleaned := 0
			for key, state := range shard.states {
				if state.LastUpdate.Before(expireTime) {
					delete(shard.states, key)
					cleaned++
				}
			}
			cleanedCounts[shardIndex] = cleaned
		}(i)
	}
	
	wg.Wait()
	
	// 汇总清理数量
	for _, count := range cleanedCounts {
		totalCleaned += count
	}
	
	return totalCleaned
}

// GetStats 获取分片统计信息
func (s *ShardedAggregateStates) GetStats() ShardedStatsInfo {
	stats := ShardedStatsInfo{
		NumShards:   s.numShards,
		ShardStats:  make([]ShardStats, s.numShards),
		TotalStates: 0,
	}
	
	// 并行收集各分片统计信息
	var wg sync.WaitGroup
	for i := 0; i < s.numShards; i++ {
		wg.Add(1)
		go func(shardIndex int) {
			defer wg.Done()
			
			shard := &s.shards[shardIndex]
			shard.mu.RLock()
			defer shard.mu.RUnlock()
			
			stats.ShardStats[shardIndex] = ShardStats{
				ShardIndex:  shardIndex,
				StateCount:  len(shard.states),
				LoadFactor:  float64(len(shard.states)) / float64(len(shard.states)+1), // 简单负载因子
			}
		}(i)
	}
	
	wg.Wait()
	
	// 计算总数和负载均衡度
	maxStates, minStates := 0, int(^uint(0)>>1) // 最大值，最小值
	for _, shardStat := range stats.ShardStats {
		stats.TotalStates += shardStat.StateCount
		if shardStat.StateCount > maxStates {
			maxStates = shardStat.StateCount
		}
		if shardStat.StateCount < minStates {
			minStates = shardStat.StateCount
		}
	}
	
	// 计算负载均衡系数 (值越接近1表示负载越均衡)
	if maxStates > 0 {
		stats.LoadBalance = float64(minStates) / float64(maxStates)
	} else {
		stats.LoadBalance = 1.0
	}
	
	return stats
}

// ShardedStatsInfo 分片统计信息
type ShardedStatsInfo struct {
	NumShards   int         `json:"num_shards"`
	TotalStates int         `json:"total_states"`
	LoadBalance float64     `json:"load_balance"` // 负载均衡系数
	ShardStats  []ShardStats `json:"shard_stats"`
}

// ShardStats 单个分片统计
type ShardStats struct {
	ShardIndex int     `json:"shard_index"`
	StateCount int     `json:"state_count"`
	LoadFactor float64 `json:"load_factor"`
}

// 兼容性函数：将分片管理器包装为原始接口
func (s *ShardedAggregateStates) GetState(stateKey string) (*AggregateState, bool) {
	shard := s.getShard(stateKey)
	
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	
	state, exists := shard.states[stateKey]
	return state, exists
}

// SetState 设置状态（主要用于测试和兼容性）
func (s *ShardedAggregateStates) SetState(stateKey string, state *AggregateState) {
	shard := s.getShard(stateKey)
	
	shard.mu.Lock()
	defer shard.mu.Unlock()
	
	shard.states[stateKey] = state
}

// ListAllStates 列出所有状态键（调试用）
func (s *ShardedAggregateStates) ListAllStates() []string {
	var allKeys []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	
	// 并行收集所有分片的键
	for i := 0; i < s.numShards; i++ {
		wg.Add(1)
		go func(shardIndex int) {
			defer wg.Done()
			
			shard := &s.shards[shardIndex]
			shard.mu.RLock()
			defer shard.mu.RUnlock()
			
			var shardKeys []string
			for key := range shard.states {
				shardKeys = append(shardKeys, key)
			}
			
			mu.Lock()
			allKeys = append(allKeys, shardKeys...)
			mu.Unlock()
		}(i)
	}
	
	wg.Wait()
	return allKeys
}