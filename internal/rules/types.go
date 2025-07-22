package rules

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// Rule 规则定义
type Rule struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description" yaml:"description"`
	Enabled     bool              `json:"enabled" yaml:"enabled"`
	Priority    int               `json:"priority" yaml:"priority"`
	Version     int               `json:"version" yaml:"version"`
	Conditions  *Condition        `json:"conditions" yaml:"conditions"`
	Actions     []Action          `json:"actions" yaml:"actions"`
	Tags        map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	CreatedAt   time.Time         `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" yaml:"updated_at"`
}

// Condition 条件定义
type Condition struct {
	Type       string       `json:"type,omitempty" yaml:"type,omitempty"`             // "simple", "expression", "lua"
	Field      string       `json:"field,omitempty" yaml:"field,omitempty"`           // 字段名
	Operator   string       `json:"operator,omitempty" yaml:"operator,omitempty"`     // 操作符
	Value      interface{}  `json:"value,omitempty" yaml:"value,omitempty"`           // 比较值
	Expression string       `json:"expression,omitempty" yaml:"expression,omitempty"` // 表达式
	Script     string       `json:"script,omitempty" yaml:"script,omitempty"`         // 脚本
	And        []*Condition `json:"and,omitempty" yaml:"and,omitempty"`               // AND条件
	Or         []*Condition `json:"or,omitempty" yaml:"or,omitempty"`                 // OR条件
	Not        *Condition   `json:"not,omitempty" yaml:"not,omitempty"`               // NOT条件
}

// Action 动作定义
type Action struct {
	Type    string                 `json:"type" yaml:"type"`
	Config  map[string]interface{} `json:"config" yaml:"config"`
	Async   bool                   `json:"async,omitempty" yaml:"async,omitempty"`
	Timeout time.Duration          `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Retry   int                    `json:"retry,omitempty" yaml:"retry,omitempty"`
}

// ProcessedPoint 处理后的数据点
type ProcessedPoint struct {
	Original    model.Point    `json:"original"`
	Processed   model.Point    `json:"processed"`
	RuleID      string         `json:"rule_id"`
	RuleName    string         `json:"rule_name"`
	Actions     []ActionResult `json:"actions"`
	ProcessedAt time.Time      `json:"processed_at"`
}

// ActionResult 动作执行结果
type ActionResult struct {
	Type     string        `json:"type"`
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Output   interface{}   `json:"output,omitempty"`
}

// Alert 报警消息
type Alert struct {
	ID        string            `json:"id"`
	RuleID    string            `json:"rule_id"`
	RuleName  string            `json:"rule_name"`
	Level     string            `json:"level"` // info, warning, error, critical
	Message   string            `json:"message"`
	DeviceID  string            `json:"device_id"`
	Key       string            `json:"key"`
	Value     interface{}       `json:"value"`
	Tags      map[string]string `json:"tags,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Throttle  time.Duration     `json:"throttle,omitempty"`
}

// AggregateResult 聚合结果
type AggregateResult struct {
	DeviceID  string                 `json:"device_id"`
	Key       string                 `json:"key"`
	Window    string                 `json:"window"`
	GroupBy   map[string]string      `json:"group_by,omitempty"`
	Functions map[string]interface{} `json:"functions"`
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time"`
	Count     int64                  `json:"count"`
	Timestamp time.Time              `json:"timestamp"`
}

// ActionHandler 动作处理器接口
type ActionHandler interface {
	Name() string
	Execute(ctx context.Context, point model.Point, rule *Rule, config map[string]interface{}) (*ActionResult, error)
}

// ConditionEvaluator 条件评估器接口
type ConditionEvaluator interface {
	Evaluate(condition *Condition, point model.Point) (bool, error)
}

// RuleStorage 规则存储接口
type RuleStorage interface {
	LoadRules() ([]*Rule, error)
	SaveRule(rule *Rule) error
	DeleteRule(id string) error
	GetRule(id string) (*Rule, error)
	ListRules() ([]*Rule, error)
	WatchChanges() (<-chan RuleChangeEvent, error)
}

// RuleChangeEvent 规则变更事件
type RuleChangeEvent struct {
	Type string `json:"type"` // create, update, delete
	Rule *Rule  `json:"rule"`
}

// RuleIndex 规则索引
type RuleIndex struct {
	DeviceIndex   map[string][]*Rule `json:"device_index"`
	KeyIndex      map[string][]*Rule `json:"key_index"`
	PriorityIndex []*Rule            `json:"priority_index"`
	TypeIndex     map[string][]*Rule `json:"type_index"`
}

// EngineMetrics 引擎指标
type EngineMetrics struct {
	RulesTotal         int64         `json:"rules_total"`
	RulesEnabled       int64         `json:"rules_enabled"`
	PointsProcessed    int64         `json:"points_processed"`
	RulesMatched       int64         `json:"rules_matched"`
	ActionsExecuted    int64         `json:"actions_executed"`
	ActionsSucceeded   int64         `json:"actions_succeeded"`
	ActionsFailed      int64         `json:"actions_failed"`
	ProcessingDuration time.Duration `json:"processing_duration"`
	LastProcessedAt    time.Time     `json:"last_processed_at"`
}

// CircularBuffer 环形缓冲区（用于时间窗口数据）
type CircularBuffer struct {
	Data  []DataPoint `json:"data"`
	Size  int         `json:"size"`
	Head  int         `json:"head"`
	Count int         `json:"count"`
}

// DataPoint 数据点（用于缓冲区）
type DataPoint struct {
	Value     interface{} `json:"value"`
	Timestamp time.Time   `json:"timestamp"`
}

// NewCircularBuffer 创建环形缓冲区
func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		Data:  make([]DataPoint, size),
		Size:  size,
		Head:  0,
		Count: 0,
	}
}

// Add 添加数据点
func (cb *CircularBuffer) Add(value interface{}, timestamp time.Time) {
	cb.Data[cb.Head] = DataPoint{
		Value:     value,
		Timestamp: timestamp,
	}
	cb.Head = (cb.Head + 1) % cb.Size
	if cb.Count < cb.Size {
		cb.Count++
	}
}

// GetLatest 获取最新的N个数据点
func (cb *CircularBuffer) GetLatest(n int) []DataPoint {
	if n > cb.Count {
		n = cb.Count
	}

	result := make([]DataPoint, n)
	for i := 0; i < n; i++ {
		idx := (cb.Head - 1 - i + cb.Size) % cb.Size
		result[i] = cb.Data[idx]
	}
	return result
}

// GetInTimeWindow 获取时间窗口内的数据点
func (cb *CircularBuffer) GetInTimeWindow(window time.Duration) []DataPoint {
	now := time.Now()
	cutoff := now.Add(-window)

	var result []DataPoint
	for i := 0; i < cb.Count; i++ {
		idx := (cb.Head - 1 - i + cb.Size) % cb.Size
		if cb.Data[idx].Timestamp.Before(cutoff) {
			break
		}
		result = append(result, cb.Data[idx])
	}
	return result
}

// Operators 支持的操作符
var Operators = map[string]func(interface{}, interface{}) bool{
	"eq": func(a, b interface{}) bool {
		return compareValues(a, b) == 0
	},
	"ne": func(a, b interface{}) bool {
		return compareValues(a, b) != 0
	},
	"gt": func(a, b interface{}) bool {
		return compareValues(a, b) > 0
	},
	"gte": func(a, b interface{}) bool {
		return compareValues(a, b) >= 0
	},
	"lt": func(a, b interface{}) bool {
		return compareValues(a, b) < 0
	},
	"lte": func(a, b interface{}) bool {
		return compareValues(a, b) <= 0
	},
	"contains": func(a, b interface{}) bool {
		if str, ok := a.(string); ok {
			if substr, ok := b.(string); ok {
				return strings.Contains(str, substr)
			}
		}
		return false
	},
	"startswith": func(a, b interface{}) bool {
		if str, ok := a.(string); ok {
			if prefix, ok := b.(string); ok {
				return strings.HasPrefix(str, prefix)
			}
		}
		return false
	},
	"endswith": func(a, b interface{}) bool {
		if str, ok := a.(string); ok {
			if suffix, ok := b.(string); ok {
				return strings.HasSuffix(str, suffix)
			}
		}
		return false
	},
	"regex": func(a, b interface{}) bool {
		if str, ok := a.(string); ok {
			if pattern, ok := b.(string); ok {
				if matched, err := MatchString(pattern, str); err == nil {
					return matched
				}
			}
		}
		return false
	},
}

// compareValues 比较两个值
func compareValues(a, b interface{}) int {
	// 处理数字类型
	if numA, ok := toFloat64(a); ok {
		if numB, ok := toFloat64(b); ok {
			if numA < numB {
				return -1
			} else if numA > numB {
				return 1
			}
			return 0
		}
	}

	// 处理字符串类型
	if strA, ok := a.(string); ok {
		if strB, ok := b.(string); ok {
			if strA < strB {
				return -1
			} else if strA > strB {
				return 1
			}
			return 0
		}
	}

	// 处理布尔类型
	if boolA, ok := a.(bool); ok {
		if boolB, ok := b.(bool); ok {
			if !boolA && boolB {
				return -1
			} else if boolA && !boolB {
				return 1
			}
			return 0
		}
	}

	return 0
}

// toFloat64 尝试将值转换为float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		if f, err := val.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}

