package rules

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/y001j/iot-gateway/internal/model"
)

// Evaluator 条件评估器
type Evaluator struct {
	functions    map[string]Function
	regexCache   map[string]*regexp.Regexp
	regexMutex   sync.RWMutex
}

// Function 内置函数接口
type Function interface {
	Name() string
	Call(args []interface{}) (interface{}, error)
}

// NewEvaluator 创建条件评估器
func NewEvaluator() *Evaluator {
	evaluator := &Evaluator{
		functions:  make(map[string]Function),
		regexCache: make(map[string]*regexp.Regexp),
	}

	// 注册内置函数
	evaluator.registerBuiltinFunctions()

	return evaluator
}

// Evaluate 评估条件
func (e *Evaluator) Evaluate(condition *Condition, point model.Point) (bool, error) {
	if condition == nil {
		return true, nil
	}

	switch condition.Type {
	case "", "simple":
		return e.evaluateSimpleCondition(condition, point)
	case "expression":
		return e.evaluateExpression(condition, point)
	case "lua":
		return e.evaluateLuaScript(condition, point)
	default:
		return false, fmt.Errorf("不支持的条件类型: %s", condition.Type)
	}
}

// evaluateSimpleCondition 评估简单条件
func (e *Evaluator) evaluateSimpleCondition(condition *Condition, point model.Point) (bool, error) {
	// 处理逻辑操作符
	if len(condition.And) > 0 {
		return e.evaluateAndCondition(condition.And, point)
	}

	if len(condition.Or) > 0 {
		return e.evaluateOrCondition(condition.Or, point)
	}

	if condition.Not != nil {
		result, err := e.Evaluate(condition.Not, point)
		if err != nil {
			return false, err
		}
		return !result, nil
	}

	// 处理字段比较
	if condition.Field != "" {
		return e.evaluateFieldCondition(condition, point)
	}

	// 默认返回true（空条件）
	return true, nil
}

// evaluateAndCondition 评估AND条件
func (e *Evaluator) evaluateAndCondition(conditions []*Condition, point model.Point) (bool, error) {
	for _, cond := range conditions {
		result, err := e.Evaluate(cond, point)
		if err != nil {
			return false, err
		}
		if !result {
			return false, nil // 短路评估
		}
	}
	return true, nil
}

// evaluateOrCondition 评估OR条件
func (e *Evaluator) evaluateOrCondition(conditions []*Condition, point model.Point) (bool, error) {
	for _, cond := range conditions {
		result, err := e.Evaluate(cond, point)
		if err != nil {
			return false, err
		}
		if result {
			return true, nil // 短路评估
		}
	}
	return false, nil
}

// evaluateFieldCondition 评估字段条件
func (e *Evaluator) evaluateFieldCondition(condition *Condition, point model.Point) (bool, error) {
	// 获取字段值
	fieldValue, err := e.getFieldValue(condition.Field, point)
	if err != nil {
		return false, fmt.Errorf("获取字段值失败: %w", err)
	}

	// 获取比较值
	compareValue := condition.Value

	// 执行比较操作
	operator := condition.Operator
	if operator == "" {
		operator = "eq" // 默认为相等比较
	}

	if opFunc, exists := Operators[operator]; exists {
		return opFunc(fieldValue, compareValue), nil
	}

	return false, fmt.Errorf("不支持的操作符: %s", operator)
}

// getFieldValue 获取字段值
func (e *Evaluator) getFieldValue(field string, point model.Point) (interface{}, error) {
	// 支持嵌套字段访问，如 "tags.location"
	parts := strings.Split(field, ".")

	var current interface{}
	switch parts[0] {
	case "device_id":
		current = point.DeviceID
	case "key":
		current = point.Key
	case "value":
		current = point.Value
	case "type":
		current = string(point.Type)
	case "timestamp":
		current = point.Timestamp
	case "tags":
		current = point.Tags
	default:
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("未知字段: %s", parts[0]), nil).
			WithContext("field", field).
			WithContext("available_fields", []string{"device_id", "key", "value", "type", "timestamp", "tags"})
	}

	// 处理嵌套字段
	for i := 1; i < len(parts); i++ {
		switch v := current.(type) {
		case map[string]string:
			if val, exists := v[parts[i]]; exists {
				current = val
			} else {
				return nil, fmt.Errorf("字段不存在: %s", field)
			}
		case map[string]interface{}:
			if val, exists := v[parts[i]]; exists {
				current = val
			} else {
				return nil, fmt.Errorf("字段不存在: %s", field)
			}
		default:
			return nil, fmt.Errorf("无法访问嵌套字段: %s", field)
		}
	}

	return current, nil
}

// evaluateExpression 评估表达式（使用增强的表达式引擎）
func (e *Evaluator) evaluateExpression(condition *Condition, point model.Point) (bool, error) {
	expression := condition.Expression
	if expression == "" {
		return false, fmt.Errorf("表达式不能为空")
	}

	// 使用增强的表达式引擎
	engine := NewExpressionEngine()
	result, err := engine.Evaluate(expression, point)
	if err != nil {
		// 回退到简单表达式解析
		return e.parseSimpleExpression(expression, point)
	}

	// 将结果转换为布尔值
	switch v := result.(type) {
	case bool:
		return v, nil
	case int, int32, int64:
		return v != 0, nil
	case float32, float64:
		if num, ok := toFloat64(v); ok {
			return num != 0, nil
		}
	case string:
		return v != "" && v != "false" && v != "0", nil
	}

	return result != nil, nil
}

// parseSimpleExpression 解析简单表达式
func (e *Evaluator) parseSimpleExpression(expression string, point model.Point) (bool, error) {
	// 移除空格
	expression = strings.ReplaceAll(expression, " ", "")

	// 处理AND操作
	if strings.Contains(expression, "&&") {
		parts := strings.Split(expression, "&&")
		for _, part := range parts {
			result, err := e.parseSimpleExpression(part, point)
			if err != nil {
				return false, err
			}
			if !result {
				return false, nil
			}
		}
		return true, nil
	}

	// 处理OR操作
	if strings.Contains(expression, "||") {
		parts := strings.Split(expression, "||")
		for _, part := range parts {
			result, err := e.parseSimpleExpression(part, point)
			if err != nil {
				return false, err
			}
			if result {
				return true, nil
			}
		}
		return false, nil
	}

	// 处理比较操作
	operators := []string{">=", "<=", "!=", "==", ">", "<"}
	for _, op := range operators {
		if strings.Contains(expression, op) {
			parts := strings.Split(expression, op)
			if len(parts) != 2 {
				continue
			}

			leftValue, err := e.parseValue(strings.TrimSpace(parts[0]), point)
			if err != nil {
				return false, err
			}

			rightValue, err := e.parseValue(strings.TrimSpace(parts[1]), point)
			if err != nil {
				return false, err
			}

			return e.compareValues(leftValue, rightValue, op)
		}
	}

	return false, fmt.Errorf("无法解析表达式: %s", expression)
}

// parseValue 解析值
func (e *Evaluator) parseValue(valueStr string, point model.Point) (interface{}, error) {
	// 去除引号
	valueStr = strings.Trim(valueStr, "\"'")

	// 检查是否是字段引用
	if strings.Contains(valueStr, ".") ||
		valueStr == "device_id" || valueStr == "key" ||
		valueStr == "value" || valueStr == "type" {
		return e.getFieldValue(valueStr, point)
	}

	// 尝试解析为数字
	if num, err := strconv.ParseFloat(valueStr, 64); err == nil {
		return num, nil
	}

	// 尝试解析为整数
	if num, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
		return num, nil
	}

	// 尝试解析为布尔值
	if b, err := strconv.ParseBool(valueStr); err == nil {
		return b, nil
	}

	// 默认作为字符串
	return valueStr, nil
}

// compareValues 比较值
func (e *Evaluator) compareValues(left, right interface{}, operator string) (bool, error) {
	switch operator {
	case "==":
		return compareValues(left, right) == 0, nil
	case "!=":
		return compareValues(left, right) != 0, nil
	case ">":
		return compareValues(left, right) > 0, nil
	case ">=":
		return compareValues(left, right) >= 0, nil
	case "<":
		return compareValues(left, right) < 0, nil
	case "<=":
		return compareValues(left, right) <= 0, nil
	default:
		return false, NewConditionError(ErrCodeConditionOperator, 
			fmt.Sprintf("不支持的比较操作符: %s", operator), nil).
			WithContext("operator", operator).
			WithContext("supported_operators", []string{"==", "!=", ">", ">=", "<", "<="})
	}
}

// evaluateLuaScript 评估Lua脚本（占位符实现）
func (e *Evaluator) evaluateLuaScript(condition *Condition, point model.Point) (bool, error) {
	// 这里是Lua脚本评估的占位符实现
	// 在实际实现中，应该集成Lua虚拟机
	return false, fmt.Errorf("Lua脚本评估暂未实现")
}

// RegisterFunction 注册自定义函数
func (e *Evaluator) RegisterFunction(fn Function) {
	e.functions[fn.Name()] = fn
}

// GetCompiledRegex 获取编译后的正则表达式（带缓存）
func (e *Evaluator) GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	// 先尝试读锁获取缓存
	e.regexMutex.RLock()
	if compiled, exists := e.regexCache[pattern]; exists {
		e.regexMutex.RUnlock()
		return compiled, nil
	}
	e.regexMutex.RUnlock()
	
	// 缓存不存在，编译正则表达式
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则表达式编译失败: %w", err)
	}
	
	// 写锁存储到缓存
	e.regexMutex.Lock()
	// 双重检查，防止并发重复编译
	if existing, exists := e.regexCache[pattern]; exists {
		e.regexMutex.Unlock()
		return existing, nil
	}
	
	// 限制缓存大小，防止内存泄漏
	if len(e.regexCache) >= 1000 {
		// 简单的LRU：清空缓存重新开始
		e.regexCache = make(map[string]*regexp.Regexp)
	}
	
	e.regexCache[pattern] = compiled
	e.regexMutex.Unlock()
	
	return compiled, nil
}

// registerBuiltinFunctions 注册内置函数
func (e *Evaluator) registerBuiltinFunctions() {
	// 数学函数
	e.RegisterFunction(&AbsFunction{})
	e.RegisterFunction(&MaxFunction{})
	e.RegisterFunction(&MinFunction{})

	// 字符串函数
	e.RegisterFunction(&LengthFunction{})
	e.RegisterFunction(&UpperFunction{})
	e.RegisterFunction(&LowerFunction{})

	// 类型转换函数
	e.RegisterFunction(&ToStringFunction{})
	e.RegisterFunction(&ToNumberFunction{})
	e.RegisterFunction(&ToBoolFunction{})
}

// 内置函数实现

// AbsFunction 绝对值函数
type AbsFunction struct{}

func (f *AbsFunction) Name() string { return "abs" }

func (f *AbsFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("abs函数需要1个参数")
	}

	if num, ok := toFloat64(args[0]); ok {
		if num < 0 {
			return -num, nil
		}
		return num, nil
	}

	return nil, fmt.Errorf("abs函数参数必须是数字")
}

// MaxFunction 最大值函数
type MaxFunction struct{}

func (f *MaxFunction) Name() string { return "max" }

func (f *MaxFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("max函数至少需要2个参数")
	}

	var maxVal float64
	var hasValue bool

	for i, arg := range args {
		if num, ok := toFloat64(arg); ok {
			if !hasValue || num > maxVal {
				maxVal = num
				hasValue = true
			}
		} else {
			return nil, fmt.Errorf("max函数第%d个参数必须是数字", i+1)
		}
	}

	if !hasValue {
		return nil, fmt.Errorf("max函数没有有效的数字参数")
	}

	return maxVal, nil
}

// MinFunction 最小值函数
type MinFunction struct{}

func (f *MinFunction) Name() string { return "min" }

func (f *MinFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("min函数至少需要2个参数")
	}

	var minVal float64
	var hasValue bool

	for i, arg := range args {
		if num, ok := toFloat64(arg); ok {
			if !hasValue || num < minVal {
				minVal = num
				hasValue = true
			}
		} else {
			return nil, fmt.Errorf("min函数第%d个参数必须是数字", i+1)
		}
	}

	if !hasValue {
		return nil, fmt.Errorf("min函数没有有效的数字参数")
	}

	return minVal, nil
}

// LengthFunction 长度函数
type LengthFunction struct{}

func (f *LengthFunction) Name() string { return "length" }

func (f *LengthFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("length函数需要1个参数")
	}

	switch v := args[0].(type) {
	case string:
		return len(v), nil
	case []interface{}:
		return len(v), nil
	case map[string]interface{}:
		return len(v), nil
	default:
		// 使用反射获取长度
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan, reflect.String:
			return rv.Len(), nil
		default:
			return nil, fmt.Errorf("length函数参数类型不支持")
		}
	}
}

// UpperFunction 转大写函数
type UpperFunction struct{}

func (f *UpperFunction) Name() string { return "upper" }

func (f *UpperFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("upper函数需要1个参数")
	}

	if str, ok := args[0].(string); ok {
		return strings.ToUpper(str), nil
	}

	return nil, fmt.Errorf("upper函数参数必须是字符串")
}

// LowerFunction 转小写函数
type LowerFunction struct{}

func (f *LowerFunction) Name() string { return "lower" }

func (f *LowerFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("lower函数需要1个参数")
	}

	if str, ok := args[0].(string); ok {
		return strings.ToLower(str), nil
	}

	return nil, fmt.Errorf("lower函数参数必须是字符串")
}

// ToStringFunction 转字符串函数
type ToStringFunction struct{}

func (f *ToStringFunction) Name() string { return "toString" }

func (f *ToStringFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("toString函数需要1个参数")
	}

	return fmt.Sprintf("%v", args[0]), nil
}

// ToNumberFunction 转数字函数
type ToNumberFunction struct{}

func (f *ToNumberFunction) Name() string { return "toNumber" }

func (f *ToNumberFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("toNumber函数需要1个参数")
	}

	if num, ok := toFloat64(args[0]); ok {
		return num, nil
	}

	if str, ok := args[0].(string); ok {
		if num, err := strconv.ParseFloat(str, 64); err == nil {
			return num, nil
		}
	}

	return nil, fmt.Errorf("无法转换为数字: %v", args[0])
}

// ToBoolFunction 转布尔函数
type ToBoolFunction struct{}

func (f *ToBoolFunction) Name() string { return "toBool" }

func (f *ToBoolFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("toBool函数需要1个参数")
	}

	switch v := args[0].(type) {
	case bool:
		return v, nil
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b, nil
		}
		return len(v) > 0, nil
	case int, int32, int64:
		return v != 0, nil
	case float32, float64:
		if num, ok := toFloat64(v); ok {
			return num != 0, nil
		}
	}

	return args[0] != nil, nil
}
