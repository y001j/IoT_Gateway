package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// DataTypeDefinition 数据类型定义
type DataTypeDefinition struct {
	Type                string                          `json:"type" yaml:"type"`                                   // 主数据类型
	Fields              map[string]FieldDefinition      `json:"fields,omitempty" yaml:"fields,omitempty"`           // 字段定义
	CoordinateSystem    string                          `json:"coordinate_system,omitempty" yaml:"coordinate_system,omitempty"` // GPS坐标系
	ColorSpace          string                          `json:"color_space,omitempty" yaml:"color_space,omitempty"` // 颜色空间
	ArrayType           string                          `json:"array_type,omitempty" yaml:"array_type,omitempty"`   // 数组类型
	TimeUnit            string                          `json:"time_unit,omitempty" yaml:"time_unit,omitempty"`     // 时间单位
	MatrixType          string                          `json:"matrix_type,omitempty" yaml:"matrix_type,omitempty"` // 矩阵类型
}

// FieldDefinition 字段定义
type FieldDefinition struct {
	Type       string                        `json:"type" yaml:"type"`                           // 字段类型
	Properties map[string]PropertyDefinition `json:"properties,omitempty" yaml:"properties,omitempty"` // 属性定义
}

// PropertyDefinition 属性定义
type PropertyDefinition struct {
	Type      string      `json:"type" yaml:"type"`                           // 属性数据类型
	Range     []float64   `json:"range,omitempty" yaml:"range,omitempty"`     // 数值范围
	Unit      string      `json:"unit,omitempty" yaml:"unit,omitempty"`       // 单位
	Optional  bool        `json:"optional,omitempty" yaml:"optional,omitempty"` // 是否可选
	Computed  bool        `json:"computed,omitempty" yaml:"computed,omitempty"` // 是否计算得出
	MinLength int         `json:"min_length,omitempty" yaml:"min_length,omitempty"` // 数组最小长度
	MaxLength int         `json:"max_length,omitempty" yaml:"max_length,omitempty"` // 数组最大长度
}

// Rule 规则定义
type Rule struct {
	ID          string               `json:"id" yaml:"id"`
	Name        string               `json:"name" yaml:"name"`
	Description string               `json:"description" yaml:"description"`
	Enabled     bool                 `json:"enabled" yaml:"enabled"`
	Priority    int                  `json:"priority" yaml:"priority"`
	Version     int                  `json:"version" yaml:"version"`
	DataType    interface{}          `json:"data_type,omitempty" yaml:"data_type,omitempty"` // 数据类型：字符串或详细定义
	Conditions  *Condition           `json:"conditions" yaml:"conditions"`
	Actions     []Action             `json:"actions" yaml:"actions"`
	Tags        map[string]string    `json:"tags,omitempty" yaml:"tags,omitempty"`
	CreatedAt   time.Time            `json:"created_at" yaml:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at" yaml:"updated_at"`
}

// GetDataTypeName 获取规则的数据类型名称
func (r *Rule) GetDataTypeName() string {
	if r.DataType == nil {
		return ""
	}
	
	// 如果是字符串形式，直接返回
	if dataTypeName, ok := r.DataType.(string); ok {
		return dataTypeName
	}
	
	// 如果是详细定义形式，返回Type字段
	if dataTypeDef, ok := r.DataType.(*DataTypeDefinition); ok {
		return dataTypeDef.Type
	}
	
	// 尝试从map[string]interface{}中提取（JSON解码后的格式）
	if dataTypeMap, ok := r.DataType.(map[string]interface{}); ok {
		if typeValue, exists := dataTypeMap["type"]; exists {
			if typeName, ok := typeValue.(string); ok {
				return typeName
			}
		}
	}
	
	return ""
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
		cmp := compareValues(a, b)
		if cmp == -2 { // NaN参与的比较
			return false // NaN != 任何值（包括NaN自身）
		}
		return cmp == 0
	},
	"ne": func(a, b interface{}) bool {
		cmp := compareValues(a, b)
		if cmp == -2 { // NaN参与的比较
			return true // NaN != 任何值
		}
		return cmp != 0
	},
	"gt": func(a, b interface{}) bool {
		cmp := compareValues(a, b)
		if cmp == -2 { // NaN参与的比较
			return false // NaN与任何值比较都返回false
		}
		return cmp > 0
	},
	"gte": func(a, b interface{}) bool {
		cmp := compareValues(a, b)
		if cmp == -2 { // NaN参与的比较
			return false // NaN与任何值比较都返回false
		}
		return cmp >= 0
	},
	"lt": func(a, b interface{}) bool {
		cmp := compareValues(a, b)
		if cmp == -2 { // NaN参与的比较
			return false // NaN与任何值比较都返回false
		}
		return cmp < 0
	},
	"lte": func(a, b interface{}) bool {
		cmp := compareValues(a, b)
		if cmp == -2 { // NaN参与的比较
			return false // NaN与任何值比较都返回false
		}
		return cmp <= 0
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
		str, ok1 := a.(string)
		pattern, ok2 := b.(string)
		
		// 验证参数类型
		if !ok1 || !ok2 {
			// 对于非字符串类型，尝试转换
			if !ok1 {
				str = fmt.Sprintf("%v", a)
			}
			if !ok2 {
				pattern = fmt.Sprintf("%v", b)
			}
		}
		
		// 验证正则表达式
		if pattern == "" {
			return false
		}
		
		// 增强的正则表达式验证
		if _, err := MatchString(pattern, ""); err != nil {
			// 无效的正则表达式模式，返回false而不是panic
			return false
		}
		
		matched, err := MatchString(pattern, str)
		return err == nil && matched
	},
}

// compareValues 比较两个值
func compareValues(a, b interface{}) int {
	// 首先处理 nil 值
	aIsNil := (a == nil)
	bIsNil := (b == nil)
	
	if aIsNil && bIsNil {
		return 0 // nil == nil
	}
	if aIsNil && !bIsNil {
		return -1 // nil < non-nil
	}
	if !aIsNil && bIsNil {
		return 1 // non-nil > nil
	}
	
	// 处理数字类型 - 包括IEEE 754特殊值
	if numA, okA, specialA := toFloat64Enhanced(a); okA {
		if numB, okB, specialB := toFloat64Enhanced(b); okB {
			// 处理NaN：按照IEEE 754标准，NaN与任何值比较都不相等
			if specialA == "nan" || specialB == "nan" {
				// NaN的特殊处理：对于eq操作返回1（不相等），对于gt/lt操作需要特殊标记
				// 使用-2作为特殊标记，表示NaN参与的比较
				return -2 
			}
			
			// 处理无穷大
			if specialA == "inf" {
				if specialB == "inf" {
					return 0 // +Inf == +Inf
				}
				return 1 // +Inf > 任何有限数或-Inf
			}
			if specialA == "-inf" {
				if specialB == "-inf" {
					return 0 // -Inf == -Inf
				}
				return -1 // -Inf < 任何有限数或+Inf
			}
			if specialB == "inf" {
				return -1 // 任何有限数 < +Inf
			}
			if specialB == "-inf" {
				return 1 // 任何有限数 > -Inf
			}
			
			// 标准浮点数比较
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

// toFloat64Enhanced 增强版数值转换，支持IEEE 754特殊值
func toFloat64Enhanced(v interface{}) (float64, bool, string) {
	// 返回值：数值，是否有效，特殊标记
	if v == nil {
		return 0, false, ""
	}

	switch val := v.(type) {
	case float64:
		if math.IsNaN(val) {
			return val, true, "nan"
		}
		if math.IsInf(val, 1) {
			return val, true, "inf"
		}
		if math.IsInf(val, -1) {
			return val, true, "-inf"
		}
		return val, true, ""
	case float32:
		f := float64(val)
		if math.IsNaN(f) {
			return f, true, "nan"
		}
		if math.IsInf(f, 1) {
			return f, true, "inf"
		}
		if math.IsInf(f, -1) {
			return f, true, "-inf"
		}
		return f, true, ""
	case int:
		return float64(val), true, ""
	case int32:
		return float64(val), true, ""
	case int64:
		return float64(val), true, ""
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, false, ""
		}
		if math.IsNaN(f) {
			return f, true, "nan"
		}
		if math.IsInf(f, 1) {
			return f, true, "inf"
		}
		if math.IsInf(f, -1) {
			return f, true, "-inf"
		}
		return f, true, ""
	default:
		return 0, false, ""
	}
}

// toFloat64 保持原有函数以确保向后兼容
func toFloat64(v interface{}) (float64, bool) {
	if v == nil {
		return 0, false
	}

	switch val := v.(type) {
	case float64:
		// 检查特殊值
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return 0, false
		}
		return val, true
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	case int:
		return float64(val), true
	case int8:
		return float64(val), true
	case int16:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint8:
		return float64(val), true
	case uint16:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	case json.Number:
		if f, err := val.Float64(); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return 0, false
			}
			return f, true
		}
	case string:
		// 验证字符串不为空
		if val == "" {
			return 0, false
		}
		
		// 检查字符串是否只包含有效字符
		if strings.TrimSpace(val) == "" {
			return 0, false
		}
		
		// 尝试将字符串转换为数字
		if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return 0, false
			}
			return f, true
		}
	case bool:
		if val {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

