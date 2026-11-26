package utils

import (
	"encoding/json"
	"sync"
)

// ShardedTags 线程安全的标签容器。
//
// 历史上这里使用 16 分片 + 全局统计锁的实现：每个数据点创建时要分配 16 个 map 和
// 16 把锁（约 40 次分配、2.8KB），而且每次读写都要获取同一把统计锁，分片并没有
// 减少竞争。数据点上的标签通常只有几个，并且绝大多数时候只被一个协程访问，所以
// 这里改为"单把读写锁 + 惰性创建的 map"，保持原有 API 不变。
type ShardedTags struct {
	mu   sync.RWMutex
	data map[string]string
	// shared 表示 data 与 Clone 出的副本共享（写时复制）：共享的 map 不再被修改，
	// 任何一方写入前先复制一份。管道给每个连接器一份数据点副本时，标签只读就不必复制。
	shared bool
}

// NewShardedTags 创建标签容器。shardCount 参数仅为兼容旧接口保留，已不再使用。
func NewShardedTags(shardCount int) *ShardedTags {
	return &ShardedTags{}
}

// NewShardedTagsFromMap 从 map 创建（复制一份，调用方后续修改 sourceMap 不会影响容器）
func NewShardedTagsFromMap(sourceMap map[string]string) *ShardedTags {
	st := &ShardedTags{}
	if len(sourceMap) > 0 {
		st.data = make(map[string]string, len(sourceMap))
		for k, v := range sourceMap {
			st.data[k] = v
		}
	}
	return st
}

// Set 设置标签值
func (st *ShardedTags) Set(key, value string) {
	st.mu.Lock()
	st.ownLocked(1)
	if st.data == nil {
		st.data = make(map[string]string, 4)
	}
	st.data[key] = value
	st.mu.Unlock()
}

// Get 获取标签值
func (st *ShardedTags) Get(key string) (string, bool) {
	st.mu.RLock()
	value, ok := st.data[key]
	st.mu.RUnlock()
	return value, ok
}

// GetAll 获取所有标签的副本
func (st *ShardedTags) GetAll() map[string]string {
	st.mu.RLock()
	result := make(map[string]string, len(st.data))
	for k, v := range st.data {
		result[k] = v
	}
	st.mu.RUnlock()
	return result
}

// GetKeys 获取所有键
func (st *ShardedTags) GetKeys() []string {
	st.mu.RLock()
	keys := make([]string, 0, len(st.data))
	for k := range st.data {
		keys = append(keys, k)
	}
	st.mu.RUnlock()
	return keys
}

// Len 获取标签总数
func (st *ShardedTags) Len() int {
	st.mu.RLock()
	n := len(st.data)
	st.mu.RUnlock()
	return n
}

// Delete 删除标签
func (st *ShardedTags) Delete(key string) {
	st.mu.Lock()
	if _, ok := st.data[key]; ok {
		st.ownLocked(0)
		delete(st.data, key)
	}
	st.mu.Unlock()
}

// Clear 清空所有标签
func (st *ShardedTags) Clear() {
	st.mu.Lock()
	st.data = nil
	st.shared = false
	st.mu.Unlock()
}

// SetMultiple 批量设置
func (st *ShardedTags) SetMultiple(tags map[string]string) {
	if len(tags) == 0 {
		return
	}
	st.mu.Lock()
	st.ownLocked(len(tags))
	if st.data == nil {
		st.data = make(map[string]string, len(tags))
	}
	for k, v := range tags {
		st.data[k] = v
	}
	st.mu.Unlock()
}

// MarshalJSON JSON 序列化
func (st *ShardedTags) MarshalJSON() ([]byte, error) {
	return json.Marshal(st.GetAll())
}

// UnmarshalJSON JSON 反序列化
func (st *ShardedTags) UnmarshalJSON(data []byte) error {
	var temp map[string]string
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	st.mu.Lock()
	st.data = temp
	st.shared = false
	st.mu.Unlock()
	return nil
}

// Clone 创建副本（写时复制）：副本与原容器共享同一个 map，直到任何一方写入。
// 以前每次都复制整个 map，管道为每个连接器复制数据点时，每个点每个连接器都要分配一次。
func (st *ShardedTags) Clone() *ShardedTags {
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.data) == 0 {
		return &ShardedTags{}
	}
	st.shared = true
	return &ShardedTags{data: st.data, shared: true}
}

// ownLocked 写入前调用（持有写锁）：data 与其他容器共享时先复制一份，extra 为预计新增的键数
func (st *ShardedTags) ownLocked(extra int) {
	if !st.shared {
		return
	}
	own := make(map[string]string, len(st.data)+extra)
	for k, v := range st.data {
		own[k] = v
	}
	st.data = own
	st.shared = false
}

// Merge 合并另一个容器
func (st *ShardedTags) Merge(other *ShardedTags) {
	if other == nil || other == st {
		return
	}
	st.SetMultiple(other.GetAll())
}

// GetStats 获取统计信息（保留旧接口字段，便于监控页面兼容）
func (st *ShardedTags) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"total_shards": 1,
		"current_size": st.Len(),
	}
}

// IsEmpty 检查是否为空
func (st *ShardedTags) IsEmpty() bool {
	return st.Len() == 0
}

// ShardedTagsAdapter 兼容性适配器，提供 map[string]string 接口
type ShardedTagsAdapter struct {
	*ShardedTags
}

// NewShardedTagsAdapter 创建适配器
func NewShardedTagsAdapter() *ShardedTagsAdapter {
	return &ShardedTagsAdapter{ShardedTags: NewShardedTags(0)}
}

// ToMap 转换为 map 副本
func (sta *ShardedTagsAdapter) ToMap() map[string]string {
	return sta.GetAll()
}

// FromMap 从 map 批量设置
func (sta *ShardedTagsAdapter) FromMap(source map[string]string) {
	sta.SetMultiple(source)
}
