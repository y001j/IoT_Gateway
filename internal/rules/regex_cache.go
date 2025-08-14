package rules

import (
	"fmt"
	"regexp"
	"sync"
	"time"
)

// RegexCacheEntry 缓存条目，包含使用时间和访问计数
type RegexCacheEntry struct {
	regex       *regexp.Regexp
	lastUsed    time.Time
	accessCount int64
}

// RegexCache 增强的正则表达式缓存，支持LRU和使用频率统计
type RegexCache struct {
	cache    map[string]*RegexCacheEntry
	mutex    sync.RWMutex
	maxSize  int
	hitCount int64
	requests int64
}

var globalRegexCache = &RegexCache{
	cache:   make(map[string]*RegexCacheEntry),
	maxSize: 1000, // 可配置的最大缓存大小
}

// GetCompiledRegex 获取编译后的正则表达式（全局缓存版本）
func GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	return globalRegexCache.GetCompiledRegex(pattern)
}

// GetCompiledRegex 获取编译后的正则表达式（带增强缓存）
func (rc *RegexCache) GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	
	// 增加请求计数
	rc.requests++
	
	// 检查缓存命中
	if entry, exists := rc.cache[pattern]; exists {
		// 更新访问信息
		entry.lastUsed = time.Now()
		entry.accessCount++
		rc.hitCount++
		return entry.regex, nil
	}
	
	// 缓存未命中，编译正则表达式
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则表达式编译失败: %w", err)
	}
	
	// 检查缓存大小并执行智能清理
	if len(rc.cache) >= rc.maxSize {
		rc.evictLeastUsed()
	}
	
	// 添加到缓存
	rc.cache[pattern] = &RegexCacheEntry{
		regex:       compiled,
		lastUsed:    time.Now(),
		accessCount: 1,
	}
	
	return compiled, nil
}

// evictLeastUsed 智能清理最少使用的缓存条目
func (rc *RegexCache) evictLeastUsed() {
	if len(rc.cache) == 0 {
		return
	}
	
	now := time.Now()
	leastUsedPattern := ""
	minScore := int64(1<<63 - 1) // 最大int64值
	
	// 计算每个条目的权重得分（结合访问频率和时间）
	for pattern, entry := range rc.cache {
		// 时间权重：最近使用时间距离现在的秒数
		timeWeight := int64(now.Sub(entry.lastUsed).Seconds())
		
		// 频率权重：访问次数的倒数（访问越多，权重越小）
		freqWeight := int64(1000) // 基础权重
		if entry.accessCount > 0 {
			freqWeight = 1000 / entry.accessCount
		}
		
		// 综合得分：时间权重 + 频率权重（得分越高越容易被清理）
		score := timeWeight + freqWeight
		
		if score < minScore || leastUsedPattern == "" {
			minScore = score
			leastUsedPattern = pattern
		}
	}
	
	// 删除得分最高的条目（最少使用且最久未用）
	if leastUsedPattern != "" {
		delete(rc.cache, leastUsedPattern)
	}
	
	// 如果缓存仍然太大，继续清理（清理25%的条目）
	if len(rc.cache) > rc.maxSize*3/4 {
		toRemove := len(rc.cache) / 4
		removed := 0
		cutoffTime := now.Add(-1 * time.Hour) // 1小时前的条目优先清理
		
		for pattern, entry := range rc.cache {
			if removed >= toRemove {
				break
			}
			if entry.lastUsed.Before(cutoffTime) && entry.accessCount < 5 {
				delete(rc.cache, pattern)
				removed++
			}
		}
	}
}

// ClearCache 清空缓存
func (rc *RegexCache) ClearCache() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.cache = make(map[string]*RegexCacheEntry)
	rc.hitCount = 0
	rc.requests = 0
}

// CacheSize 获取缓存大小
func (rc *RegexCache) CacheSize() int {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	return len(rc.cache)
}

// GetCacheStats 获取缓存统计信息
func (rc *RegexCache) GetCacheStats() (size int, hitRate float64, requests int64, hits int64) {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	
	size = len(rc.cache)
	requests = rc.requests
	hits = rc.hitCount
	
	if requests > 0 {
		hitRate = float64(hits) / float64(requests)
	}
	
	return size, hitRate, requests, hits
}

// GetTopPatterns 获取访问最频繁的正则模式（用于监控和调优）
func (rc *RegexCache) GetTopPatterns(limit int) []string {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	
	type patternCount struct {
		pattern string
		count   int64
	}
	
	var patterns []patternCount
	for pattern, entry := range rc.cache {
		patterns = append(patterns, patternCount{
			pattern: pattern,
			count:   entry.accessCount,
		})
	}
	
	// 简单排序（冒泡排序，适用于小数据集）
	for i := 0; i < len(patterns)-1; i++ {
		for j := 0; j < len(patterns)-i-1; j++ {
			if patterns[j].count < patterns[j+1].count {
				patterns[j], patterns[j+1] = patterns[j+1], patterns[j]
			}
		}
	}
	
	// 限制返回数量
	if limit > 0 && limit < len(patterns) {
		patterns = patterns[:limit]
	}
	
	result := make([]string, len(patterns))
	for i, p := range patterns {
		result[i] = p.pattern
	}
	
	return result
}

// SetMaxSize 设置最大缓存大小
func (rc *RegexCache) SetMaxSize(size int) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.maxSize = size
}

// MatchString 使用缓存的正则表达式进行匹配
func MatchString(pattern, s string) (bool, error) {
	compiled, err := GetCompiledRegex(pattern)
	if err != nil {
		return false, err
	}
	return compiled.MatchString(s), nil
}

// GetGlobalCacheStats 获取全局缓存统计信息（便于外部监控）
func GetGlobalCacheStats() (size int, hitRate float64, requests int64, hits int64) {
	return globalRegexCache.GetCacheStats()
}