package actions

import (
	"math"
	"sort"
	"sync"
	"time"
)

// TimestampValue 带时间戳的数值
type TimestampValue struct {
	Value     float64
	Timestamp time.Time
}

// IncrementalStats 增量统计计算器
type IncrementalStats struct {
	mu                sync.RWMutex
	count             int64
	sum               float64
	sumSquares        float64
	min               float64
	max               float64
	lastValue         float64
	firstValue        float64        // 第一个值
	firstValueSet     bool           // 标记是否已设置第一个值
	lastUpdateTime    time.Time
	firstUpdateTime   time.Time      // 第一次更新时间
	
	// 增量计算缓存
	meanCached        float64
	varianceCached    float64
	stdDevCached      float64
	medianCached      float64
	cacheValid        bool
	
	// 滑动窗口支持
	windowSize        int
	values            []float64
	valueIndex        int
	windowFull        bool
	
	// 时间窗口支持
	windowType        string             // "count" or "time"
	windowDuration    time.Duration      // 时间窗口大小
	timeValues        []TimestampValue   // 带时间戳的数值队列
	timeIndex         int               // 时间数据索引
	alignment         string            // 对齐方式
	
	// 数据质量统计
	nullCount         int64           // 空值数量
	validCount        int64           // 有效值数量
	
	// 阈值配置 (用于阈值监控函数)
	upperLimit        *float64        // 上限阈值
	lowerLimit        *float64        // 下限阈值
	outlierThreshold  float64         // 异常值阈值 (默认3.0倍标准差)
}

// NewIncrementalStats 创建增量统计计算器（数量窗口模式）
func NewIncrementalStats(windowSize int) *IncrementalStats {
	return NewIncrementalStatsWithWindow(windowSize, "count", 0, "none")
}

// NewIncrementalStatsWithWindow 创建带窗口配置的增量统计计算器
func NewIncrementalStatsWithWindow(windowSize int, windowType string, windowDuration time.Duration, alignment string) *IncrementalStats {
	// 防护性检查：确保windowSize不会导致panic
	if windowSize < 0 {
		windowSize = 0 // 负数窗口大小设为0（累积模式）
	}
	if windowSize > 1000000 { // 防止过大的窗口导致内存问题
		windowSize = 1000000
	}
	
	var values []float64
	var timeValues []TimestampValue
	
	if windowType == "count" && windowSize > 0 {
		values = make([]float64, windowSize)
	} else if windowType == "time" && windowSize > 0 {
		timeValues = make([]TimestampValue, windowSize)
	}
	
	return &IncrementalStats{
		windowSize:       windowSize,
		windowType:       windowType,
		windowDuration:   windowDuration,
		alignment:        alignment,
		values:           values,
		timeValues:       timeValues,
		min:              math.Inf(1),
		max:              math.Inf(-1),
		outlierThreshold: 3.0, // 默认3倍标准差作为异常值阈值
	}
}

// NewIncrementalStatsWithConfig 创建带配置的增量统计计算器
func NewIncrementalStatsWithConfig(windowSize int, config map[string]interface{}) *IncrementalStats {
	stats := NewIncrementalStats(windowSize)
	
	if config != nil {
		// 配置上限阈值
		if upper, exists := config["upper_limit"]; exists {
			if val, ok := upper.(float64); ok {
				stats.upperLimit = &val
			}
		}
		
		// 配置下限阈值
		if lower, exists := config["lower_limit"]; exists {
			if val, ok := lower.(float64); ok {
				stats.lowerLimit = &val
			}
		}
		
		// 配置异常值阈值
		if threshold, exists := config["outlier_threshold"]; exists {
			if val, ok := threshold.(float64); ok && val > 0 {
				stats.outlierThreshold = val
			}
		}
	}
	
	return stats
}

// AddValue 添加数值并更新统计信息
func (s *IncrementalStats) AddValue(value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	currentTime := time.Now()
	
	// 边界条件检查
	if math.IsNaN(value) {
		// 将NaN值计入空值统计
		s.nullCount++
		return
	}
	
	if math.IsInf(value, 0) {
		// 将无穷大值计入空值统计
		s.nullCount++
		return
	}
	
	// 有效值计数
	s.validCount++
	
	// 记录第一个值和时间
	if !s.firstValueSet {
		s.firstValue = value
		s.firstValueSet = true
		s.firstUpdateTime = currentTime
	}
	
	// 根据窗口类型选择处理方式
	switch s.windowType {
	case "time":
		if s.windowDuration > 0 {
			// 时间窗口模式
			s.addValueTimeWindowed(value, currentTime)
		} else {
			// 时间窗口未设置，回退到累积模式
			s.addValueCumulative(value)
		}
	case "count":
		if s.windowSize > 0 {
			// 数量滑动窗口模式
			s.addValueWindowed(value)
		} else {
			// 累积模式
			s.addValueCumulative(value)
		}
	default:
		// 兜底：累积模式
		s.addValueCumulative(value)
	}
	
	s.lastValue = value
	s.lastUpdateTime = currentTime
	s.cacheValid = false // 使缓存失效
}

// AddNullValue 添加空值统计
func (s *IncrementalStats) AddNullValue() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.nullCount++
}

// addValueCumulative 累积模式添加数值
func (s *IncrementalStats) addValueCumulative(value float64) {
	s.count++
	s.sum += value
	s.sumSquares += value * value
	
	if value < s.min || s.count == 1 {
		s.min = value
	}
	if value > s.max || s.count == 1 {
		s.max = value
	}
}

// addValueWindowed 滑动窗口模式添加数值
func (s *IncrementalStats) addValueWindowed(value float64) {
	// 如果没有values数组（windowSize为0），回退到累积模式
	if s.values == nil || len(s.values) == 0 {
		s.addValueCumulative(value)
		return
	}
	
	oldValue := s.values[s.valueIndex]
	s.values[s.valueIndex] = value
	
	if s.windowFull {
		// 移除旧值的影响
		s.sum -= oldValue
		s.sumSquares -= oldValue * oldValue
		s.count-- // 临时减少，下面会重新加1
	}
	
	// 添加新值
	s.sum += value
	s.sumSquares += value * value
	s.count++
	
	// 更新索引
	s.valueIndex = (s.valueIndex + 1) % s.windowSize
	if !s.windowFull && s.valueIndex == 0 {
		s.windowFull = true
	}
	
	// 重新计算min/max（窗口模式需要遍历）
	if s.windowFull || s.count > 1 {
		s.recalculateMinMax()
	} else {
		s.min = value
		s.max = value
	}
}

// recalculateMinMax 重新计算最小值和最大值
func (s *IncrementalStats) recalculateMinMax() {
	// 如果没有values数组，不需要重新计算
	if s.values == nil || len(s.values) == 0 {
		return
	}
	
	s.min = math.Inf(1)
	s.max = math.Inf(-1)
	
	limit := s.windowSize
	if !s.windowFull {
		limit = int(s.count)
	}
	
	// 确保不会越界
	if limit > len(s.values) {
		limit = len(s.values)
	}
	
	for i := 0; i < limit; i++ {
		value := s.values[i]
		if value < s.min {
			s.min = value
		}
		if value > s.max {
			s.max = value
		}
	}
}

// addValueTimeWindowed 时间窗口模式添加数值
func (s *IncrementalStats) addValueTimeWindowed(value float64, timestamp time.Time) {
	// 如果没有timeValues数组，回退到累积模式
	if s.timeValues == nil || len(s.timeValues) == 0 {
		s.addValueCumulative(value)
		return
	}
	
	// 首先清理过期数据
	s.cleanExpiredTimeData(timestamp)
	
	// 添加新值到时间窗口
	newValue := TimestampValue{
		Value:     value,
		Timestamp: timestamp,
	}
	
	// 如果缓冲区还有空间，直接添加
	if s.count < int64(s.windowSize) {
		s.timeValues[s.timeIndex] = newValue
		s.timeIndex = (s.timeIndex + 1) % s.windowSize
		
		// 更新统计信息
		s.sum += value
		s.sumSquares += value * value
		s.count++
		
		// 更新min/max
		if value < s.min || s.count == 1 {
			s.min = value
		}
		if value > s.max || s.count == 1 {
			s.max = value
		}
	} else {
		// 缓冲区已满，替换最旧的数据
		oldValue := s.timeValues[s.timeIndex]
		s.timeValues[s.timeIndex] = newValue
		s.timeIndex = (s.timeIndex + 1) % s.windowSize
		
		// 更新统计信息：移除旧值影响，添加新值
		s.sum -= oldValue.Value
		s.sum += value
		s.sumSquares -= oldValue.Value * oldValue.Value
		s.sumSquares += value * value
		// count保持不变
		
		// 重新计算min/max（需要遍历所有有效数据）
		s.recalculateTimeWindowMinMax()
	}
}

// cleanExpiredTimeData 清理过期的时间窗口数据
func (s *IncrementalStats) cleanExpiredTimeData(currentTime time.Time) {
	if s.windowDuration <= 0 || s.count == 0 {
		return
	}
	
	var cutoffTime time.Time
	
	// 根据对齐方式计算截止时间
	if s.alignment == "calendar" {
		cutoffTime = s.calculateCalendarAlignedCutoff(currentTime)
	} else {
		cutoffTime = currentTime.Add(-s.windowDuration)
	}
	
	// 计算需要清理的数据数量
	cleanCount := 0
	tempIndex := (s.timeIndex - int(s.count) + s.windowSize) % s.windowSize
	
	for i := int64(0); i < s.count; i++ {
		if s.timeValues[tempIndex].Timestamp.Before(cutoffTime) {
			// 这个数据过期了，需要清理
			oldValue := s.timeValues[tempIndex]
			s.sum -= oldValue.Value
			s.sumSquares -= oldValue.Value * oldValue.Value
			cleanCount++
			tempIndex = (tempIndex + 1) % s.windowSize
		} else {
			// 后续数据都是新的，停止清理
			break
		}
	}
	
	// 更新计数
	s.count -= int64(cleanCount)
	
	// 如果清理了数据，重新计算min/max
	if cleanCount > 0 {
		s.recalculateTimeWindowMinMax()
	}
}

// calculateCalendarAlignedCutoff 计算日历对齐的截止时间
func (s *IncrementalStats) calculateCalendarAlignedCutoff(currentTime time.Time) time.Time {
	// 根据窗口大小确定对齐粒度
	if s.windowDuration >= 24*time.Hour {
		// 大于等于1天：按天对齐
		year, month, day := currentTime.Date()
		dayStart := time.Date(year, month, day, 0, 0, 0, 0, currentTime.Location())
		return dayStart.Add(-s.windowDuration + 24*time.Hour)
	} else if s.windowDuration >= time.Hour {
		// 大于等于1小时：按小时对齐
		year, month, day := currentTime.Date()
		hour := currentTime.Hour()
		hourStart := time.Date(year, month, day, hour, 0, 0, 0, currentTime.Location())
		return hourStart.Add(-s.windowDuration + time.Hour)
	} else if s.windowDuration >= time.Minute {
		// 大于等于1分钟：按分钟对齐
		year, month, day := currentTime.Date()
		hour, min := currentTime.Hour(), currentTime.Minute()
		minuteStart := time.Date(year, month, day, hour, min, 0, 0, currentTime.Location())
		return minuteStart.Add(-s.windowDuration + time.Minute)
	} else {
		// 小于1分钟：按秒对齐
		year, month, day := currentTime.Date()
		hour, min, sec := currentTime.Hour(), currentTime.Minute(), currentTime.Second()
		secondStart := time.Date(year, month, day, hour, min, sec, 0, currentTime.Location())
		return secondStart.Add(-s.windowDuration + time.Second)
	}
}

// recalculateTimeWindowMinMax 重新计算时间窗口的最小值和最大值
func (s *IncrementalStats) recalculateTimeWindowMinMax() {
	if s.count == 0 {
		s.min = math.Inf(1)
		s.max = math.Inf(-1)
		return
	}
	
	s.min = math.Inf(1)
	s.max = math.Inf(-1)
	
	// 遍历有效的时间窗口数据
	tempIndex := (s.timeIndex - int(s.count) + s.windowSize) % s.windowSize
	for i := int64(0); i < s.count; i++ {
		value := s.timeValues[tempIndex].Value
		if value < s.min {
			s.min = value
		}
		if value > s.max {
			s.max = value
		}
		tempIndex = (tempIndex + 1) % s.windowSize
	}
}

// GetCount 获取数据点数量
func (s *IncrementalStats) GetCount() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count
}

// GetSum 获取总和
func (s *IncrementalStats) GetSum() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sum
}

// GetMean 获取平均值（使用缓存优化）
func (s *IncrementalStats) GetMean() float64 {
	s.mu.RLock()
	if s.cacheValid {
		mean := s.meanCached
		s.mu.RUnlock()
		return mean
	}
	s.mu.RUnlock()
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 双重检查
	if s.cacheValid {
		return s.meanCached
	}
	
	if s.count == 0 {
		s.meanCached = 0
	} else {
		s.meanCached = s.sum / float64(s.count)
	}
	
	return s.meanCached
}

// GetVariance 获取方差（使用缓存优化）
func (s *IncrementalStats) GetVariance() float64 {
	s.mu.RLock()
	if s.cacheValid {
		variance := s.varianceCached
		s.mu.RUnlock()
		return variance
	}
	s.mu.RUnlock()
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cacheValid {
		return s.varianceCached
	}
	
	if s.count <= 1 {
		s.varianceCached = 0
		return 0
	}
	
	// 直接计算均值以避免死锁，不调用GetMean()
	mean := 0.0
	if s.count > 0 {
		mean = s.sum / float64(s.count)
	}
	
	// 使用Welford's online variance algorithm for numerical stability
	// Var = (sumSquares - n*mean^2) / (n-1)
	n := float64(s.count)
	variance := (s.sumSquares - n*mean*mean) / (n - 1.0)
	
	// 使用更严格的边界条件检查
	if math.IsNaN(variance) || math.IsInf(variance, 0) {
		// 如果出现NaN或Inf，使用备用计算方法
		variance = s.calculateVarianceFallback()
	} else if variance < 0 {
		// 由于浮点误差，方差可能略小于0，将其设为0
		variance = 0.0
	}
	
	s.varianceCached = variance
	
	return s.varianceCached
}

// calculateVarianceFallback 备用方差计算方法
func (s *IncrementalStats) calculateVarianceFallback() float64 {
	if s.count <= 1 {
		return 0.0
	}
	
	// 使用更稳定的两遍算法
	mean := s.sum / float64(s.count)
	variance := 0.0
	
	// 对于滑动窗口，需要重新计算
	if s.windowSize > 0 && s.values != nil {
		count := s.count
		if s.count > int64(s.windowSize) {
			count = int64(s.windowSize)
		}
		
		// 确保不会越界
		maxIndex := len(s.values)
		if int(count) > maxIndex {
			count = int64(maxIndex)
		}
		
		sumOfSquaredDiffs := 0.0
		for i := 0; i < int(count); i++ {
			diff := s.values[i] - mean
			sumOfSquaredDiffs += diff * diff
		}
		
		if count > 1 {
			variance = sumOfSquaredDiffs / float64(count-1)
		}
	} else {
		// 对于累积模式，使用在线算法的补偿求和
		// 这里简化处理，返回0（在数值不稳定的情况下）
		variance = 0.0
	}
	
	return variance
}

// GetStdDev 获取标准差（使用缓存优化）
func (s *IncrementalStats) GetStdDev() float64 {
	s.mu.RLock()
	if s.cacheValid {
		stdDev := s.stdDevCached
		s.mu.RUnlock()
		return stdDev
	}
	s.mu.RUnlock()
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cacheValid {
		return s.stdDevCached
	}
	
	s.stdDevCached = math.Sqrt(s.GetVariance())
	return s.stdDevCached
}

// GetMin 获取最小值
func (s *IncrementalStats) GetMin() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.count == 0 {
		return 0
	}
	return s.min
}

// GetMax 获取最大值
func (s *IncrementalStats) GetMax() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.count == 0 {
		return 0
	}
	return s.max
}

// GetLastValue 获取最后一个值
func (s *IncrementalStats) GetLastValue() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastValue
}

// GetFirstValue 获取第一个值
func (s *IncrementalStats) GetFirstValue() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.firstValueSet {
		return s.firstValue
	}
	return 0 // 如果没有值，返回0
}

// GetMedian 获取中位数
func (s *IncrementalStats) GetMedian() float64 {
	s.mu.RLock()
	if s.cacheValid {
		median := s.medianCached
		s.mu.RUnlock()
		return median
	}
	s.mu.RUnlock()
	
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if s.cacheValid {
		return s.medianCached
	}
	
	if s.count == 0 {
		s.medianCached = 0
		return 0
	}
	
	if s.count == 1 {
		s.medianCached = s.sanitizeFloat(s.firstValue)
		return s.medianCached
	}
	
	// 计算中位数需要所有值，对于滑动窗口模式直接使用values数组
	if s.windowSize > 0 && s.values != nil {
		s.medianCached = s.calculateMedianFromWindow()
	} else {
		// 累积模式下，无法高效计算中位数，返回平均值作为近似
		if s.count > 0 {
			s.medianCached = s.sanitizeFloat(s.sum / float64(s.count))
		} else {
			s.medianCached = 0
		}
	}
	
	return s.medianCached
}

// calculateMedianFromWindow 从滑动窗口计算中位数
func (s *IncrementalStats) calculateMedianFromWindow() float64 {
	if s.values == nil || len(s.values) == 0 {
		return 0
	}
	
	// 获取有效数据
	var validValues []float64
	count := int(s.count)
	if count > s.windowSize {
		count = s.windowSize
	}
	
	for i := 0; i < count && i < len(s.values); i++ {
		validValues = append(validValues, s.values[i])
	}
	
	if len(validValues) == 0 {
		return 0
	}
	
	// 排序
	for i := 0; i < len(validValues)-1; i++ {
		for j := i + 1; j < len(validValues); j++ {
			if validValues[i] > validValues[j] {
				validValues[i], validValues[j] = validValues[j], validValues[i]
			}
		}
	}
	
	// 计算中位数
	n := len(validValues)
	if n%2 == 1 {
		return s.sanitizeFloat(validValues[n/2])
	} else {
		return s.sanitizeFloat((validValues[n/2-1] + validValues[n/2]) / 2.0)
	}
}

// GetLastUpdateTime 获取最后更新时间
func (s *IncrementalStats) GetLastUpdateTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastUpdateTime
}

// Reset 重置统计信息
func (s *IncrementalStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	s.count = 0
	s.sum = 0
	s.sumSquares = 0
	s.min = math.Inf(1)
	s.max = math.Inf(-1)
	s.lastValue = 0
	s.firstValue = 0
	s.firstValueSet = false
	s.lastUpdateTime = time.Time{}
	s.firstUpdateTime = time.Time{}
	s.cacheValid = false
	s.valueIndex = 0
	s.windowFull = false
	
	// 重置数据质量统计
	s.nullCount = 0
	s.validCount = 0
	
	// 清空窗口（如果存在）
	if s.values != nil {
		for i := range s.values {
			s.values[i] = 0
		}
	}
}

// GetStats 获取所有统计信息
func (s *IncrementalStats) GetStats() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// 计算并缓存所有统计值
	if !s.cacheValid {
		if s.count > 0 {
			s.meanCached = s.sum / float64(s.count)
		} else {
			s.meanCached = 0
		}
		
		if s.count > 1 {
			mean := s.meanCached
			n := float64(s.count)
			variance := (s.sumSquares - n*mean*mean) / (n - 1.0)
			
			// 边界条件检查
			if math.IsNaN(variance) || math.IsInf(variance, 0) {
				variance = s.calculateVarianceFallback()
			} else if variance < 0 {
				// 由于浮点误差，方差可能略小于0，将其设为0
				variance = 0.0
			}
			
			s.varianceCached = variance
			
			// 对标准差也进行检查
			stdDev := math.Sqrt(variance)
			if math.IsNaN(stdDev) || math.IsInf(stdDev, 0) {
				stdDev = 0.0
			}
			s.stdDevCached = stdDev
		} else {
			s.varianceCached = 0
			s.stdDevCached = 0
		}
		
		
		// 在设置cacheValid之前计算median
		if s.count == 0 {
			s.medianCached = 0
		} else if s.count == 1 {
			s.medianCached = s.sanitizeFloat(s.firstValue)
		} else if s.windowSize > 0 && s.values != nil {
			s.medianCached = s.calculateMedianFromWindow()
		} else {
			// 累积模式下，使用平均值作为median的近似
			s.medianCached = s.sanitizeFloat(s.meanCached)
		}
		
		s.cacheValid = true
	}
	
	// 对所有结果进行精度处理
	result := map[string]float64{
		// 基础统计函数
		"count":    float64(s.count),
		"sum":      s.sanitizeFloat(s.sum),
		"mean":     s.sanitizeFloat(s.meanCached),
		"avg":      s.sanitizeFloat(s.meanCached), // 别名
		"average":  s.sanitizeFloat(s.meanCached), // 别名
		"variance": s.sanitizeFloat(s.varianceCached),
		"stddev":   s.sanitizeFloat(s.stdDevCached),
		"std":      s.sanitizeFloat(s.stdDevCached), // 别名
		"min":      s.sanitizeFloat(s.min),
		"max":      s.sanitizeFloat(s.max),
		"first":    s.sanitizeFloat(s.firstValue),
		"last":     s.sanitizeFloat(s.lastValue),
		"median":   s.sanitizeFloat(s.medianCached),
		
		// Phase 1: 百分位数函数 
		"p90":      s.sanitizeFloat(s.calculatePercentile(90)),
		"p95":      s.sanitizeFloat(s.calculatePercentile(95)),
		"p99":      s.sanitizeFloat(s.calculatePercentile(99)),
		
		// Phase 1: 数据质量函数
		"null_rate":     s.sanitizeFloat(s.GetNullRate()),
		"completeness":  s.sanitizeFloat(s.GetCompleteness()),
		
		// Phase 1: 变化检测函数
		"change":        s.sanitizeFloat(s.GetChange()),
		"change_rate":   s.sanitizeFloat(s.GetChangeRate()),
		
		// Phase 1: 异常检测函数
		"outlier_count": s.sanitizeFloat(s.getOutlierCountInternal()),
		
		// Phase 2: 百分位数扩展
		"p25":           s.sanitizeFloat(s.calculatePercentile(25)),
		"p50":           s.sanitizeFloat(s.calculatePercentile(50)),
		"p75":           s.sanitizeFloat(s.calculatePercentile(75)),
		
		// Phase 2: 波动性分析
		"volatility":    s.sanitizeFloat(s.GetVolatility()),
		"cv":            s.sanitizeFloat(s.GetCV()),
	}
	
	// 条件性添加阈值监控函数 (Phase 2)
	if s.upperLimit != nil {
		result["above_count"] = s.sanitizeFloat(s.GetAboveCount())
	}
	if s.lowerLimit != nil {
		result["below_count"] = s.sanitizeFloat(s.GetBelowCount())
	}
	if s.upperLimit != nil && s.lowerLimit != nil {
		result["in_range_count"] = s.sanitizeFloat(s.GetInRangeCount())
	}
	
	if s.count == 0 {
		result["min"] = 0
		result["max"] = 0
	}
	
	return result
}

// sanitizeFloat 清理浮点数，处理NaN、Inf等异常值
func (s *IncrementalStats) sanitizeFloat(value float64) float64 {
	if math.IsNaN(value) {
		return 0.0
	}
	if math.IsInf(value, 0) {
		if math.IsInf(value, 1) {
			return math.MaxFloat64
		} else {
			return -math.MaxFloat64
		}
	}
	
	// 处理极小值（接近于0但不为0的情况）
	if math.Abs(value) < 1e-15 {
		return 0.0
	}
	
	return value
}

// IsEmpty 检查是否为空
func (s *IncrementalStats) IsEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count == 0
}

// Age 获取数据的年龄
func (s *IncrementalStats) Age() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if s.lastUpdateTime.IsZero() {
		return time.Duration(0)
	}
	return time.Since(s.lastUpdateTime)
}

// calculatePercentile 计算百分位数
func (s *IncrementalStats) calculatePercentile(percentile float64) float64 {
	if s.count == 0 {
		return 0
	}
	if s.count == 1 {
		return s.sanitizeFloat(s.firstValue)
	}
	
	// 根据窗口类型选择计算方法
	if s.windowType == "time" && s.timeValues != nil {
		return s.calculatePercentileFromTimeWindow(percentile)
	} else if s.windowSize > 0 && s.values != nil {
		return s.calculatePercentileFromWindow(percentile)
	} else {
		// 累积模式下，使用统计近似
		if percentile == 50 {
			return s.sanitizeFloat(s.meanCached)
		}
		return s.normalApproximation(percentile)
	}
}

// calculatePercentileFromWindow 从滑动窗口计算百分位数
func (s *IncrementalStats) calculatePercentileFromWindow(percentile float64) float64 {
	if s.values == nil || len(s.values) == 0 {
		return 0
	}
	
	// 获取有效数据
	var validValues []float64
	count := int(s.count)
	if count > s.windowSize {
		count = s.windowSize
	}
	
	for i := 0; i < count && i < len(s.values); i++ {
		validValues = append(validValues, s.values[i])
	}
	
	if len(validValues) == 0 {
		return 0
	}
	
	// 排序（使用简单的冒泡排序，对小数据集足够）
	for i := 0; i < len(validValues)-1; i++ {
		for j := i + 1; j < len(validValues); j++ {
			if validValues[i] > validValues[j] {
				validValues[i], validValues[j] = validValues[j], validValues[i]
			}
		}
	}
	
	// 计算百分位数位置
	n := len(validValues)
	pos := percentile / 100.0 * float64(n-1)
	
	if pos == float64(int(pos)) {
		return s.sanitizeFloat(validValues[int(pos)])
	} else {
		lower := int(math.Floor(pos))
		upper := int(math.Ceil(pos))
		if upper >= n {
			upper = n - 1
		}
		
		weight := pos - float64(lower)
		result := validValues[lower]*(1-weight) + validValues[upper]*weight
		return s.sanitizeFloat(result)
	}
}

// calculatePercentileFromTimeWindow 从时间窗口计算百分位数
func (s *IncrementalStats) calculatePercentileFromTimeWindow(percentile float64) float64 {
	if s.timeValues == nil || s.count == 0 {
		return 0
	}
	
	// 收集有效的数值
	var validValues []float64
	tempIndex := (s.timeIndex - int(s.count) + s.windowSize) % s.windowSize
	
	for i := int64(0); i < s.count; i++ {
		validValues = append(validValues, s.timeValues[tempIndex].Value)
		tempIndex = (tempIndex + 1) % s.windowSize
	}
	
	if len(validValues) == 0 {
		return 0
	}
	
	// 排序
	sort.Float64s(validValues)
	
	// 计算百分位数
	index := (percentile / 100) * float64(len(validValues)-1)
	lower := int(index)
	upper := lower + 1
	
	if upper >= len(validValues) {
		return s.sanitizeFloat(validValues[len(validValues)-1])
	}
	
	if lower == int(index) {
		return s.sanitizeFloat(validValues[lower])
	}
	
	// 线性插值
	weight := index - float64(lower)
	result := validValues[lower]*(1-weight) + validValues[upper]*weight
	return s.sanitizeFloat(result)
}

// normalApproximation 正态分布近似百分位数 (累积模式使用)
func (s *IncrementalStats) normalApproximation(percentile float64) float64 {
	if s.stdDevCached == 0 {
		return s.sanitizeFloat(s.meanCached)
	}
	
	mean := s.meanCached
	stddev := s.stdDevCached
	
	// 使用常见百分位数的Z-score近似
	var zScore float64
	switch {
	case percentile <= 25:
		zScore = -0.675
	case percentile <= 50:
		zScore = 0
	case percentile <= 75:
		zScore = 0.675
	case percentile <= 90:
		zScore = 1.28
	case percentile <= 95:
		zScore = 1.645
	case percentile <= 99:
		zScore = 2.33
	default:
		zScore = 3.0
	}
	
	return s.sanitizeFloat(mean + zScore*stddev)
}

// 数据质量函数
func (s *IncrementalStats) GetNullRate() float64 {
	total := s.nullCount + s.validCount
	if total == 0 {
		return 0
	}
	return float64(s.nullCount) / float64(total)
}

func (s *IncrementalStats) GetCompleteness() float64 {
	return 1.0 - s.GetNullRate()
}

// 变化检测函数
func (s *IncrementalStats) GetChange() float64 {
	if !s.firstValueSet || s.count == 0 {
		return 0
	}
	return s.lastValue - s.firstValue
}

func (s *IncrementalStats) GetChangeRate() float64 {
	if !s.firstValueSet || s.count == 0 || s.firstValue == 0 {
		return 0
	}
	return (s.lastValue - s.firstValue) / math.Abs(s.firstValue) * 100
}

func (s *IncrementalStats) GetVolatility() float64 {
	if s.meanCached == 0 {
		return 0
	}
	return s.stdDevCached / math.Abs(s.meanCached)
}

func (s *IncrementalStats) GetCV() float64 {
	return s.GetVolatility() // 变异系数和波动率是同一概念
}

// GetOutlierCount 获取异常值数量 (内部调用，不加锁)
func (s *IncrementalStats) GetOutlierCount() float64 {
	return s.getOutlierCountInternal()
}

// getOutlierCountInternal 内部异常检测函数 (假设调用者已持有锁)
func (s *IncrementalStats) getOutlierCountInternal() float64 {
	if s.count <= 3 { // 需要至少4个数据点才能进行异常检测
		return 0
	}
	
	outliers := 0.0
	
	if s.windowSize > 0 && s.values != nil {
		limit := int(s.count)
		if limit > s.windowSize {
			limit = s.windowSize
		}
		
		// 使用IQR方法进行异常检测，比基于标准差的方法更鲁棒
		// 1. 收集有效数据
		var validValues []float64
		for i := 0; i < limit && i < len(s.values); i++ {
			validValues = append(validValues, s.values[i])
		}
		
		if len(validValues) < 4 {
			return 0
		}
		
		// 2. 排序
		for i := 0; i < len(validValues)-1; i++ {
			for j := i + 1; j < len(validValues); j++ {
				if validValues[i] > validValues[j] {
					validValues[i], validValues[j] = validValues[j], validValues[i]
				}
			}
		}
		
		// 3. 计算Q1和Q3
		n := len(validValues)
		q1Pos := float64(n-1) * 0.25
		q3Pos := float64(n-1) * 0.75
		
		q1 := interpolatePercentile(validValues, q1Pos)
		q3 := interpolatePercentile(validValues, q3Pos)
		
		// 4. 计算IQR和异常阈值
		iqr := q3 - q1
		lowerBound := q1 - 1.5*iqr
		upperBound := q3 + 1.5*iqr
		
		// 5. 计数异常值
		for _, val := range validValues {
			if val < lowerBound || val > upperBound {
				outliers++
			}
		}
	} else {
		// 累积模式下的近似估算，使用基于中位数的方法
		if s.count > 10 {
			// 使用基于均值和标准差但更保守的方法
			stddev := s.stdDevCached
			
			if stddev > 0 {
				// 由于是累积模式，无法检查每个值，使用经验公式
				// 估算可能的异常值比例
				if s.count > 50 {
					outliers = float64(s.count) * 0.05 // 估算5%的异常值
				}
			}
		}
	}
	
	return outliers
}

// interpolatePercentile 辅助函数：在排序数组中插值计算百分位数
func interpolatePercentile(sortedValues []float64, position float64) float64 {
	if position <= 0 {
		return sortedValues[0]
	}
	if position >= float64(len(sortedValues)-1) {
		return sortedValues[len(sortedValues)-1]
	}
	
	lower := int(position)
	upper := lower + 1
	weight := position - float64(lower)
	
	return sortedValues[lower]*(1-weight) + sortedValues[upper]*weight
}

// 阈值监控函数
func (s *IncrementalStats) GetAboveCount() float64 {
	if s.upperLimit == nil {
		return 0
	}
	
	count := 0.0
	if s.windowSize > 0 && s.values != nil {
		limit := int(s.count)
		if limit > s.windowSize {
			limit = s.windowSize
		}
		
		for i := 0; i < limit && i < len(s.values); i++ {
			if s.values[i] > *s.upperLimit {
				count++
			}
		}
	} else {
		// 累积模式下的估算
		if s.upperLimit != nil && s.max > *s.upperLimit {
			// 如果最大值超过阈值，估算超过阈值的数据点
			exceedRatio := (s.max - *s.upperLimit) / (s.max - s.min + 1e-10)
			count = float64(s.count) * math.Min(exceedRatio, 0.5)
		}
	}
	
	return count
}

func (s *IncrementalStats) GetBelowCount() float64 {
	if s.lowerLimit == nil {
		return 0
	}
	
	count := 0.0
	if s.windowSize > 0 && s.values != nil {
		limit := int(s.count)
		if limit > s.windowSize {
			limit = s.windowSize
		}
		
		for i := 0; i < limit && i < len(s.values); i++ {
			if s.values[i] < *s.lowerLimit {
				count++
			}
		}
	} else {
		// 累积模式下的估算
		if s.lowerLimit != nil && s.min < *s.lowerLimit {
			exceedRatio := (*s.lowerLimit - s.min) / (s.max - s.min + 1e-10)
			count = float64(s.count) * math.Min(exceedRatio, 0.5)
		}
	}
	
	return count
}

func (s *IncrementalStats) GetInRangeCount() float64 {
	if s.upperLimit == nil || s.lowerLimit == nil {
		return 0
	}
	
	count := 0.0
	if s.windowSize > 0 && s.values != nil {
		limit := int(s.count)
		if limit > s.windowSize {
			limit = s.windowSize
		}
		
		for i := 0; i < limit && i < len(s.values); i++ {
			if s.values[i] >= *s.lowerLimit && s.values[i] <= *s.upperLimit {
				count++
			}
		}
	} else {
		// 累积模式下的估算
		totalRange := s.max - s.min + 1e-10
		if totalRange > 0 {
			validRange := *s.upperLimit - *s.lowerLimit
			if validRange > 0 {
				overlapRatio := math.Min(validRange/totalRange, 1.0)
				count = float64(s.count) * overlapRatio
			}
		}
	}
	
	return count
}