package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules"
)

// TransformHandler Transform动作处理器
type TransformHandler struct{
	natsConn *nats.Conn
}

// NewTransformHandler 创建Transform处理器
func NewTransformHandler(natsConn *nats.Conn) *TransformHandler {
	return &TransformHandler{
		natsConn: natsConn,
	}
}

// Name 返回处理器名称
func (h *TransformHandler) Name() string {
	return "transform"
}

// Execute 执行转换动作
func (h *TransformHandler) Execute(ctx context.Context, point model.Point, rule *rules.Rule, config map[string]interface{}) (*rules.ActionResult, error) {
	start := time.Now()

	// 解析配置
	transformConfig, err := h.parseConfig(config)
	if err != nil {
		return &rules.ActionResult{
			Type:     "transform",
			Success:  false,
			Error:    fmt.Sprintf("解析配置失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// 执行转换
	transformedPoint, err := h.transformPoint(point, transformConfig)
	if err != nil {
		return &rules.ActionResult{
			Type:     "transform",
			Success:  false,
			Error:    fmt.Sprintf("数据转换失败: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// 发布转换后的数据到NATS
	var publishSubject string
	var publishError error
	
	if h.natsConn != nil {
		// 获取发布主题
		if subject, ok := config["publish_subject"].(string); ok && subject != "" {
			publishSubject = subject
		} else {
			// 默认主题格式：transformed.{device_id}.{key}
			publishSubject = fmt.Sprintf("transformed.%s.%s", transformedPoint.DeviceID, transformedPoint.Key)
		}

		// 准备发布数据
		publishData := map[string]interface{}{
			"device_id":        transformedPoint.DeviceID,
			"key":              transformedPoint.Key,
			"value":            rules.SafeValueForJSON(transformedPoint.Value),
			"type":             string(transformedPoint.Type),
			"timestamp":        transformedPoint.Timestamp,
			"tags":             rules.SafeValueForJSON(transformedPoint.Tags),
			"transform_info": map[string]interface{}{
				"rule_id":          rule.ID,
				"rule_name":        rule.Name,
				"action":           "transform",
				"transform_type":   transformConfig.Type,
				"original_value":   rules.SafeValueForJSON(point.Value),
				"transformation":   transformConfig.Type,
			},
			"processed_at": time.Now(),
		}

		// 序列化并发布
		if jsonData, err := json.Marshal(publishData); err == nil {
			if err := h.natsConn.Publish(publishSubject, jsonData); err != nil {
				publishError = err
				log.Error().Err(err).Str("subject", publishSubject).Msg("发布转换数据到NATS失败")
			} else {
				log.Debug().
					Str("rule_id", rule.ID).
					Str("subject", publishSubject).
					Int("bytes", len(jsonData)).
					Msg("转换数据发布到NATS成功")
			}
		} else {
			publishError = err
		}
	}

	// 记录转换结果
	log.Debug().
		Str("rule_id", rule.ID).
		Str("device_id", point.DeviceID).
		Str("key", point.Key).
		Interface("original_value", point.Value).
		Interface("transformed_value", transformedPoint.Value).
		Str("transform_type", transformConfig.Type).
		Str("publish_subject", publishSubject).
		Msg("数据转换完成")

	resultOutput := map[string]interface{}{
		"original_point":    point,
		"transformed_point": transformedPoint,
		"transform_type":    transformConfig.Type,
	}

	if publishSubject != "" {
		resultOutput["publish_subject"] = publishSubject
		if publishError != nil {
			resultOutput["publish_error"] = publishError.Error()
		} else {
			resultOutput["published"] = true
		}
	}

	return &rules.ActionResult{
		Type:     "transform",
		Success:  true,
		Duration: time.Since(start),
		Output:   resultOutput,
	}, nil
}

// TransformConfig 转换配置
type TransformConfig struct {
	Type         string                 `json:"type"`          // scale, offset, unit_convert, format, expression, lookup
	Parameters   map[string]interface{} `json:"parameters"`    // 转换参数
	OutputKey    string                 `json:"output_key"`    // 输出字段名（可选）
	OutputType   string                 `json:"output_type"`   // 输出数据类型
	Precision    int                    `json:"precision"`     // 数值精度
	Conditions   []string               `json:"conditions"`    // 转换条件
	ErrorAction  string                 `json:"error_action"`  // 错误处理：ignore, default, error
	DefaultValue interface{}            `json:"default_value"` // 默认值
}

// parseConfig 解析配置
func (h *TransformHandler) parseConfig(config map[string]interface{}) (*TransformConfig, error) {
	transformConfig := &TransformConfig{
		Type:         "scale",
		Parameters:   make(map[string]interface{}),
		OutputKey:    "",
		OutputType:   "",
		Precision:    -1,
		Conditions:   []string{},
		ErrorAction:  "error",
		DefaultValue: nil,
	}

	// 解析转换类型
	if transformType, ok := config["type"].(string); ok {
		transformConfig.Type = transformType
	}

	// 解析参数
	if parameters, ok := config["parameters"].(map[string]interface{}); ok {
		transformConfig.Parameters = parameters
	}

	// 解析输出配置
	if outputKey, ok := config["output_key"].(string); ok {
		transformConfig.OutputKey = outputKey
	}

	if outputType, ok := config["output_type"].(string); ok {
		transformConfig.OutputType = outputType
	}

	if precision, ok := config["precision"].(float64); ok {
		transformConfig.Precision = int(precision)
	}

	// 解析错误处理
	if errorAction, ok := config["error_action"].(string); ok {
		transformConfig.ErrorAction = errorAction
	}

	if defaultValue, ok := config["default_value"]; ok {
		transformConfig.DefaultValue = defaultValue
	}

	// 解析条件
	if conditions, ok := config["conditions"].([]interface{}); ok {
		for _, cond := range conditions {
			if condStr, ok := cond.(string); ok {
				transformConfig.Conditions = append(transformConfig.Conditions, condStr)
			}
		}
	}

	return transformConfig, nil
}

// transformPoint 转换数据点
func (h *TransformHandler) transformPoint(point model.Point, config *TransformConfig) (model.Point, error) {
	// 创建新的数据点
	transformedPoint := point

	// 根据转换类型执行转换
	var transformedValue interface{}
	var err error

	switch config.Type {
	case "scale":
		transformedValue, err = h.scaleTransform(point.Value, config.Parameters)
	case "offset":
		transformedValue, err = h.offsetTransform(point.Value, config.Parameters)
	case "unit_convert":
		transformedValue, err = h.unitConvertTransform(point.Value, config.Parameters)
	case "format":
		transformedValue, err = h.formatTransform(point.Value, config.Parameters)
	case "expression":
		transformedValue, err = h.expressionTransform(point.Value, config.Parameters)
	case "lookup":
		transformedValue, err = h.lookupTransform(point.Value, config.Parameters)
	case "round":
		transformedValue, err = h.roundTransform(point.Value, config.Parameters)
	case "clamp":
		transformedValue, err = h.clampTransform(point.Value, config.Parameters)
	case "map":
		transformedValue, err = h.mapTransform(point.Value, config.Parameters)
	default:
		return point, fmt.Errorf("不支持的转换类型: %s", config.Type)
	}

	// 错误处理
	if err != nil {
		switch config.ErrorAction {
		case "ignore":
			return point, nil
		case "default":
			if config.DefaultValue != nil {
				transformedValue = config.DefaultValue
			} else {
				return point, nil
			}
		default:
			return point, err
		}
	}

	// 应用精度设置
	if config.Precision >= 0 {
		if num, ok := transformedValue.(float64); ok {
			factor := math.Pow(10, float64(config.Precision))
			transformedValue = math.Round(num*factor) / factor
		}
	}

	// 类型转换
	if config.OutputType != "" {
		transformedValue, err = h.convertType(transformedValue, config.OutputType)
		if err != nil {
			return point, fmt.Errorf("类型转换失败: %w", err)
		}
	}

	// 设置转换后的值
	transformedPoint.Value = transformedValue

	// 设置输出字段
	if config.OutputKey != "" {
		transformedPoint.Key = config.OutputKey
	}

	// 更新时间戳
	transformedPoint.Timestamp = time.Now()

	return transformedPoint, nil
}

// scaleTransform 缩放转换
func (h *TransformHandler) scaleTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	factor, ok := params["factor"].(float64)
	if !ok {
		return nil, fmt.Errorf("缩放因子未配置或类型错误")
	}

	num, err := h.toFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("无法转换为数字: %w", err)
	}

	return num * factor, nil
}

// offsetTransform 偏移转换
func (h *TransformHandler) offsetTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	offset, ok := params["offset"].(float64)
	if !ok {
		return nil, fmt.Errorf("偏移量未配置或类型错误")
	}

	num, err := h.toFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("无法转换为数字: %w", err)
	}

	return num + offset, nil
}

// unitConvertTransform 单位转换
func (h *TransformHandler) unitConvertTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	fromUnit, ok := params["from"].(string)
	if !ok {
		return nil, fmt.Errorf("源单位未配置")
	}

	toUnit, ok := params["to"].(string)
	if !ok {
		return nil, fmt.Errorf("目标单位未配置")
	}

	num, err := h.toFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("无法转换为数字: %w", err)
	}

	// 单位转换逻辑
	convertedValue, err := h.convertUnit(num, fromUnit, toUnit)
	if err != nil {
		return nil, fmt.Errorf("单位转换失败: %w", err)
	}

	return convertedValue, nil
}

// formatTransform 格式化转换
func (h *TransformHandler) formatTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	format, ok := params["format"].(string)
	if !ok {
		return nil, fmt.Errorf("格式字符串未配置")
	}

	return fmt.Sprintf(format, value), nil
}

// expressionTransform 表达式转换
func (h *TransformHandler) expressionTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	expression, ok := params["expression"].(string)
	if !ok {
		return nil, fmt.Errorf("表达式未配置")
	}

	// 简单的表达式计算（这里可以集成更强大的表达式引擎）
	return h.evaluateSimpleExpression(expression, value)
}

// lookupTransform 查找表转换
func (h *TransformHandler) lookupTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	lookupTable, ok := params["table"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("查找表未配置")
	}

	key := fmt.Sprintf("%v", value)
	if result, exists := lookupTable[key]; exists {
		return result, nil
	}

	// 检查是否有默认值
	if defaultValue, exists := params["default"]; exists {
		return defaultValue, nil
	}

	return nil, fmt.Errorf("查找表中未找到值: %s", key)
}

// roundTransform 四舍五入转换
func (h *TransformHandler) roundTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	num, err := h.toFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("无法转换为数字: %w", err)
	}

	decimals := 0
	if d, ok := params["decimals"].(float64); ok {
		decimals = int(d)
	}

	factor := math.Pow(10, float64(decimals))
	return math.Round(num*factor) / factor, nil
}

// clampTransform 限幅转换
func (h *TransformHandler) clampTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	num, err := h.toFloat64(value)
	if err != nil {
		return nil, fmt.Errorf("无法转换为数字: %w", err)
	}

	min, hasMin := params["min"].(float64)
	max, hasMax := params["max"].(float64)

	if hasMin && num < min {
		return min, nil
	}
	if hasMax && num > max {
		return max, nil
	}

	return num, nil
}

// mapTransform 映射转换
func (h *TransformHandler) mapTransform(value interface{}, params map[string]interface{}) (interface{}, error) {
	mapping, ok := params["mapping"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("映射表未配置")
	}

	key := fmt.Sprintf("%v", value)
	if result, exists := mapping[key]; exists {
		return result, nil
	}

	return value, nil // 未找到映射时返回原值
}

// convertUnit 单位转换
func (h *TransformHandler) convertUnit(value float64, fromUnit, toUnit string) (float64, error) {
	// 温度转换
	if strings.Contains(fromUnit, "C") || strings.Contains(fromUnit, "F") || strings.Contains(fromUnit, "K") {
		return h.convertTemperature(value, fromUnit, toUnit)
	}

	// 长度转换
	if h.isLengthUnit(fromUnit) && h.isLengthUnit(toUnit) {
		return h.convertLength(value, fromUnit, toUnit)
	}

	// 重量转换
	if h.isWeightUnit(fromUnit) && h.isWeightUnit(toUnit) {
		return h.convertWeight(value, fromUnit, toUnit)
	}

	return value, fmt.Errorf("不支持的单位转换: %s -> %s", fromUnit, toUnit)
}

// convertTemperature 温度转换
func (h *TransformHandler) convertTemperature(value float64, from, to string) (float64, error) {
	// 先转换为摄氏度
	var celsius float64
	switch strings.ToUpper(from) {
	case "C", "CELSIUS":
		celsius = value
	case "F", "FAHRENHEIT":
		celsius = (value - 32) * 5 / 9
	case "K", "KELVIN":
		celsius = value - 273.15
	default:
		return value, fmt.Errorf("不支持的温度单位: %s", from)
	}

	// 从摄氏度转换为目标单位
	switch strings.ToUpper(to) {
	case "C", "CELSIUS":
		return celsius, nil
	case "F", "FAHRENHEIT":
		return celsius*9/5 + 32, nil
	case "K", "KELVIN":
		return celsius + 273.15, nil
	default:
		return value, fmt.Errorf("不支持的温度单位: %s", to)
	}
}

// convertLength 长度转换
func (h *TransformHandler) convertLength(value float64, from, to string) (float64, error) {
	// 转换为米
	var meters float64
	switch strings.ToLower(from) {
	case "mm", "millimeter":
		meters = value / 1000
	case "cm", "centimeter":
		meters = value / 100
	case "m", "meter":
		meters = value
	case "km", "kilometer":
		meters = value * 1000
	case "in", "inch":
		meters = value * 0.0254
	case "ft", "foot":
		meters = value * 0.3048
	default:
		return value, fmt.Errorf("不支持的长度单位: %s", from)
	}

	// 从米转换为目标单位
	switch strings.ToLower(to) {
	case "mm", "millimeter":
		return meters * 1000, nil
	case "cm", "centimeter":
		return meters * 100, nil
	case "m", "meter":
		return meters, nil
	case "km", "kilometer":
		return meters / 1000, nil
	case "in", "inch":
		return meters / 0.0254, nil
	case "ft", "foot":
		return meters / 0.3048, nil
	default:
		return value, fmt.Errorf("不支持的长度单位: %s", to)
	}
}

// convertWeight 重量转换
func (h *TransformHandler) convertWeight(value float64, from, to string) (float64, error) {
	// 转换为克
	var grams float64
	switch strings.ToLower(from) {
	case "mg", "milligram":
		grams = value / 1000
	case "g", "gram":
		grams = value
	case "kg", "kilogram":
		grams = value * 1000
	case "oz", "ounce":
		grams = value * 28.3495
	case "lb", "pound":
		grams = value * 453.592
	default:
		return value, fmt.Errorf("不支持的重量单位: %s", from)
	}

	// 从克转换为目标单位
	switch strings.ToLower(to) {
	case "mg", "milligram":
		return grams * 1000, nil
	case "g", "gram":
		return grams, nil
	case "kg", "kilogram":
		return grams / 1000, nil
	case "oz", "ounce":
		return grams / 28.3495, nil
	case "lb", "pound":
		return grams / 453.592, nil
	default:
		return value, fmt.Errorf("不支持的重量单位: %s", to)
	}
}

// isLengthUnit 检查是否是长度单位
func (h *TransformHandler) isLengthUnit(unit string) bool {
	lengthUnits := []string{"mm", "cm", "m", "km", "in", "ft", "millimeter", "centimeter", "meter", "kilometer", "inch", "foot"}
	unit = strings.ToLower(unit)
	for _, u := range lengthUnits {
		if u == unit {
			return true
		}
	}
	return false
}

// isWeightUnit 检查是否是重量单位
func (h *TransformHandler) isWeightUnit(unit string) bool {
	weightUnits := []string{"mg", "g", "kg", "oz", "lb", "milligram", "gram", "kilogram", "ounce", "pound"}
	unit = strings.ToLower(unit)
	for _, u := range weightUnits {
		if u == unit {
			return true
		}
	}
	return false
}

// evaluateSimpleExpression 计算简单表达式
func (h *TransformHandler) evaluateSimpleExpression(expression string, value interface{}) (interface{}, error) {
	// 替换变量
	expr := strings.ReplaceAll(expression, "x", fmt.Sprintf("%v", value))

	// 简单的数学表达式计算（这里可以集成更强大的表达式引擎）
	// 目前只支持基本的四则运算
	return h.calculateExpression(expr)
}

// calculateExpression 计算表达式
func (h *TransformHandler) calculateExpression(expr string) (interface{}, error) {
	// 移除空格
	expr = strings.ReplaceAll(expr, " ", "")
	
	// 尝试直接解析为数字
	if num, err := strconv.ParseFloat(expr, 64); err == nil {
		return num, nil
	}
	
	// 使用递归下降解析器进行表达式计算
	parser := &ExpressionParser{expr: expr, pos: 0}
	result, err := parser.parseExpression()
	if err != nil {
		return nil, fmt.Errorf("表达式计算失败: %w", err)
	}
	
	if parser.pos < len(parser.expr) {
		return nil, fmt.Errorf("表达式有未解析的部分: %s", expr[parser.pos:])
	}
	
	return result, nil
}

// ExpressionParser 表达式解析器
type ExpressionParser struct {
	expr string
	pos  int
}

// parseExpression 解析表达式 (处理 + 和 -)
func (p *ExpressionParser) parseExpression() (float64, error) {
	left, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	
	for p.pos < len(p.expr) {
		op := p.peek()
		if op != '+' && op != '-' {
			break
		}
		p.advance()
		
		right, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
	
	return left, nil
}

// parseTerm 解析项 (处理 * 和 /)
func (p *ExpressionParser) parseTerm() (float64, error) {
	left, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	
	for p.pos < len(p.expr) {
		op := p.peek()
		if op != '*' && op != '/' && op != '%' {
			break
		}
		p.advance()
		
		right, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		
		switch op {
		case '*':
			left *= right
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("除零错误")
			}
			left /= right
		case '%':
			if right == 0 {
				return 0, fmt.Errorf("取模零错误")
			}
			left = math.Mod(left, right)
		}
	}
	
	return left, nil
}

// parseFactor 解析因子 (处理数字、括号、函数和负号)
func (p *ExpressionParser) parseFactor() (float64, error) {
	// 处理负号
	if p.peek() == '-' {
		p.advance()
		factor, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -factor, nil
	}
	
	// 处理正号
	if p.peek() == '+' {
		p.advance()
		return p.parseFactor()
	}
	
	// 处理括号
	if p.peek() == '(' {
		p.advance()
		result, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("缺少右括号")
		}
		p.advance()
		return result, nil
	}
	
	// 处理函数调用
	if p.isAlpha(p.peek()) {
		return p.parseFunction()
	}
	
	// 处理数字
	return p.parseNumber()
}

// parseFunction 解析函数调用
func (p *ExpressionParser) parseFunction() (float64, error) {
	start := p.pos
	for p.pos < len(p.expr) && (p.isAlpha(p.peek()) || p.isDigit(p.peek())) {
		p.advance()
	}
	
	funcName := p.expr[start:p.pos]
	
	if p.peek() != '(' {
		return 0, fmt.Errorf("函数调用缺少左括号: %s", funcName)
	}
	p.advance()
	
	arg, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	
	if p.peek() != ')' {
		return 0, fmt.Errorf("函数调用缺少右括号: %s", funcName)
	}
	p.advance()
	
	// 计算函数值
	switch strings.ToLower(funcName) {
	case "abs":
		return math.Abs(arg), nil
	case "sqrt":
		if arg < 0 {
			return 0, fmt.Errorf("sqrt参数不能为负数")
		}
		return math.Sqrt(arg), nil
	case "sin":
		return math.Sin(arg), nil
	case "cos":
		return math.Cos(arg), nil
	case "tan":
		return math.Tan(arg), nil
	case "ln":
		if arg <= 0 {
			return 0, fmt.Errorf("ln参数必须大于0")
		}
		return math.Log(arg), nil
	case "log":
		if arg <= 0 {
			return 0, fmt.Errorf("log参数必须大于0")
		}
		return math.Log10(arg), nil
	case "exp":
		return math.Exp(arg), nil
	case "floor":
		return math.Floor(arg), nil
	case "ceil":
		return math.Ceil(arg), nil
	case "round":
		return math.Round(arg), nil
	default:
		return 0, fmt.Errorf("未知函数: %s", funcName)
	}
}

// parseNumber 解析数字
func (p *ExpressionParser) parseNumber() (float64, error) {
	start := p.pos
	
	// 处理数字部分
	for p.pos < len(p.expr) && (p.isDigit(p.peek()) || p.peek() == '.') {
		p.advance()
	}
	
	if start == p.pos {
		return 0, fmt.Errorf("期望数字，位置: %d", p.pos)
	}
	
	numStr := p.expr[start:p.pos]
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("无效数字: %s", numStr)
	}
	
	return num, nil
}

// peek 查看当前字符
func (p *ExpressionParser) peek() byte {
	if p.pos >= len(p.expr) {
		return 0
	}
	return p.expr[p.pos]
}

// advance 移动到下一个字符
func (p *ExpressionParser) advance() {
	if p.pos < len(p.expr) {
		p.pos++
	}
}

// isDigit 检查是否为数字字符
func (p *ExpressionParser) isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

// isAlpha 检查是否为字母字符
func (p *ExpressionParser) isAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

// convertType 类型转换
func (h *TransformHandler) convertType(value interface{}, targetType string) (interface{}, error) {
	switch strings.ToLower(targetType) {
	case "string":
		return fmt.Sprintf("%v", value), nil
	case "int":
		if num, err := h.toFloat64(value); err == nil {
			return int(num), nil
		}
		return nil, fmt.Errorf("无法转换为整数")
	case "float":
		return h.toFloat64(value)
	case "bool":
		return h.toBool(value), nil
	default:
		return value, nil
	}
}

// toFloat64 转换为float64
func (h *TransformHandler) toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("无法转换为数字: %T", value)
	}
}

// toBool 转换为bool
func (h *TransformHandler) toBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true" || v == "1"
	case int, int32, int64:
		return v != 0
	case float32, float64:
		if num, ok := value.(float64); ok {
			return num != 0
		}
		if num, ok := value.(float32); ok {
			return num != 0
		}
	}
	return value != nil
}
