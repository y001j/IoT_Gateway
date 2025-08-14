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
	regexCache   sync.Map // 使用sync.Map替代带锁的map
}

// Function 内置函数接口
type Function interface {
	Name() string
	Call(args []interface{}) (interface{}, error)
}

// NewEvaluator 创建条件评估器
func NewEvaluator() *Evaluator {
	evaluator := &Evaluator{
		functions: make(map[string]Function),
		// regexCache 使用sync.Map，无需初始化
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

	// 首先检查复合条件（优先级最高）
	if condition.And != nil {
		return e.evaluateAndCondition(condition.And, point)
	}

	if condition.Or != nil {
		return e.evaluateOrCondition(condition.Or, point)
	}

	if condition.Not != nil {
		result, err := e.Evaluate(condition.Not, point)
		if err != nil {
			return false, err
		}
		return !result, nil
	}

	// 然后根据条件类型处理
	switch condition.Type {
	case "", "simple":
		return e.evaluateSimpleCondition(condition, point)
	case "and":
		// 对于类型为"and"的条件，如果And字段为空，则检查是否有其他字段定义的复合逻辑
		if len(condition.And) == 0 {
			return false, fmt.Errorf("and类型条件缺少and字段定义")
		}
		return e.evaluateAndCondition(condition.And, point)
	case "or":
		// 对于类型为"or"的条件，如果Or字段为空，则检查是否有其他字段定义的复合逻辑
		if len(condition.Or) == 0 {
			return false, fmt.Errorf("or类型条件缺少or字段定义")
		}
		return e.evaluateOrCondition(condition.Or, point)
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
	// 注意：在新的Evaluate方法中，复合条件已经在上层处理了
	// 这里只处理具体的字段比较条件
	
	// 处理字段比较 - 包括空字段名的验证
	return e.evaluateFieldCondition(condition, point)
}

// evaluateAndCondition 评估AND条件
func (e *Evaluator) evaluateAndCondition(conditions []*Condition, point model.Point) (bool, error) {
	if len(conditions) == 0 {
		return true, nil // 空AND条件数组返回true（没有任何条件为false）
	}

	for i, cond := range conditions {
		if cond == nil {
			return false, NewConditionError(ErrCodeConditionParse, "AND条件包含空条件", nil).
				WithContext("condition_index", i).
				WithContext("condition_type", "and")
		}

		result, err := e.Evaluate(cond, point)
		if err != nil {
			return false, NewConditionError(ErrCodeConditionEval, "AND条件评估失败", err).
				WithContext("condition_index", i).
				WithContext("condition", cond)
		}
		if !result {
			return false, nil // 短路评估
		}
	}
	return true, nil
}

// evaluateOrCondition 评估OR条件
func (e *Evaluator) evaluateOrCondition(conditions []*Condition, point model.Point) (bool, error) {
	if len(conditions) == 0 {
		return false, nil // 空OR条件数组返回false（没有任何条件为true）
	}

	for i, cond := range conditions {
		if cond == nil {
			return false, NewConditionError(ErrCodeConditionParse, "OR条件包含空条件", nil).
				WithContext("condition_index", i).
				WithContext("condition_type", "or")
		}

		result, err := e.Evaluate(cond, point)
		if err != nil {
			return false, NewConditionError(ErrCodeConditionEval, "OR条件评估失败", err).
				WithContext("condition_index", i).
				WithContext("condition", cond)
		}
		if result {
			return true, nil // 短路评估
		}
	}
	return false, nil
}

// evaluateFieldCondition 评估字段条件
func (e *Evaluator) evaluateFieldCondition(condition *Condition, point model.Point) (bool, error) {
	// 验证字段名
	if condition.Field == "" {
		return false, NewConditionError(ErrCodeConditionField, "字段名不能为空", nil).
			WithContext("condition", condition)
	}
	
	// 验证字段名不能只包含空格
	if strings.TrimSpace(condition.Field) == "" {
		return false, NewConditionError(ErrCodeConditionField, "字段名不能只包含空格", nil).
			WithContext("field", condition.Field)
	}

	// 获取字段值
	fieldValue, err := e.getFieldValue(condition.Field, point)
	if err != nil {
		return false, NewConditionError(ErrCodeConditionField, "获取字段值失败", err).
			WithContext("field", condition.Field).
			WithContext("point", point.DeviceID)
	}

	// 执行比较操作
	operator := condition.Operator
	if operator == "" {
		operator = "eq" // 默认为相等比较
	}

	// 验证操作符
	opFunc, exists := Operators[operator]
	if !exists {
		return false, NewConditionError(ErrCodeConditionOperator, "不支持的操作符", nil).
			WithContext("operator", operator).
			WithContext("supported_operators", []string{"eq", "ne", "gt", "gte", "lt", "lte", "contains", "startswith", "endswith", "regex"})
	}
	
	// 特殊处理：预验证特定操作符的类型兼容性
	if operator == "regex" {
		if patternStr, ok := condition.Value.(string); ok {
			// 检查空正则表达式
			if strings.TrimSpace(patternStr) == "" {
				return false, NewConditionError(ErrCodeConditionOperator, "正则表达式模式不能为空", nil).
					WithContext("pattern", patternStr)
			}
			
			// 验证正则表达式语法
			if _, err := GetCompiledRegex(patternStr); err != nil {
				return false, NewConditionError(ErrCodeConditionOperator, "无效的正则表达式", err).
					WithContext("pattern", patternStr)
			}
		} else {
			return false, NewConditionError(ErrCodeConditionOperator, "regex操作符的值必须是字符串", nil).
				WithContext("value_type", fmt.Sprintf("%T", condition.Value))
		}
	}
	
	// 验证字符串操作符只能用于字符串字段
	stringOperators := map[string]bool{
		"contains":   true,
		"startswith": true,
		"endswith":   true,
		"regex":      true,
	}
	
	if stringOperators[operator] {
		if _, ok := fieldValue.(string); !ok {
			return false, NewConditionError(ErrCodeConditionOperator, 
				fmt.Sprintf("操作符'%s'只能用于字符串类型字段", operator), nil).
				WithContext("field_type", fmt.Sprintf("%T", fieldValue)).
				WithContext("field_value", fieldValue).
				WithContext("operator", operator)
		}
	}

	// 执行比较，捕获panic
	defer func() {
		if r := recover(); r != nil {
			err = NewConditionError(ErrCodeConditionEval, "条件评估时发生异常", fmt.Errorf("%v", r)).
				WithContext("field_value", fieldValue).
				WithContext("compare_value", condition.Value).
				WithContext("operator", operator)
		}
	}()

	return opFunc(fieldValue, condition.Value), err
}

// getFieldValue 获取字段值
func (e *Evaluator) getFieldValue(field string, point model.Point) (interface{}, error) {
	// 支持嵌套字段访问，如 "tags.location" 或复合数据字段 "location.latitude"
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
		current = point.GetTagsCopy()
	default:
		// 检查是否为复合数据字段访问
		if len(parts) > 1 && point.IsComposite() {
			return e.getCompositeFieldValue(field, parts, point)
		}
		
		// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
		if tagValue, exists := point.GetTag(parts[0]); exists {
			current = tagValue
			break
		}
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("未知字段: %s", parts[0]), nil).
			WithContext("field", field).
			WithContext("available_fields", []string{"device_id", "key", "value", "type", "timestamp", "tags", "或任何tags中的字段", "复合数据字段如location.latitude"})
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

// getCompositeFieldValue 获取复合数据字段值
func (e *Evaluator) getCompositeFieldValue(field string, parts []string, point model.Point) (interface{}, error) {
	compositeData, err := point.GetCompositeData()
	if err != nil {
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("无法获取复合数据: %s", err.Error()), err).
			WithContext("field", field)
	}

	// 根据复合数据类型处理字段访问
	switch point.Type {
	case model.TypeLocation:
		return e.getLocationFieldValue(field, parts, compositeData)
	case model.TypeVector3D:
		return e.getVector3DFieldValue(field, parts, compositeData)
	case model.TypeColor:
		return e.getColorFieldValue(field, parts, compositeData)
	// 通用复合数据类型
	case model.TypeVector:
		return e.getVectorFieldValue(field, parts, compositeData)
	case model.TypeArray:
		return e.getArrayFieldValue(field, parts, compositeData)
	case model.TypeMatrix:
		return e.getMatrixFieldValue(field, parts, compositeData)
	case model.TypeTimeSeries:
		return e.getTimeSeriesFieldValue(field, parts, compositeData)
	default:
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("不支持的复合数据类型: %s", point.Type), nil).
			WithContext("field", field).
			WithContext("data_type", string(point.Type))
	}
}

// getLocationFieldValue 获取地理位置数据字段值
func (e *Evaluator) getLocationFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	locationData, ok := compositeData.(*model.LocationData)
	if !ok {
		return nil, fmt.Errorf("复合数据不是LocationData类型")
	}

	fieldName := parts[1]
	switch fieldName {
	case "latitude":
		return locationData.Latitude, nil
	case "longitude":
		return locationData.Longitude, nil
	case "altitude":
		return locationData.Altitude, nil
	case "accuracy":
		return locationData.Accuracy, nil
	case "speed":
		return locationData.Speed, nil
	case "heading":
		return locationData.Heading, nil
	default:
		// 检查衍生值
		if derivedValues := locationData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("LocationData中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", []string{
				"latitude", "longitude", "altitude", "accuracy", "speed", "heading",
				"coordinate_system", "has_altitude", "has_speed", "has_heading", 
				"elevation_category", "speed_category",
			})
	}
}

// getVector3DFieldValue 获取三轴向量数据字段值
func (e *Evaluator) getVector3DFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	vectorData, ok := compositeData.(*model.Vector3D)
	if !ok {
		return nil, fmt.Errorf("复合数据不是Vector3D类型")
	}

	fieldName := parts[1]
	switch fieldName {
	case "x":
		return vectorData.X, nil
	case "y":
		return vectorData.Y, nil
	case "z":
		return vectorData.Z, nil
	default:
		// 检查衍生值
		if derivedValues := vectorData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("Vector3D中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", []string{
				"x", "y", "z",
				"magnitude", "x_ratio", "y_ratio", "z_ratio", "dominant_axis",
			})
	}
}

// getColorFieldValue 获取颜色数据字段值
func (e *Evaluator) getColorFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	colorData, ok := compositeData.(*model.ColorData)
	if !ok {
		return nil, fmt.Errorf("复合数据不是ColorData类型")
	}

	fieldName := parts[1]
	switch fieldName {
	case "r", "red":
		return int(colorData.R), nil
	case "g", "green":
		return int(colorData.G), nil
	case "b", "blue":
		return int(colorData.B), nil
	case "a", "alpha":
		return int(colorData.A), nil
	default:
		// 检查衍生值
		if derivedValues := colorData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("ColorData中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", []string{
				"r", "red", "g", "green", "b", "blue", "a", "alpha",
				"hue", "saturation", "lightness",
			})
	}
}

// evaluateExpression 评估表达式（使用增强的表达式引擎）
func (e *Evaluator) evaluateExpression(condition *Condition, point model.Point) (bool, error) {
	expression := condition.Expression
	if expression == "" {
		return false, NewConditionError(ErrCodeConditionParse, "表达式不能为空", nil).
			WithContext("condition", condition)
	}

	// 验证表达式不能过长
	if len(expression) > 10000 {
		return false, NewConditionError(ErrCodeConditionParse, "表达式过长", nil).
			WithContext("expression_length", len(expression)).
			WithContext("max_length", 10000)
	}

	// 使用增强的表达式引擎
	engine := NewExpressionEngine()
	result, err := engine.Evaluate(expression, point)
	if err != nil {
		// 检查是否是语法错误，如果是则直接报告
		if strings.Contains(err.Error(), "表达式语法错误") {
			return false, NewConditionError(ErrCodeConditionParse, "表达式语法错误", err).
				WithContext("expression", expression)
		}
		
		// 只有在非语法错误时才回退到简单表达式解析
		fallbackResult, fallbackErr := e.parseSimpleExpression(expression, point)
		if fallbackErr != nil {
			return false, NewConditionError(ErrCodeConditionParse, "表达式解析失败", fallbackErr).
				WithContext("expression", expression).
				WithContext("primary_error", err.Error())
		}
		return fallbackResult, nil
	}

	// 将结果转换为布尔值
	return e.convertToBool(result), nil
}

// convertToBool 将结果转换为布尔值
func (e *Evaluator) convertToBool(result interface{}) bool {
	switch v := result.(type) {
	case bool:
		return v
	case int, int8, int16, int32, int64:
		return v != 0
	case uint, uint8, uint16, uint32, uint64:
		return v != 0
	case float32, float64:
		if num, ok := toFloat64(v); ok {
			return num != 0
		}
	case string:
		return v != "" && v != "false" && v != "0"
	case nil:
		return false
	}
	return result != nil
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

// GetCompiledRegex 获取编译后的正则表达式（无锁缓存）
func (e *Evaluator) GetCompiledRegex(pattern string) (*regexp.Regexp, error) {
	// 无锁读取
	if compiled, ok := e.regexCache.Load(pattern); ok {
		return compiled.(*regexp.Regexp), nil
	}
	
	// 编译正则表达式
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("正则表达式编译失败: %w", err)
	}
	
	// 原子存储，处理并发编译
	actual, _ := e.regexCache.LoadOrStore(pattern, compiled)
	return actual.(*regexp.Regexp), nil
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
		return nil, NewConditionError(ErrCodeConditionType, "toNumber函数需要1个参数", nil).
			WithContext("args_count", len(args))
	}

	arg := args[0]
	if arg == nil {
		return nil, NewConditionError(ErrCodeConditionType, "toNumber函数参数不能为nil", nil)
	}

	if num, ok := toFloat64(arg); ok {
		return num, nil
	}

	// 详细的错误信息
	return nil, NewConditionError(ErrCodeConditionType, "无法转换为数字", nil).
		WithContext("value", arg).
		WithContext("type", fmt.Sprintf("%T", arg)).
		WithContext("supported_types", []string{"int", "float", "string", "bool"})
}

// ToBoolFunction 转布尔函数
type ToBoolFunction struct{}

func (f *ToBoolFunction) Name() string { return "toBool" }

func (f *ToBoolFunction) Call(args []interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, NewConditionError(ErrCodeConditionType, "toBool函数需要1个参数", nil).
			WithContext("args_count", len(args))
	}

	arg := args[0]
	if arg == nil {
		return false, nil
	}

	switch v := arg.(type) {
	case bool:
		return v, nil
	case string:
		// 严格的布尔转换
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off", "":
			return false, nil
		default:
			return len(v) > 0, nil
		}
	case int, int8, int16, int32, int64:
		return v != 0, nil
	case uint, uint8, uint16, uint32, uint64:
		return v != 0, nil
	case float32, float64:
		if num, ok := toFloat64(v); ok {
			return num != 0, nil
		}
	}

	return arg != nil, nil
}

// 通用复合数据类型字段访问函数

// getVectorFieldValue 获取通用向量数据字段值
func (e *Evaluator) getVectorFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	vectorData, ok := compositeData.(*model.VectorData)
	if !ok {
		return nil, fmt.Errorf("复合数据不是VectorData类型")
	}

	if len(parts) < 2 {
		return nil, fmt.Errorf("向量字段访问需要指定子字段")
	}

	fieldName := parts[1]
	switch fieldName {
	case "dimension":
		return vectorData.Dimension, nil
	case "length":
		return len(vectorData.Values), nil
	case "unit":
		return vectorData.Unit, nil
	default:
		// 检查是否为索引访问 (例如: vector.0, vector.1)
		if idx, err := strconv.Atoi(fieldName); err == nil {
			if idx >= 0 && idx < len(vectorData.Values) {
				return vectorData.Values[idx], nil
			}
			return nil, fmt.Errorf("向量索引超出范围: %d", idx)
		}
		
		// 检查是否为标签访问
		if len(vectorData.Labels) > 0 {
			for i, label := range vectorData.Labels {
				if label == fieldName {
					if i < len(vectorData.Values) {
						return vectorData.Values[i], nil
					}
				}
			}
		}
		
		// 检查衍生值
		if derivedValues := vectorData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		availableFields := []string{"dimension", "length", "unit", "magnitude", "norm", "min", "max", "sum", "mean", "range", "dominant_dimension", "dominant_value"}
		if len(vectorData.Labels) > 0 {
			availableFields = append(availableFields, vectorData.Labels...)
		}
		// 添加索引访问说明
		for i := 0; i < vectorData.Dimension; i++ {
			availableFields = append(availableFields, strconv.Itoa(i))
		}
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("VectorData中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", availableFields)
	}
}

// getArrayFieldValue 获取数组数据字段值
func (e *Evaluator) getArrayFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("复合数据不是ArrayData类型")
	}

	if len(parts) < 2 {
		return nil, fmt.Errorf("数组字段访问需要指定子字段")
	}

	fieldName := parts[1]
	switch fieldName {
	case "size", "length":
		return arrayData.Size, nil
	case "data_type":
		return arrayData.DataType, nil
	case "unit":
		return arrayData.Unit, nil
	default:
		// 检查是否为索引访问 (例如: array.0, array.1)
		if idx, err := strconv.Atoi(fieldName); err == nil {
			if idx >= 0 && idx < len(arrayData.Values) {
				return arrayData.Values[idx], nil
			}
			return nil, fmt.Errorf("数组索引超出范围: %d", idx)
		}
		
		// 检查是否为标签访问
		if len(arrayData.Labels) > 0 {
			for i, label := range arrayData.Labels {
				if label == fieldName {
					if i < len(arrayData.Values) {
						return arrayData.Values[i], nil
					}
				}
			}
		}
		
		// 检查衍生值
		if derivedValues := arrayData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		availableFields := []string{"size", "length", "data_type", "unit", "type_distribution", "numeric_count", "null_count"}
		if arrayData.DataType == "mixed" || arrayData.DataType == "float" || arrayData.DataType == "int" {
			availableFields = append(availableFields, "numeric_min", "numeric_max", "numeric_sum", "numeric_mean", "numeric_range")
		}
		if len(arrayData.Labels) > 0 {
			availableFields = append(availableFields, arrayData.Labels...)
		}
		// 添加索引访问说明
		for i := 0; i < arrayData.Size; i++ {
			availableFields = append(availableFields, strconv.Itoa(i))
		}
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("ArrayData中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", availableFields)
	}
}

// getMatrixFieldValue 获取矩阵数据字段值
func (e *Evaluator) getMatrixFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	matrixData, ok := compositeData.(*model.MatrixData)
	if !ok {
		return nil, fmt.Errorf("复合数据不是MatrixData类型")
	}

	if len(parts) < 2 {
		return nil, fmt.Errorf("矩阵字段访问需要指定子字段")
	}

	fieldName := parts[1]
	switch fieldName {
	case "rows":
		return matrixData.Rows, nil
	case "cols":
		return matrixData.Cols, nil
	case "unit":
		return matrixData.Unit, nil
	default:
		// 检查是否为矩阵元素访问 (例如: matrix.0_1 表示[0][1])
		if strings.Contains(fieldName, "_") {
			coordinates := strings.Split(fieldName, "_")
			if len(coordinates) == 2 {
				if row, err := strconv.Atoi(coordinates[0]); err == nil {
					if col, err := strconv.Atoi(coordinates[1]); err == nil {
						if row >= 0 && row < len(matrixData.Values) && col >= 0 && col < len(matrixData.Values[row]) {
							return matrixData.Values[row][col], nil
						}
						return nil, fmt.Errorf("矩阵索引超出范围: [%d][%d]", row, col)
					}
				}
			}
		}
		
		// 检查衍生值
		if derivedValues := matrixData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		availableFields := []string{"rows", "cols", "unit", "size", "is_square", "min", "max", "sum", "mean", "range"}
		if matrixData.Rows == matrixData.Cols {
			availableFields = append(availableFields, "trace", "is_diagonal", "is_identity")
		}
		// 添加矩阵元素访问说明
		availableFields = append(availableFields, "使用格式: row_col (如: 0_1 表示 [0][1] 元素)")
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("MatrixData中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", availableFields)
	}
}

// getTimeSeriesFieldValue 获取时间序列数据字段值
func (e *Evaluator) getTimeSeriesFieldValue(field string, parts []string, compositeData model.CompositeData) (interface{}, error) {
	timeSeriesData, ok := compositeData.(*model.TimeSeriesData)
	if !ok {
		return nil, fmt.Errorf("复合数据不是TimeSeriesData类型")
	}

	if len(parts) < 2 {
		return nil, fmt.Errorf("时间序列字段访问需要指定子字段")
	}

	fieldName := parts[1]
	switch fieldName {
	case "length":
		return len(timeSeriesData.Values), nil
	case "unit":
		return timeSeriesData.Unit, nil
	case "interval":
		return timeSeriesData.Interval.String(), nil
	case "first_timestamp":
		if len(timeSeriesData.Timestamps) > 0 {
			return timeSeriesData.Timestamps[0], nil
		}
		return nil, nil
	case "last_timestamp":
		if len(timeSeriesData.Timestamps) > 0 {
			return timeSeriesData.Timestamps[len(timeSeriesData.Timestamps)-1], nil
		}
		return nil, nil
	case "first_value":
		if len(timeSeriesData.Values) > 0 {
			return timeSeriesData.Values[0], nil
		}
		return nil, nil
	case "last_value":
		if len(timeSeriesData.Values) > 0 {
			return timeSeriesData.Values[len(timeSeriesData.Values)-1], nil
		}
		return nil, nil
	default:
		// 检查是否为索引访问 (例如: timeseries.0, timeseries.-1 表示最后一个)
		if idx, err := strconv.Atoi(fieldName); err == nil {
			// 支持负数索引
			if idx < 0 {
				idx = len(timeSeriesData.Values) + idx
			}
			if idx >= 0 && idx < len(timeSeriesData.Values) {
				return timeSeriesData.Values[idx], nil
			}
			return nil, fmt.Errorf("时间序列索引超出范围: %d", idx)
		}
		
		// 检查衍生值
		if derivedValues := timeSeriesData.GetDerivedValues(); derivedValues != nil {
			if value, exists := derivedValues[fieldName]; exists {
				return value, nil
			}
		}
		
		availableFields := []string{
			"length", "unit", "interval", "first_timestamp", "last_timestamp", 
			"first_value", "last_value", "duration", "avg_interval", 
			"min", "max", "sum", "mean", "range", "trend_slope", "trend",
		}
		// 添加索引访问说明
		availableFields = append(availableFields, "支持数字索引访问 (如: 0, 1, -1)")
		
		return nil, NewConditionError(ErrCodeConditionField, 
			fmt.Sprintf("TimeSeriesData中未知字段: %s", fieldName), nil).
			WithContext("field", field).
			WithContext("available_fields", availableFields)
	}
}
