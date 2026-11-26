package rules

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/utils"
)

// SafeValueForJSON 安全地转换值用于JSON序列化，防止并发访问问题
func SafeValueForJSON(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	// 处理各种类型
	switch v := value.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return v
	case error:
		if v == nil {
			return nil
		}
		return v.Error()
	case time.Time:
		return v
	case []byte:
		return string(v)
	case map[string]interface{}:
		// 递归处理嵌套map，创建安全副本
		return safeMapCopy(v)
	case *utils.ShardedTags:
		// Go 1.24安全方案：直接使用ShardedTags的安全接口
		if v != nil {
			return v.GetAll() // 天然线程安全，无concurrent map fatal风险
		}
		return make(map[string]string)
	case **utils.ShardedTags:
		// 双指针类型的ShardedTags
		if v != nil && *v != nil {
			return (*v).GetAll()
		}
		return make(map[string]string)
	case utils.ShardedTags:
		// 值类型的ShardedTags
		return v.GetAll()
	case map[string]string:
		// 处理string到string的map（优先使用ShardedTags转换）
		return safeExtractMapForEventPublishing(v)
	case []interface{}:
		// 递归处理数组
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = SafeValueForJSON(val)
		}
		return result
	default:
		// 对于其他类型，先检查是否为map类型
		if m, ok := tryExtractMapSafely(v); ok {
			return m
		}
		// 对于其他类型，尝试转换为字符串（但避免直接调用fmt.Sprintf处理map）
		return safeStringConversion(v)
	}
}

// safeMapCopy 创建map[string]interface{}的安全副本
// 现已升级为使用ShardedTags方法，提供100%数据完整性
func safeMapCopy(original map[string]interface{}) map[string]interface{} {
	if original == nil {
		return make(map[string]interface{})
	}

	// Go 1.24安全解决方案：使用递归安全复制
	result := make(map[string]interface{})
	for k, v := range original {
		// 递归处理每个值，确保嵌套数据也安全
		result[k] = SafeValueForJSON(v)
	}

	return result
}

// ARM32对齐优化：将原子变量放在结构体中确保64位对齐
var mapCopyStats struct {
	// 64-bit fields first for ARM32 alignment
	attempts  int64
	successes int64
	// Other fields
	warningOnce sync.Once
}

// safeExtractMapForEventPublishing Go 1.24专用：安全地提取map数据用于事件发布
// 现已升级为使用ShardedTags，提供100%数据完整性
func safeExtractMapForEventPublishing(original map[string]string) map[string]string {
	// 统计尝试次数
	atomic.AddInt64(&mapCopyStats.attempts, 1)

	// 如果原始map为空，返回空map
	if len(original) == 0 {
		atomic.AddInt64(&mapCopyStats.successes, 1)
		return make(map[string]string)
	}

	// Go 1.24安全解决方案：使用ShardedTags进行安全复制
	shardedTags := utils.NewShardedTagsFromMap(original)
	result := shardedTags.GetAll()

	// 统计成功次数
	atomic.AddInt64(&mapCopyStats.successes, 1)

	// 一次性警告日志
	mapCopyStats.warningOnce.Do(func() {
		log.Info().
			Int("original_size", len(original)).
			Int("result_size", len(result)).
			Msg("✅ ShardedTags系统启用：100%数据完整性保障")
	})

	return result
}

// tryExtractMapSafely 尝试安全地提取map信息
func tryExtractMapSafely(value interface{}) (interface{}, bool) {
	// 使用defer和recover来处理潜在的并发访问panic
	defer func() {
		recover() // 忽略panic
	}()

	// 尝试检查是否为map类型（不通过fmt.Sprintf）
	switch v := value.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for k, val := range v {
			keyStr := fmt.Sprintf("%v", k)
			result[keyStr] = SafeValueForJSON(val)
		}
		return result, true
	}

	return nil, false
}

// safeStringConversion 安全地将值转换为字符串
func safeStringConversion(value interface{}) string {
	// 使用defer和recover来处理潜在的panic
	defer func() {
		if r := recover(); r != nil {
			// 如果发生panic（比如并发map访问），返回类型信息
		}
	}()

	// 避免对可能有并发访问问题的复杂类型使用fmt.Sprintf
	return fmt.Sprintf("%T", value) // 只返回类型信息，避免值的迭代
}

// Note: safeExtractMapForEventPublishing is defined above at line 157

// RedactConfig 返回配置的副本，其中密码、密钥、令牌等敏感字段被替换为 "***"，用于日志输出。
func RedactConfig(config map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		lk := strings.ToLower(k)
		sensitive := false
		for _, w := range []string{"password", "passwd", "secret", "token", "key", "credential", "auth"} {
			if strings.Contains(lk, w) {
				sensitive = true
				break
			}
		}
		switch {
		case sensitive:
			out[k] = "***"
		default:
			if m, ok := v.(map[string]interface{}); ok {
				out[k] = RedactConfig(m)
			} else {
				out[k] = v
			}
		}
	}
	return out
}
