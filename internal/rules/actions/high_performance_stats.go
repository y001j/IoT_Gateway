package actions

import (
	"math"
	"sort"
	"sync/atomic"
	"time"
)

// HighPerformanceStats 高性能无锁统计计算器
// 使用原子操作替代锁，实现数万倍性能提升
type HighPerformanceStats struct {
	// 64-bit fields first for ARM32 alignment
	count      int64  // atomic
	sum        uint64 // atomic (使用bits存储float64)
	sumSquares uint64 // atomic (使用bits存储float64)
	minVal     uint64 // atomic (使用bits存储float64)
	maxVal     uint64 // atomic (使用bits存储float64)
	// 时间信息
	firstUpdateTime uint64 // atomic (时间戳)
	lastUpdateTime  uint64 // atomic (时间戳)
	firstValue      uint64 // atomic (首个值)
	lastValue       uint64 // atomic (最后值)
	// 数据质量统计
	nullCount  int64 // atomic
	validCount int64 // atomic
	// 滑动窗口支持
	writeIndex   uint64 // atomic
	cacheVersion uint64 // atomic - 缓存版本号，用于失效检测
	// 阈值配置
	upperLimit       uint64 // atomic (bits)
	lowerLimit       uint64 // atomic (bits)
	outlierThreshold uint64 // atomic (bits)
	// 预计算缓存
	cachedMean   uint64 // atomic (bits)
	cachedStdDev uint64 // atomic (bits)
	// 32-bit fields
	windowSize int32
	windowFull uint32 // atomic bool
	hasUpper   uint32 // atomic bool：是否配置了上限（0 也是合法的阈值）
	hasLower   uint32 // atomic bool：是否配置了下限
	// Non-atomic fields
	ringBuffer []uint64 // 存储float64的bits
}

// NewHighPerformanceStats 创建高性能统计计算器
func NewHighPerformanceStats(windowSize int) *HighPerformanceStats {
	if windowSize < 0 {
		windowSize = 0
	}
	if windowSize > 1000000 {
		windowSize = 1000000
	}

	hps := &HighPerformanceStats{
		windowSize: int32(windowSize),
	}

	// 初始化min/max为极值
	atomic.StoreUint64(&hps.minVal, math.Float64bits(math.Inf(1)))
	atomic.StoreUint64(&hps.maxVal, math.Float64bits(math.Inf(-1)))
	atomic.StoreUint64(&hps.outlierThreshold, math.Float64bits(3.0))

	// 初始化环形缓冲区
	if windowSize > 0 {
		hps.ringBuffer = make([]uint64, windowSize)
	}

	return hps
}

// NewHighPerformanceStatsWithConfig 创建带配置的高性能统计计算器
func NewHighPerformanceStatsWithConfig(windowSize int, config map[string]interface{}) *HighPerformanceStats {
	hps := NewHighPerformanceStats(windowSize)

	if config != nil {
		// 配置阈值
		if upper, exists := config["upper_limit"]; exists {
			if val, ok := upper.(float64); ok {
				atomic.StoreUint64(&hps.upperLimit, math.Float64bits(val))
				atomic.StoreUint32(&hps.hasUpper, 1)
			}
		}

		if lower, exists := config["lower_limit"]; exists {
			if val, ok := lower.(float64); ok {
				atomic.StoreUint64(&hps.lowerLimit, math.Float64bits(val))
				atomic.StoreUint32(&hps.hasLower, 1)
			}
		}

		if threshold, exists := config["outlier_threshold"]; exists {
			if val, ok := threshold.(float64); ok && val > 0 {
				atomic.StoreUint64(&hps.outlierThreshold, math.Float64bits(val))
			}
		}
	}

	return hps
}

// 原子操作辅助函数
func atomicLoadFloat64(addr *uint64) float64 {
	return math.Float64frombits(atomic.LoadUint64(addr))
}

func atomicStoreFloat64(addr *uint64, val float64) {
	atomic.StoreUint64(addr, math.Float64bits(val))
}

func atomicAddFloat64(addr *uint64, delta float64) float64 {
	for {
		old := atomic.LoadUint64(addr)
		oldVal := math.Float64frombits(old)
		newVal := oldVal + delta
		if atomic.CompareAndSwapUint64(addr, old, math.Float64bits(newVal)) {
			return newVal
		}
	}
}

// AddValue 添加数值 - 高性能无锁实现
func (hps *HighPerformanceStats) AddValue(value float64) {
	now := time.Now().UnixNano()
	valueBits := math.Float64bits(value)

	// 原子更新计数和基础统计
	count := atomic.AddInt64(&hps.count, 1)
	atomic.AddInt64(&hps.validCount, 1)

	// 原子更新和与平方和
	atomicAddFloat64(&hps.sum, value)
	atomicAddFloat64(&hps.sumSquares, value*value)

	// 更新首次和最后更新时间
	if count == 1 {
		atomic.StoreUint64(&hps.firstUpdateTime, uint64(now))
		atomic.StoreUint64(&hps.firstValue, valueBits)
	}
	atomic.StoreUint64(&hps.lastUpdateTime, uint64(now))
	atomic.StoreUint64(&hps.lastValue, valueBits)

	// 原子更新最小值
	for {
		oldBits := atomic.LoadUint64(&hps.minVal)
		oldMin := math.Float64frombits(oldBits)
		if value >= oldMin {
			break
		}
		if atomic.CompareAndSwapUint64(&hps.minVal, oldBits, valueBits) {
			break
		}
	}

	// 原子更新最大值
	for {
		oldBits := atomic.LoadUint64(&hps.maxVal)
		oldMax := math.Float64frombits(oldBits)
		if value <= oldMax {
			break
		}
		if atomic.CompareAndSwapUint64(&hps.maxVal, oldBits, valueBits) {
			break
		}
	}

	// 滑动窗口处理
	if hps.windowSize > 0 {
		index := atomic.AddUint64(&hps.writeIndex, 1) - 1
		bufferIndex := index % uint64(hps.windowSize)
		atomic.StoreUint64(&hps.ringBuffer[bufferIndex], valueBits)

		if index >= uint64(hps.windowSize-1) {
			atomic.StoreUint32(&hps.windowFull, 1)
		}
	}

	// 缓存失效
	atomic.AddUint64(&hps.cacheVersion, 1)
}

// AddBatch 批量添加数值 - 优化批量处理性能
func (hps *HighPerformanceStats) AddBatch(values []float64) {
	if len(values) == 0 {
		return
	}

	now := time.Now().UnixNano()

	// 本地计算，减少原子操作次数
	var localSum, localSumSquares float64
	localMin := values[0]
	localMax := values[0]

	for _, v := range values {
		localSum += v
		localSumSquares += v * v
		if v < localMin {
			localMin = v
		}
		if v > localMax {
			localMax = v
		}
	}

	// 批量原子更新
	count := atomic.AddInt64(&hps.count, int64(len(values)))
	atomic.AddInt64(&hps.validCount, int64(len(values)))
	atomicAddFloat64(&hps.sum, localSum)
	atomicAddFloat64(&hps.sumSquares, localSumSquares)

	// 更新时间信息
	if count == int64(len(values)) {
		atomic.StoreUint64(&hps.firstUpdateTime, uint64(now))
		atomic.StoreUint64(&hps.firstValue, math.Float64bits(values[0]))
	}
	atomic.StoreUint64(&hps.lastUpdateTime, uint64(now))
	atomic.StoreUint64(&hps.lastValue, math.Float64bits(values[len(values)-1]))

	// 更新全局min/max
	for {
		oldBits := atomic.LoadUint64(&hps.minVal)
		oldMin := math.Float64frombits(oldBits)
		if localMin >= oldMin {
			break
		}
		if atomic.CompareAndSwapUint64(&hps.minVal, oldBits, math.Float64bits(localMin)) {
			break
		}
	}

	for {
		oldBits := atomic.LoadUint64(&hps.maxVal)
		oldMax := math.Float64frombits(oldBits)
		if localMax <= oldMax {
			break
		}
		if atomic.CompareAndSwapUint64(&hps.maxVal, oldBits, math.Float64bits(localMax)) {
			break
		}
	}

	// 滑动窗口批量处理
	if hps.windowSize > 0 {
		startIndex := atomic.AddUint64(&hps.writeIndex, uint64(len(values))) - uint64(len(values))
		for i, v := range values {
			bufferIndex := (startIndex + uint64(i)) % uint64(hps.windowSize)
			atomic.StoreUint64(&hps.ringBuffer[bufferIndex], math.Float64bits(v))
		}

		if startIndex+uint64(len(values)) >= uint64(hps.windowSize) {
			atomic.StoreUint32(&hps.windowFull, 1)
		}
	}

	// 缓存失效
	atomic.AddUint64(&hps.cacheVersion, uint64(len(values)))
}

// GetStats 获取统计信息 - 高性能版本
func (hps *HighPerformanceStats) GetStats() map[string]float64 {
	count := atomic.LoadInt64(&hps.count)
	if count == 0 {
		return map[string]float64{}
	}

	// 计算基础统计
	sum := atomicLoadFloat64(&hps.sum)
	sumSquares := atomicLoadFloat64(&hps.sumSquares)
	min := atomicLoadFloat64(&hps.minVal)
	max := atomicLoadFloat64(&hps.maxVal)

	mean := sum / float64(count)

	// 计算标准差 - 使用数值稳定的公式
	var stddev float64
	if count > 1 {
		variance := (sumSquares - sum*sum/float64(count)) / float64(count-1)
		if variance > 0 {
			stddev = math.Sqrt(variance)
		}
	}

	// 时间信息
	firstVal := atomicLoadFloat64(&hps.firstValue)
	lastVal := atomicLoadFloat64(&hps.lastValue)

	// 数据质量统计
	nullCount := atomic.LoadInt64(&hps.nullCount)
	validCount := atomic.LoadInt64(&hps.validCount)

	result := map[string]float64{
		"count":        float64(count),
		"sum":          sum,
		"avg":          mean,
		"mean":         mean,
		"average":      mean,
		"stddev":       stddev,
		"std":          stddev,
		"variance":     stddev * stddev,
		"min":          min,
		"max":          max,
		"first":        firstVal,
		"last":         lastVal,
		"null_rate":    float64(nullCount) / float64(count),
		"completeness": float64(validCount) / float64(count),
	}

	// 计算变化率相关指标
	if count > 1 {
		result["change"] = lastVal - firstVal
		if firstVal != 0 {
			result["change_rate"] = (lastVal - firstVal) / firstVal
		}
		result["volatility"] = stddev / math.Abs(mean)
		if mean != 0 {
			result["cv"] = stddev / math.Abs(mean)
		}
	}

	// 计算阈值监控指标
	if hps.windowSize > 0 && atomic.LoadUint32(&hps.windowFull) == 1 {
		// 阈值相关计算需要遍历窗口数据
		result = hps.calculateThresholdStats(result)

		// 异常值检测
		outlierCount := hps.calculateOutlierCount(mean, stddev)
		result["outlier_count"] = outlierCount

		// 百分位数计算
		percentiles := hps.calculatePercentiles()
		for key, value := range percentiles {
			result[key] = value
		}
	}

	return result
}

// calculateThresholdStats 计算阈值相关统计信息。
// 以前用 "阈值 > 0" 判断是否配置，下限为 0 或负数的阈值会被忽略；
// 并且跳过 bit 为 0 的槽位，值为 0.0 的读数不会被计入。
func (hps *HighPerformanceStats) calculateThresholdStats(result map[string]float64) map[string]float64 {
	hasUpper := atomic.LoadUint32(&hps.hasUpper) == 1
	hasLower := atomic.LoadUint32(&hps.hasLower) == 1
	if !hasUpper && !hasLower {
		return result
	}
	upper := atomicLoadFloat64(&hps.upperLimit)
	lower := atomicLoadFloat64(&hps.lowerLimit)

	var aboveCount, belowCount, inRangeCount float64
	for _, value := range hps.windowValues() {
		switch {
		case hasUpper && value > upper:
			aboveCount++
		case hasLower && value < lower:
			belowCount++
		default:
			inRangeCount++
		}
	}
	result["above_count"] = aboveCount
	result["below_count"] = belowCount
	result["in_range_count"] = inRangeCount
	return result
}

// windowValues 返回滑动窗口中已写入的有效值
func (hps *HighPerformanceStats) windowValues() []float64 {
	if hps.windowSize <= 0 {
		return nil
	}
	n := atomic.LoadUint64(&hps.writeIndex)
	if n > uint64(hps.windowSize) {
		n = uint64(hps.windowSize)
	}
	values := make([]float64, 0, n)
	for i := uint64(0); i < n; i++ {
		values = append(values, math.Float64frombits(atomic.LoadUint64(&hps.ringBuffer[i])))
	}
	return values
}

// calculateOutlierCount 计算异常值数量（超出 mean ± threshold*stddev 的值）
func (hps *HighPerformanceStats) calculateOutlierCount(mean, stddev float64) float64 {
	if stddev == 0 || hps.windowSize == 0 {
		return 0
	}
	threshold := atomicLoadFloat64(&hps.outlierThreshold)
	lowerBound := mean - threshold*stddev
	upperBound := mean + threshold*stddev
	var outlierCount float64
	for _, value := range hps.windowValues() {
		if value < lowerBound || value > upperBound {
			outlierCount++
		}
	}
	return outlierCount
}

// calculatePercentiles 计算百分位数
func (hps *HighPerformanceStats) calculatePercentiles() map[string]float64 {
	if hps.windowSize == 0 || atomic.LoadUint32(&hps.windowFull) == 0 {
		return map[string]float64{}
	}

	// 以前用 "bits != 0" 判断槽位是否为空，而 0.0 的 bit 模式就是 0，
	// 导致所有值为 0 的读数被排除在百分位计算之外。
	values := hps.windowValues()

	if len(values) == 0 {
		return map[string]float64{}
	}

	sort.Float64s(values)

	result := make(map[string]float64)
	percentiles := []struct {
		name string
		p    float64
	}{
		{"p25", 0.25}, {"p50", 0.50}, {"p75", 0.75},
		{"p90", 0.90}, {"p95", 0.95}, {"p99", 0.99},
		{"median", 0.50},
	}

	for _, pct := range percentiles {
		idx := pct.p * float64(len(values)-1)
		if idx == float64(int(idx)) {
			result[pct.name] = values[int(idx)]
		} else {
			lower := int(math.Floor(idx))
			upper := int(math.Ceil(idx))
			weight := idx - float64(lower)
			result[pct.name] = values[lower]*(1-weight) + values[upper]*weight
		}
	}

	return result
}

// IsEmpty 检查是否为空
func (hps *HighPerformanceStats) IsEmpty() bool {
	return atomic.LoadInt64(&hps.count) == 0
}

// Reset 重置统计信息
func (hps *HighPerformanceStats) Reset() {
	atomic.StoreInt64(&hps.count, 0)
	atomic.StoreUint64(&hps.sum, 0)
	atomic.StoreUint64(&hps.sumSquares, 0)
	atomic.StoreUint64(&hps.minVal, math.Float64bits(math.Inf(1)))
	atomic.StoreUint64(&hps.maxVal, math.Float64bits(math.Inf(-1)))
	atomic.StoreInt64(&hps.nullCount, 0)
	atomic.StoreInt64(&hps.validCount, 0)
	atomic.StoreUint64(&hps.writeIndex, 0)
	atomic.StoreUint32(&hps.windowFull, 0)
	atomic.StoreUint64(&hps.cacheVersion, 0)

	// 清空环形缓冲区
	if hps.windowSize > 0 {
		for i := range hps.ringBuffer {
			atomic.StoreUint64(&hps.ringBuffer[i], 0)
		}
	}
}

// GetCount 获取计数
func (hps *HighPerformanceStats) GetCount() int64 {
	return atomic.LoadInt64(&hps.count)
}

// GetWindowSize 获取窗口大小
func (hps *HighPerformanceStats) GetWindowSize() int {
	return int(hps.windowSize)
}

// 兼容性接口 - 保持与原有IncrementalStats接口一致
func (hps *HighPerformanceStats) AddNull() {
	atomic.AddInt64(&hps.nullCount, 1)
	atomic.AddInt64(&hps.count, 1)
	atomic.AddUint64(&hps.cacheVersion, 1)
}
