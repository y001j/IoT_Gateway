package actions

import (
	"math"
	"sync"
	"time"
)

// IncrementalStats 增量统计计算器
type IncrementalStats struct {
	mu                sync.RWMutex
	count             int64
	sum               float64
	sumSquares        float64
	min               float64
	max               float64
	lastValue         float64
	lastUpdateTime    time.Time
	
	// 增量计算缓存
	meanCached        float64
	varianceCached    float64
	stdDevCached      float64
	cacheValid        bool
	
	// 滑动窗口支持
	windowSize        int
	values            []float64
	valueIndex        int
	windowFull        bool
}

// NewIncrementalStats 创建增量统计计算器
func NewIncrementalStats(windowSize int) *IncrementalStats {
	return &IncrementalStats{
		windowSize: windowSize,
		values:     make([]float64, windowSize),
		min:        math.Inf(1),
		max:        math.Inf(-1),
	}
}

// AddValue 添加数值并更新统计信息
func (s *IncrementalStats) AddValue(value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	currentTime := time.Now()
	
	if s.windowSize > 0 {
		// 滑动窗口模式
		s.addValueWindowed(value)
	} else {
		// 累积模式
		s.addValueCumulative(value)
	}
	
	s.lastValue = value
	s.lastUpdateTime = currentTime
	s.cacheValid = false // 使缓存失效
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
	s.min = math.Inf(1)
	s.max = math.Inf(-1)
	
	limit := s.windowSize
	if !s.windowFull {
		limit = int(s.count)
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
	
	mean := s.GetMean()
	s.varianceCached = (s.sumSquares - 2*mean*s.sum + float64(s.count)*mean*mean) / float64(s.count-1)
	
	return s.varianceCached
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
	s.lastUpdateTime = time.Time{}
	s.cacheValid = false
	s.valueIndex = 0
	s.windowFull = false
	
	// 清空窗口
	for i := range s.values {
		s.values[i] = 0
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
			s.varianceCached = (s.sumSquares - 2*mean*s.sum + float64(s.count)*mean*mean) / float64(s.count-1)
			s.stdDevCached = math.Sqrt(s.varianceCached)
		} else {
			s.varianceCached = 0
			s.stdDevCached = 0
		}
		
		s.cacheValid = true
	}
	
	result := map[string]float64{
		"count":    float64(s.count),
		"sum":      s.sum,
		"mean":     s.meanCached,
		"avg":      s.meanCached, // 别名
		"variance": s.varianceCached,
		"stddev":   s.stdDevCached,
		"min":      s.min,
		"max":      s.max,
		"last":     s.lastValue,
	}
	
	if s.count == 0 {
		result["min"] = 0
		result["max"] = 0
	}
	
	return result
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