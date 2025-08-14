package utils

import (
	"encoding/json"
	"sync"
)

// ThreadSafeTags 线程安全的标签容器，替代map[string]string
// 专门为Go 1.24 maps.fatal问题设计的解决方案
type ThreadSafeTags struct {
	mu   sync.RWMutex
	data map[string]string
	
	// 性能优化：缓存常用操作结果
	cachedJSON   []byte
	cacheValid   bool
	cacheMutex   sync.RWMutex
}

// NewThreadSafeTags 创建新的线程安全标签容器
func NewThreadSafeTags() *ThreadSafeTags {
	return &ThreadSafeTags{
		data: make(map[string]string),
	}
}

// NewThreadSafeTagsFromMap 从现有map创建（一次性安全转换）
func NewThreadSafeTagsFromMap(sourceMap map[string]string) *ThreadSafeTags {
	tags := &ThreadSafeTags{
		data: make(map[string]string),
	}
	
	// 使用写锁保护初始化
	tags.mu.Lock()
	defer tags.mu.Unlock()
	
	// 安全地复制数据（在锁保护下）
	if sourceMap != nil {
		for k, v := range sourceMap {
			tags.data[k] = v
		}
	}
	
	return tags
}

// Set 设置标签值
func (t *ThreadSafeTags) Set(key, value string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	t.data[key] = value
	t.invalidateCache()
}

// Get 获取标签值
func (t *ThreadSafeTags) Get(key string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	value, exists := t.data[key]
	return value, exists
}

// GetAll 获取所有标签的安全副本
func (t *ThreadSafeTags) GetAll() map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	// 创建安全副本
	result := make(map[string]string, len(t.data))
	for k, v := range t.data {
		result[k] = v
	}
	return result
}

// GetKeys 获取所有键
func (t *ThreadSafeTags) GetKeys() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	keys := make([]string, 0, len(t.data))
	for k := range t.data {
		keys = append(keys, k)
	}
	return keys
}

// Len 获取标签数量
func (t *ThreadSafeTags) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	return len(t.data)
}

// Delete 删除标签
func (t *ThreadSafeTags) Delete(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	delete(t.data, key)
	t.invalidateCache()
}

// Clear 清空所有标签
func (t *ThreadSafeTags) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	t.data = make(map[string]string)
	t.invalidateCache()
}

// MarshalJSON 实现JSON序列化
func (t *ThreadSafeTags) MarshalJSON() ([]byte, error) {
	// 检查缓存
	t.cacheMutex.RLock()
	if t.cacheValid && len(t.cachedJSON) > 0 {
		cached := make([]byte, len(t.cachedJSON))
		copy(cached, t.cachedJSON)
		t.cacheMutex.RUnlock()
		return cached, nil
	}
	t.cacheMutex.RUnlock()
	
	// 获取数据副本
	data := t.GetAll()
	
	// 序列化并缓存
	jsonData, err := json.Marshal(data)
	if err == nil {
		t.cacheMutex.Lock()
		t.cachedJSON = jsonData
		t.cacheValid = true
		t.cacheMutex.Unlock()
	}
	
	return jsonData, err
}

// UnmarshalJSON 实现JSON反序列化
func (t *ThreadSafeTags) UnmarshalJSON(data []byte) error {
	var temp map[string]string
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	
	t.mu.Lock()
	defer t.mu.Unlock()
	
	t.data = temp
	t.invalidateCache()
	return nil
}

// invalidateCache 使缓存失效（必须在写锁下调用）
func (t *ThreadSafeTags) invalidateCache() {
	t.cacheMutex.Lock()
	t.cacheValid = false
	t.cacheMutex.Unlock()
}

// ToMap 转换为标准map（兼容性方法）
func (t *ThreadSafeTags) ToMap() map[string]string {
	return t.GetAll()
}

// IsEmpty 检查是否为空
func (t *ThreadSafeTags) IsEmpty() bool {
	return t.Len() == 0
}

// Merge 合并另一个ThreadSafeTags
func (t *ThreadSafeTags) Merge(other *ThreadSafeTags) {
	if other == nil {
		return
	}
	
	otherData := other.GetAll()
	
	t.mu.Lock()
	defer t.mu.Unlock()
	
	for k, v := range otherData {
		t.data[k] = v
	}
	t.invalidateCache()
}

// Clone 创建深度副本
func (t *ThreadSafeTags) Clone() *ThreadSafeTags {
	newTags := NewThreadSafeTags()
	newTags.mu.Lock()
	defer newTags.mu.Unlock()
	
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	for k, v := range t.data {
		newTags.data[k] = v
	}
	
	return newTags
}

// 性能优化：批量操作
func (t *ThreadSafeTags) SetMultiple(tags map[string]string) {
	if len(tags) == 0 {
		return
	}
	
	t.mu.Lock()
	defer t.mu.Unlock()
	
	for k, v := range tags {
		t.data[k] = v
	}
	t.invalidateCache()
}

// 调试和监控支持
func (t *ThreadSafeTags) GetStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	return map[string]interface{}{
		"size":         len(t.data),
		"cache_valid":  t.cacheValid,
		"has_cache":    len(t.cachedJSON) > 0,
	}
}