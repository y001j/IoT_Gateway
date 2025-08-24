package utils

import (
	
	"log"
	"sync"
)

// TagsMigrationStrategy 标签迁移策略
type TagsMigrationStrategy struct {
	// 迁移统计
	conversions     int64
	failures        int64
	mu              sync.RWMutex
	
	// 性能配置
	enableCaching   bool
	maxCacheSize    int
	
	// 兼容性配置
	go124SafeMode   bool
}

// NewTagsMigrationStrategy 创建迁移策略
func NewTagsMigrationStrategy(go124Safe bool) *TagsMigrationStrategy {
	return &TagsMigrationStrategy{
		enableCaching: true,
		maxCacheSize:  1000,
		go124SafeMode: go124Safe,
	}
}

// ConvertMapToThreadSafeTags 安全地将map转换为ThreadSafeTags
func (tms *TagsMigrationStrategy) ConvertMapToThreadSafeTags(sourceMap map[string]string) *ThreadSafeTags {
	if sourceMap == nil {
		return NewThreadSafeTags()
	}
	
	// Go 1.24安全模式：避免直接map访问
	if tms.go124SafeMode {
		return tms.convertWithGo124Safety(sourceMap)
	}
	
	// 标准转换（适用于Go 1.23及以下）
	return NewThreadSafeTagsFromMap(sourceMap)
}

// convertWithGo124Safety Go 1.24安全转换
func (tms *TagsMigrationStrategy) convertWithGo124Safety(sourceMap map[string]string) *ThreadSafeTags {
	// Go 1.24安全解决方案：使用ShardedTags进行安全转换
	shardedTags := NewShardedTagsFromMap(sourceMap)
	allTags := shardedTags.GetAll()
	
	// 创建ThreadSafeTags并设置从ShardedTags获得的数据
	tags := NewThreadSafeTagsFromMap(allTags)
	
	tms.mu.Lock()
	tms.conversions++
	tms.mu.Unlock()
	
	log.Printf("INFO: Go 1.24安全转换 - 使用ShardedTags系统成功转换")
	return tags
}

// GetMigrationStats 获取迁移统计
func (tms *TagsMigrationStrategy) GetMigrationStats() (conversions, failures int64) {
	tms.mu.RLock()
	defer tms.mu.RUnlock()
	return tms.conversions, tms.failures
}

// BackwardCompatibilityAdapter 向后兼容适配器
type BackwardCompatibilityAdapter struct {
	threadSafeTags *ThreadSafeTags
}

// NewBackwardCompatibilityAdapter 创建兼容性适配器
func NewBackwardCompatibilityAdapter(tags *ThreadSafeTags) *BackwardCompatibilityAdapter {
	return &BackwardCompatibilityAdapter{
		threadSafeTags: tags,
	}
}

// AsMap 提供map接口兼容性
func (bca *BackwardCompatibilityAdapter) AsMap() map[string]string {
	if bca.threadSafeTags == nil {
		return make(map[string]string)
	}
	return bca.threadSafeTags.GetAll()
}

// SetFromMap 从map设置（批量操作）
func (bca *BackwardCompatibilityAdapter) SetFromMap(source map[string]string) {
	if bca.threadSafeTags == nil || source == nil {
		return
	}
	bca.threadSafeTags.SetMultiple(source)
}

// SafeTagsContainer 完整的安全标签解决方案
type SafeTagsContainer struct {
	tags    *ThreadSafeTags
	adapter *BackwardCompatibilityAdapter
	
	// 性能监控
	accessCount int64
	errorCount  int64
	mu          sync.RWMutex
}

// NewSafeTagsContainer 创建安全标签容器
func NewSafeTagsContainer() *SafeTagsContainer {
	tags := NewThreadSafeTags()
	return &SafeTagsContainer{
		tags:    tags,
		adapter: NewBackwardCompatibilityAdapter(tags),
	}
}

// NewSafeTagsContainerFromMap 从map安全创建
func NewSafeTagsContainerFromMap(sourceMap map[string]string, strategy *TagsMigrationStrategy) *SafeTagsContainer {
	container := NewSafeTagsContainer()
	
	if strategy != nil {
		container.tags = strategy.ConvertMapToThreadSafeTags(sourceMap)
		container.adapter = NewBackwardCompatibilityAdapter(container.tags)
	}
	
	return container
}

// GetTags 获取ThreadSafeTags实例
func (stc *SafeTagsContainer) GetTags() *ThreadSafeTags {
	return stc.tags
}

// GetAdapter 获取兼容性适配器
func (stc *SafeTagsContainer) GetAdapter() *BackwardCompatibilityAdapter {
	return stc.adapter
}

// GetStats 获取容器统计信息
func (stc *SafeTagsContainer) GetStats() map[string]interface{} {
	stc.mu.RLock()
	defer stc.mu.RUnlock()
	
	stats := stc.tags.GetStats()
	stats["access_count"] = stc.accessCount
	stats["error_count"] = stc.errorCount
	
	return stats
}

// 兼容性方法：提供类似原始map的接口
func (stc *SafeTagsContainer) Set(key, value string) {
	stc.mu.Lock()
	stc.accessCount++
	stc.mu.Unlock()
	
	stc.tags.Set(key, value)
}

func (stc *SafeTagsContainer) Get(key string) (string, bool) {
	stc.mu.Lock()
	stc.accessCount++
	stc.mu.Unlock()
	
	return stc.tags.Get(key)
}

func (stc *SafeTagsContainer) ToMap() map[string]string {
	stc.mu.Lock()
	stc.accessCount++
	stc.mu.Unlock()
	
	return stc.tags.GetAll()
}