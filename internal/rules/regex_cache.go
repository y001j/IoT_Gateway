package rules

import (
	"fmt"
	"regexp"
	"sync"
)

// RegexCache 全局正则表达式缓存
type RegexCache struct {
	cache map[string]*regexp.Regexp
	mutex sync.RWMutex
}

var globalRegexCache = &RegexCache{
	cache: make(map[string]*regexp.Regexp),
}

// GetCompiledRegex 获取编译后的正则表达式（全局缓存版本）
func GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	return globalRegexCache.GetCompiledRegex(pattern)
}

// GetCompiledRegex 获取编译后的正则表达式（带缓存）
func (rc *RegexCache) GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	// 先尝试读锁获取缓存
	rc.mutex.RLock()
	if compiled, exists := rc.cache[pattern]; exists {
		rc.mutex.RUnlock()
		return compiled, nil
	}
	rc.mutex.RUnlock()
	
	// 缓存不存在，编译正则表达式
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则表达式编译失败: %w", err)
	}
	
	// 写锁存储到缓存
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	
	// 双重检查，防止并发重复编译
	if existing, exists := rc.cache[pattern]; exists {
		return existing, nil
	}
	
	// 限制缓存大小，防止内存泄漏
	if len(rc.cache) >= 1000 {
		// 简单的LRU：清空缓存重新开始
		rc.cache = make(map[string]*regexp.Regexp)
	}
	
	rc.cache[pattern] = compiled
	return compiled, nil
}

// ClearCache 清空缓存
func (rc *RegexCache) ClearCache() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()
	rc.cache = make(map[string]*regexp.Regexp)
}

// CacheSize 获取缓存大小
func (rc *RegexCache) CacheSize() int {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()
	return len(rc.cache)
}

// MatchString 使用缓存的正则表达式进行匹配
func MatchString(pattern, s string) (bool, error) {
	compiled, err := GetCompiledRegex(pattern)
	if err != nil {
		return false, err
	}
	return compiled.MatchString(s), nil
}