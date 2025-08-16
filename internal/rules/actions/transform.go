package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
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

	// 记录transform action被调用
	log.Info().
		Str("rule_id", rule.ID).
		Str("device_id", point.DeviceID).
		Str("key", point.Key).
		Interface("original_tags", point.GetTagsCopy()).
		Interface("config", config).
		Msg("🔄 Transform action被执行")

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
			"tags":             rules.SafeValueForJSON(transformedPoint.GetTagsCopy()),
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
	AddTags      map[string]string      `json:"add_tags"`      // 添加标签
}

// parseConfig 解析配置
func (h *TransformHandler) parseConfig(config map[string]interface{}) (*TransformConfig, error) {
	transformConfig := &TransformConfig{
		Type:         "pass_through", // 默认为透传类型
		Parameters:   make(map[string]interface{}),
		OutputKey:    "",
		OutputType:   "",
		Precision:    -1,
		Conditions:   []string{},
		ErrorAction:  "error",
		DefaultValue: nil,
		AddTags:      make(map[string]string),
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

	// 解析添加标签
	if addTags, ok := config["add_tags"].(map[string]interface{}); ok {
		for k, v := range addTags {
			if strVal, ok := v.(string); ok {
				transformConfig.AddTags[k] = strVal
			}
		}
	}

	return transformConfig, nil
}

// transformPoint 转换数据点
func (h *TransformHandler) transformPoint(point model.Point, config *TransformConfig) (model.Point, error) {
	// 创建新的数据点
	transformedPoint := point

	// 检查是否为复合数据处理
	if point.IsComposite() {
		return h.transformCompositePoint(point, config)
	}

	// 根据转换类型执行转换
	var transformedValue interface{}
	var err error

	switch config.Type {
	case "pass_through":
		// 透传模式：不修改值，只添加标签或其他元数据
		transformedValue = point.Value
		err = nil
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

	// 设置输出字段（支持模板替换）
	if config.OutputKey != "" {
		transformedPoint.Key = h.parseTemplateString(config.OutputKey, point)
	}

	// 添加新标签（保留原有标签）
	if len(config.AddTags) > 0 {
		// Go 1.24安全：Tags字段通过AddTag方法自动初始化
		
		// 记录标签合并过程
		log.Debug().
			Interface("original_tags", point.GetTagsCopy()).
			Interface("add_tags", config.AddTags).
			Msg("Transform action标签合并过程")
			
		for k, v := range config.AddTags {
			// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
			transformedPoint.AddTag(k, v)
		}
		
		// 记录合并后的标签
		log.Debug().
			Interface("merged_tags", transformedPoint.GetTagsCopy()).
			Msg("Transform action标签合并完成")
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

// transformCompositePoint 转换复合数据点
func (h *TransformHandler) transformCompositePoint(point model.Point, config *TransformConfig) (model.Point, error) {
	compositeData, err := point.GetCompositeData()
	if err != nil {
		return point, fmt.Errorf("获取复合数据失败: %w", err)
	}

	var transformedValue interface{}
	var newDataType model.DataType

	switch config.Type {
	case "pass_through":
		// 透传模式
		transformedValue = compositeData
		newDataType = point.Type
	case "geo_distance":
		// 地理距离计算
		transformedValue, err = h.geoDistanceTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "geo_bearing":
		// 地理方位角计算
		transformedValue, err = h.geoBearingTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "geo_transform":
		// GPS通用变换操作，根据sub_type参数决定具体操作
		transformedValue, newDataType, err = h.GeoTransformDispatch(compositeData, config.Parameters)
	case "vector_transform":
		// 3D向量通用变换操作，根据sub_type参数决定具体操作
		transformedValue, newDataType, err = h.VectorTransformDispatch(compositeData, config.Parameters)
	case "vector_magnitude":
		// 向量模长计算
		transformedValue, err = h.vectorMagnitudeTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "vector_normalize":
		// 向量归一化
		transformedValue, err = h.vectorNormalizeTransform(compositeData, config.Parameters)
		newDataType = model.TypeVector3D
	case "extract_field":
		// 提取复合数据字段
		transformedValue, newDataType, err = h.extractFieldTransform(point, compositeData, config.Parameters)
	case "color_convert":
		// 颜色空间转换
		transformedValue, err = h.colorConvertTransform(compositeData, config.Parameters)
		newDataType = point.Type
	// 通用复合数据类型转换
	case "vector_stats":
		// 向量统计计算
		transformedValue, err = h.vectorStatsTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "array_aggregate":
		// 数组聚合操作
		transformedValue, err = h.arrayAggregateTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "matrix_operation":
		// 矩阵操作
		transformedValue, err = h.matrixOperationTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "timeseries_analysis":
		// 时间序列分析
		transformedValue, err = h.timeseriesAnalysisTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "composite_to_array":
		// 复合数据转换为数组
		transformedValue, err = h.compositeToArrayTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "array_filter":
		// 数组过滤操作
		transformedValue, err = h.arrayFilterTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "array_sort":
		// 数组排序操作
		transformedValue, err = h.arraySortTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "array_slice":
		// 数组切片操作
		transformedValue, err = h.arraySliceTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "array_smooth":
		// 数组平滑操作
		transformedValue, err = h.arraySmoothTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "array_normalize":
		// 数组归一化操作
		transformedValue, err = h.arrayNormalizeTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "array_transform":
		// 数组变换操作
		transformedValue, err = h.arrayTransformTransform(compositeData, config.Parameters)
		newDataType = model.TypeArray
	case "geo_geofence":
		// 地理围栏检查
		transformedValue, err = h.geoGeofenceTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "geo_coordinate_convert":
		// 坐标系转换
		transformedValue, err = h.geoCoordinateConvertTransform(compositeData, config.Parameters)
		newDataType = model.TypeLocation
	case "vector_projection":
		// 向量投影
		transformedValue, err = h.vectorProjectionTransform(compositeData, config.Parameters)
		newDataType = model.TypeVector3D
	case "vector_cross":
		// 向量叉积
		transformedValue, err = h.vectorCrossTransform(compositeData, config.Parameters)
		newDataType = model.TypeVector3D
	case "vector_dot":
		// 向量点积
		transformedValue, err = h.vectorDotTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "color_similarity":
		// 颜色相似度计算
		transformedValue, err = h.colorSimilarityTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	case "color_extract_dominant":
		// 主色调提取
		transformedValue, err = h.colorExtractDominantTransform(compositeData, config.Parameters)
		newDataType = model.TypeFloat
	default:
		// 对于不支持的转换类型，尝试将复合数据转换为可处理的数值类型
		if numericValue, extractErr := h.extractNumericValue(compositeData, config.Parameters); extractErr == nil {
			// 递归调用普通转换
			tempPoint := model.NewPoint(point.Key, point.DeviceID, numericValue, model.TypeFloat)
			tempPoint.SetTagsSafe(point.GetTagsCopy())
			return h.transformPoint(tempPoint, config)
		}
		return point, fmt.Errorf("复合数据类型不支持转换类型: %s", config.Type)
	}

	if err != nil {
		return point, fmt.Errorf("复合数据转换失败: %w", err)
	}

	// 创建转换后的点
	var transformedPoint model.Point
	if compositeResult, ok := transformedValue.(model.CompositeData); ok {
		transformedPoint = model.NewCompositePoint(
			config.OutputKey, 
			point.DeviceID, 
			compositeResult,
		)
	} else {
		transformedPoint = model.NewPoint(
			config.OutputKey,
			point.DeviceID,
			transformedValue,
			newDataType,
		)
	}

	// 使用原始key如果没有指定输出key
	if config.OutputKey == "" {
		transformedPoint.Key = point.Key
	}

	// Go 1.24安全：使用GetTagsSafe复制原始标签
	originalTags := point.GetTagsSafe()
	for k, v := range originalTags {
		transformedPoint.AddTag(k, v)
	}

	// 添加新标签
	if len(config.AddTags) > 0 {
		// Go 1.24安全：Tags字段通过AddTag方法自动初始化
		for k, v := range config.AddTags {
			// Go 1.24安全：使用AddTag方法替代直接Tags[]访问
			transformedPoint.AddTag(k, v)
		}
	}

	transformedPoint.Timestamp = time.Now()
	return transformedPoint, nil
}

// geoDistanceTransform 地理距离计算转换
func (h *TransformHandler) geoDistanceTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	locationData, ok := compositeData.(*model.LocationData)
	if !ok {
		return nil, fmt.Errorf("数据类型不是LocationData")
	}

	// 获取目标坐标
	targetLat, ok := params["target_latitude"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少目标纬度参数")
	}

	targetLng, ok := params["target_longitude"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少目标经度参数")
	}

	// 使用Haversine公式计算距离
	distance := h.haversineDistance(locationData.Latitude, locationData.Longitude, targetLat, targetLng)

	// 单位转换
	unit := "m"
	if unitParam, ok := params["unit"].(string); ok {
		unit = unitParam
	}

	switch strings.ToLower(unit) {
	case "km":
		return distance / 1000, nil
	case "mi", "miles":
		return distance / 1609.34, nil
	default:
		return distance, nil
	}
}

// geoBearingTransform 地理方位角计算转换
func (h *TransformHandler) geoBearingTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	locationData, ok := compositeData.(*model.LocationData)
	if !ok {
		return nil, fmt.Errorf("数据类型不是LocationData")
	}

	targetLat, ok := params["target_latitude"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少目标纬度参数")
	}

	targetLng, ok := params["target_longitude"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少目标经度参数")
	}

	bearing := h.calculateBearing(locationData.Latitude, locationData.Longitude, targetLat, targetLng)
	return bearing, nil
}

// vectorMagnitudeTransform 向量模长计算转换
func (h *TransformHandler) vectorMagnitudeTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.Vector3D)
	if !ok {
		return nil, fmt.Errorf("数据类型不是Vector3D")
	}

	magnitude := math.Sqrt(vectorData.X*vectorData.X + vectorData.Y*vectorData.Y + vectorData.Z*vectorData.Z)
	return magnitude, nil
}

// vectorNormalizeTransform 向量归一化转换
func (h *TransformHandler) vectorNormalizeTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.Vector3D)
	if !ok {
		return nil, fmt.Errorf("数据类型不是Vector3D")
	}

	magnitude := math.Sqrt(vectorData.X*vectorData.X + vectorData.Y*vectorData.Y + vectorData.Z*vectorData.Z)
	if magnitude == 0 {
		return &model.Vector3D{X: 0, Y: 0, Z: 0}, nil
	}

	return &model.Vector3D{
		X: vectorData.X / magnitude,
		Y: vectorData.Y / magnitude,
		Z: vectorData.Z / magnitude,
	}, nil
}

// extractFieldTransform 提取复合数据字段转换
func (h *TransformHandler) extractFieldTransform(point model.Point, compositeData model.CompositeData, params map[string]interface{}) (interface{}, model.DataType, error) {
	fieldName, ok := params["field"].(string)
	if !ok {
		return nil, model.TypeString, fmt.Errorf("缺少field参数")
	}

	switch point.Type {
	case model.TypeLocation:
		locationData := compositeData.(*model.LocationData)
		switch fieldName {
		case "latitude":
			return locationData.Latitude, model.TypeFloat, nil
		case "longitude":
			return locationData.Longitude, model.TypeFloat, nil
		case "altitude":
			return locationData.Altitude, model.TypeFloat, nil
		case "speed":
			return locationData.Speed, model.TypeFloat, nil
		case "heading":
			return locationData.Heading, model.TypeFloat, nil
		case "accuracy":
			return locationData.Accuracy, model.TypeFloat, nil
		}

	case model.TypeVector3D:
		vectorData := compositeData.(*model.Vector3D)
		switch fieldName {
		case "x":
			return vectorData.X, model.TypeFloat, nil
		case "y":
			return vectorData.Y, model.TypeFloat, nil
		case "z":
			return vectorData.Z, model.TypeFloat, nil
		}

	case model.TypeColor:
		colorData := compositeData.(*model.ColorData)
		switch fieldName {
		case "r", "red":
			return int(colorData.R), model.TypeInt, nil
		case "g", "green":
			return int(colorData.G), model.TypeInt, nil
		case "b", "blue":
			return int(colorData.B), model.TypeInt, nil
		case "a", "alpha":
			return int(colorData.A), model.TypeInt, nil
		}
	}

	// 检查衍生值
	if derivedValues := compositeData.GetDerivedValues(); derivedValues != nil {
		if value, exists := derivedValues[fieldName]; exists {
			// 根据值类型确定数据类型
			switch v := value.(type) {
			case int:
				return v, model.TypeInt, nil
			case float64:
				return v, model.TypeFloat, nil
			case bool:
				return v, model.TypeBool, nil
			default:
				return fmt.Sprintf("%v", v), model.TypeString, nil
			}
		}
	}

	return nil, model.TypeString, fmt.Errorf("未知字段: %s", fieldName)
}

// colorConvertTransform 颜色空间转换
func (h *TransformHandler) colorConvertTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	colorData, ok := compositeData.(*model.ColorData)
	if !ok {
		return nil, fmt.Errorf("数据类型不是ColorData")
	}

	// 目前保持原样，未来可以添加RGB到HSV等转换
	return colorData, nil
}

// extractNumericValue 从复合数据中提取数值
func (h *TransformHandler) extractNumericValue(compositeData model.CompositeData, params map[string]interface{}) (float64, error) {
	// 尝试提取可用于数学运算的数值
	switch data := compositeData.(type) {
	case *model.LocationData:
		if field, ok := params["extract_field"].(string); ok {
			switch field {
			case "latitude":
				return data.Latitude, nil
			case "longitude":
				return data.Longitude, nil
			case "altitude":
				return data.Altitude, nil
			case "speed":
				return data.Speed, nil
			case "heading":
				return data.Heading, nil
			case "accuracy":
				return data.Accuracy, nil
			}
		}
		// 默认返回纬度
		return data.Latitude, nil

	case *model.Vector3D:
		if field, ok := params["extract_field"].(string); ok {
			switch field {
			case "x":
				return data.X, nil
			case "y":
				return data.Y, nil
			case "z":
				return data.Z, nil
			case "magnitude":
				return math.Sqrt(data.X*data.X + data.Y*data.Y + data.Z*data.Z), nil
			}
		}
		// 默认返回模长
		return math.Sqrt(data.X*data.X + data.Y*data.Y + data.Z*data.Z), nil

	case *model.ColorData:
		if field, ok := params["extract_field"].(string); ok {
			switch field {
			case "r", "red":
				return float64(data.R), nil
			case "g", "green":
				return float64(data.G), nil
			case "b", "blue":
				return float64(data.B), nil
			case "a", "alpha":
				return float64(data.A), nil
			}
		}
		// 默认返回亮度
		r := float64(data.R) / 255.0
		g := float64(data.G) / 255.0
		b := float64(data.B) / 255.0
		return (math.Max(r, math.Max(g, b)) + math.Min(r, math.Min(g, b))) / 2.0, nil
	}

	return 0, fmt.Errorf("无法从复合数据中提取数值")
}

// haversineDistance 使用Haversine公式计算地理距离（返回米）
func (h *TransformHandler) haversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371000.0 // 地球半径（米）

	// 转换为弧度
	lat1Rad := lat1 * math.Pi / 180
	lng1Rad := lng1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lng2Rad := lng2 * math.Pi / 180

	// Haversine公式
	dlat := lat2Rad - lat1Rad
	dlng := lng2Rad - lng1Rad

	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dlng/2)*math.Sin(dlng/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

// calculateBearing 计算方位角（度）
func (h *TransformHandler) calculateBearing(lat1, lng1, lat2, lng2 float64) float64 {
	// 转换为弧度
	lat1Rad := lat1 * math.Pi / 180
	lng1Rad := lng1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lng2Rad := lng2 * math.Pi / 180

	dlng := lng2Rad - lng1Rad

	y := math.Sin(dlng) * math.Cos(lat2Rad)
	x := math.Cos(lat1Rad)*math.Sin(lat2Rad) - math.Sin(lat1Rad)*math.Cos(lat2Rad)*math.Cos(dlng)

	bearing := math.Atan2(y, x)
	bearing = bearing * 180 / math.Pi
	bearing = math.Mod(bearing+360, 360)

	return bearing
}

// 通用复合数据转换函数

// vectorStatsTransform 向量统计计算转换
func (h *TransformHandler) vectorStatsTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.VectorData)
	if !ok {
		return nil, fmt.Errorf("数据不是VectorData类型")
	}

	statType, ok := parameters["stat_type"].(string)
	if !ok {
		statType = "magnitude" // 默认计算模长
	}

	switch statType {
	case "magnitude", "norm":
		// 计算向量模长
		sumSquares := 0.0
		for _, val := range vectorData.Values {
			sumSquares += val * val
		}
		return math.Sqrt(sumSquares), nil

	case "sum":
		// 计算向量元素总和
		sum := 0.0
		for _, val := range vectorData.Values {
			sum += val
		}
		return sum, nil

	case "mean", "average":
		// 计算向量元素平均值
		if len(vectorData.Values) == 0 {
			return 0.0, nil
		}
		sum := 0.0
		for _, val := range vectorData.Values {
			sum += val
		}
		return sum / float64(len(vectorData.Values)), nil

	case "min":
		// 计算最小值
		if len(vectorData.Values) == 0 {
			return 0.0, nil
		}
		min := vectorData.Values[0]
		for _, val := range vectorData.Values[1:] {
			if val < min {
				min = val
			}
		}
		return min, nil

	case "max":
		// 计算最大值
		if len(vectorData.Values) == 0 {
			return 0.0, nil
		}
		max := vectorData.Values[0]
		for _, val := range vectorData.Values[1:] {
			if val > max {
				max = val
			}
		}
		return max, nil

	case "index":
		// 获取指定索引的值
		if idx, exists := parameters["index"]; exists {
			if index, ok := idx.(int); ok {
				if index >= 0 && index < len(vectorData.Values) {
					return vectorData.Values[index], nil
				}
			}
			if indexFloat, ok := idx.(float64); ok {
				index := int(indexFloat)
				if index >= 0 && index < len(vectorData.Values) {
					return vectorData.Values[index], nil
				}
			}
		}
		return nil, fmt.Errorf("索引参数无效或超出范围")

	default:
		return nil, fmt.Errorf("不支持的统计类型: %s", statType)
	}
}

// arrayAggregateTransform 数组聚合操作转换
func (h *TransformHandler) arrayAggregateTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	operation, ok := parameters["operation"].(string)
	if !ok {
		operation = "count" // 默认计算数量
	}

	switch operation {
	case "count", "length", "size":
		return len(arrayData.Values), nil

	case "sum":
		// 计算数值元素总和
		sum := 0.0
		count := 0
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				sum += num
				count++
			}
		}
		if count == 0 {
			return 0.0, nil
		}
		return sum, nil

	case "average", "mean":
		// 计算数值元素平均值
		sum := 0.0
		count := 0
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				sum += num
				count++
			}
		}
		if count == 0 {
			return 0.0, nil
		}
		return sum / float64(count), nil

	case "min":
		// 计算数值元素最小值
		var min float64
		found := false
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				if !found || num < min {
					min = num
					found = true
				}
			}
		}
		if !found {
			return 0.0, nil
		}
		return min, nil

	case "max":
		// 计算数值元素最大值
		var max float64
		found := false
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				if !found || num > max {
					max = num
					found = true
				}
			}
		}
		if !found {
			return 0.0, nil
		}
		return max, nil

	case "null_count":
		// 计算null值数量
		count := 0
		for _, val := range arrayData.Values {
			if val == nil {
				count++
			}
		}
		return count, nil

	case "non_null_count":
		// 计算非null值数量
		count := 0
		for _, val := range arrayData.Values {
			if val != nil {
				count++
			}
		}
		return count, nil

	case "index":
		// 获取指定索引的值，转换为数值
		if idx, exists := parameters["index"]; exists {
			if index, ok := idx.(int); ok {
				if index >= 0 && index < len(arrayData.Values) {
					return h.toFloat64(arrayData.Values[index])
				}
			}
			if indexFloat, ok := idx.(float64); ok {
				index := int(indexFloat)
				if index >= 0 && index < len(arrayData.Values) {
					return h.toFloat64(arrayData.Values[index])
				}
			}
		}
		return nil, fmt.Errorf("索引参数无效或超出范围")

	case "std", "stddev":
		// 计算标准差
		numericValues := make([]float64, 0)
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				numericValues = append(numericValues, num)
			}
		}
		if len(numericValues) < 2 {
			return 0.0, nil
		}

		// 计算均值
		mean := 0.0
		for _, v := range numericValues {
			mean += v
		}
		mean /= float64(len(numericValues))

		// 计算方差
		variance := 0.0
		for _, v := range numericValues {
			diff := v - mean
			variance += diff * diff
		}
		variance /= float64(len(numericValues) - 1)
		return math.Sqrt(variance), nil

	case "median":
		// 计算中位数
		numericValues := make([]float64, 0)
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				numericValues = append(numericValues, num)
			}
		}
		if len(numericValues) == 0 {
			return 0.0, nil
		}

		// 排序并获取中位数
		sort.Float64s(numericValues)
		n := len(numericValues)
		if n%2 == 0 {
			return (numericValues[n/2-1] + numericValues[n/2]) / 2, nil
		} else {
			return numericValues[n/2], nil
		}

	case "p90":
		// 计算90分位数
		return h.calculatePercentile(arrayData, 90)

	case "p95":
		// 计算95分位数
		return h.calculatePercentile(arrayData, 95)

	case "p99":
		// 计算99分位数
		return h.calculatePercentile(arrayData, 99)

	case "p25":
		// 计算25分位数
		return h.calculatePercentile(arrayData, 25)

	case "p50":
		// 计算50分位数（中位数）
		return h.calculatePercentile(arrayData, 50)

	case "p75":
		// 计算75分位数
		return h.calculatePercentile(arrayData, 75)

	default:
		return nil, fmt.Errorf("不支持的聚合操作: %s", operation)
	}
}

// matrixOperationTransform 矩阵操作转换
func (h *TransformHandler) matrixOperationTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	matrixData, ok := compositeData.(*model.MatrixData)
	if !ok {
		return nil, fmt.Errorf("数据不是MatrixData类型")
	}

	operation, ok := parameters["operation"].(string)
	if !ok {
		operation = "sum" // 默认计算所有元素总和
	}

	switch operation {
	case "sum":
		// 计算所有元素总和
		sum := 0.0
		for _, row := range matrixData.Values {
			for _, val := range row {
				sum += val
			}
		}
		return sum, nil

	case "mean", "average":
		// 计算所有元素平均值
		sum := 0.0
		count := 0
		for _, row := range matrixData.Values {
			for _, val := range row {
				sum += val
				count++
			}
		}
		if count == 0 {
			return 0.0, nil
		}
		return sum / float64(count), nil

	case "min":
		// 计算最小元素
		if len(matrixData.Values) == 0 || len(matrixData.Values[0]) == 0 {
			return 0.0, nil
		}
		min := matrixData.Values[0][0]
		for _, row := range matrixData.Values {
			for _, val := range row {
				if val < min {
					min = val
				}
			}
		}
		return min, nil

	case "max":
		// 计算最大元素
		if len(matrixData.Values) == 0 || len(matrixData.Values[0]) == 0 {
			return 0.0, nil
		}
		max := matrixData.Values[0][0]
		for _, row := range matrixData.Values {
			for _, val := range row {
				if val > max {
					max = val
				}
			}
		}
		return max, nil

	case "trace":
		// 计算矩阵的迹（对角线元素之和）
		if matrixData.Rows != matrixData.Cols {
			return nil, fmt.Errorf("只能计算方阵的迹")
		}
		trace := 0.0
		for i := 0; i < matrixData.Rows; i++ {
			trace += matrixData.Values[i][i]
		}
		return trace, nil

	case "determinant":
		// 计算行列式（仅支持2x2和3x3矩阵）
		if matrixData.Rows != matrixData.Cols {
			return nil, fmt.Errorf("只能计算方阵的行列式")
		}
		if matrixData.Rows == 2 {
			return matrixData.Values[0][0]*matrixData.Values[1][1] - matrixData.Values[0][1]*matrixData.Values[1][0], nil
		}
		if matrixData.Rows == 3 {
			m := matrixData.Values
			det := m[0][0]*(m[1][1]*m[2][2] - m[1][2]*m[2][1]) -
				  m[0][1]*(m[1][0]*m[2][2] - m[1][2]*m[2][0]) +
				  m[0][2]*(m[1][0]*m[2][1] - m[1][1]*m[2][0])
			return det, nil
		}
		return nil, fmt.Errorf("仅支持2x2或3x3矩阵的行列式计算")

	case "element":
		// 获取指定位置的元素
		if rowParam, exists := parameters["row"]; exists {
			if colParam, exists := parameters["col"]; exists {
				var row, col int
				if r, ok := rowParam.(int); ok {
					row = r
				} else if r, ok := rowParam.(float64); ok {
					row = int(r)
				} else {
					return nil, fmt.Errorf("行索引参数无效")
				}
				
				if c, ok := colParam.(int); ok {
					col = c
				} else if c, ok := colParam.(float64); ok {
					col = int(c)
				} else {
					return nil, fmt.Errorf("列索引参数无效")
				}
				
				if row >= 0 && row < len(matrixData.Values) && col >= 0 && col < len(matrixData.Values[row]) {
					return matrixData.Values[row][col], nil
				}
				return nil, fmt.Errorf("矩阵索引超出范围: [%d][%d]", row, col)
			}
		}
		return nil, fmt.Errorf("缺少行或列索引参数")

	default:
		return nil, fmt.Errorf("不支持的矩阵操作: %s", operation)
	}
}

// timeseriesAnalysisTransform 时间序列分析转换
func (h *TransformHandler) timeseriesAnalysisTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	timeseriesData, ok := compositeData.(*model.TimeSeriesData)
	if !ok {
		return nil, fmt.Errorf("数据不是TimeSeriesData类型")
	}

	analysis, ok := parameters["analysis"].(string)
	if !ok {
		analysis = "mean" // 默认计算平均值
	}

	switch analysis {
	case "mean", "average":
		// 计算时间序列平均值
		if len(timeseriesData.Values) == 0 {
			return 0.0, nil
		}
		sum := 0.0
		for _, val := range timeseriesData.Values {
			sum += val
		}
		return sum / float64(len(timeseriesData.Values)), nil

	case "sum":
		// 计算时间序列总和
		sum := 0.0
		for _, val := range timeseriesData.Values {
			sum += val
		}
		return sum, nil

	case "min":
		// 计算最小值
		if len(timeseriesData.Values) == 0 {
			return 0.0, nil
		}
		min := timeseriesData.Values[0]
		for _, val := range timeseriesData.Values[1:] {
			if val < min {
				min = val
			}
		}
		return min, nil

	case "max":
		// 计算最大值
		if len(timeseriesData.Values) == 0 {
			return 0.0, nil
		}
		max := timeseriesData.Values[0]
		for _, val := range timeseriesData.Values[1:] {
			if val > max {
				max = val
			}
		}
		return max, nil

	case "range":
		// 计算值域
		if len(timeseriesData.Values) == 0 {
			return 0.0, nil
		}
		min, max := timeseriesData.Values[0], timeseriesData.Values[0]
		for _, val := range timeseriesData.Values[1:] {
			if val < min {
				min = val
			}
			if val > max {
				max = val
			}
		}
		return max - min, nil

	case "trend":
		// 计算线性趋势斜率
		return timeseriesData.GetDerivedValues()["trend_slope"], nil

	case "latest", "last":
		// 获取最新值
		if len(timeseriesData.Values) == 0 {
			return 0.0, nil
		}
		return timeseriesData.Values[len(timeseriesData.Values)-1], nil

	case "first":
		// 获取第一个值
		if len(timeseriesData.Values) == 0 {
			return 0.0, nil
		}
		return timeseriesData.Values[0], nil

	case "length", "count":
		// 获取数据点数量
		return len(timeseriesData.Values), nil

	case "variance":
		// 计算方差
		if len(timeseriesData.Values) <= 1 {
			return 0.0, nil
		}
		mean := 0.0
		for _, val := range timeseriesData.Values {
			mean += val
		}
		mean /= float64(len(timeseriesData.Values))
		
		variance := 0.0
		for _, val := range timeseriesData.Values {
			diff := val - mean
			variance += diff * diff
		}
		variance /= float64(len(timeseriesData.Values) - 1)
		return variance, nil

	case "stddev":
		// 计算标准差
		if len(timeseriesData.Values) <= 1 {
			return 0.0, nil
		}
		mean := 0.0
		for _, val := range timeseriesData.Values {
			mean += val
		}
		mean /= float64(len(timeseriesData.Values))
		
		variance := 0.0
		for _, val := range timeseriesData.Values {
			diff := val - mean
			variance += diff * diff
		}
		variance /= float64(len(timeseriesData.Values) - 1)
		return math.Sqrt(variance), nil

	case "trend_analysis":
		// 趋势分析
		return h.performTrendAnalysis(timeseriesData, parameters)

	case "seasonal_decompose":
		// 季节性分解
		return h.performSeasonalDecompose(timeseriesData, parameters)

	case "moving_average":
		// 移动平均
		return h.performMovingAverage(timeseriesData, parameters)

	case "diff":
		// 差分运算
		return h.performDifferencing(timeseriesData, parameters)

	case "resample":
		// 重采样
		return h.performResampling(timeseriesData, parameters)

	case "anomaly_detection":
		// 异常检测
		return h.performAnomalyDetection(timeseriesData, parameters)

	case "forecast":
		// 时序预测
		return h.performForecasting(timeseriesData, parameters)

	case "correlation":
		// 相关性分析
		return h.performCorrelationAnalysis(timeseriesData, parameters)

	default:
		return nil, fmt.Errorf("不支持的时间序列分析类型: %s", analysis)
	}
}

// compositeToArrayTransform 复合数据转换为数组转换
func (h *TransformHandler) compositeToArrayTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	switch data := compositeData.(type) {
	case *model.VectorData:
		// 向量转数组
		values := make([]interface{}, len(data.Values))
		for i, v := range data.Values {
			values[i] = v
		}
		return &model.ArrayData{
			Values:   values,
			DataType: "float",
			Size:     len(values),
			Unit:     data.Unit,
			Labels:   data.Labels,
		}, nil

	case *model.MatrixData:
		// 矩阵转数组（展平）
		values := make([]interface{}, 0, data.Rows*data.Cols)
		for _, row := range data.Values {
			for _, val := range row {
				values = append(values, val)
			}
		}
		return &model.ArrayData{
			Values:   values,
			DataType: "float",
			Size:     len(values),
			Unit:     data.Unit,
		}, nil

	case *model.TimeSeriesData:
		// 时间序列转数组（仅数值）
		values := make([]interface{}, len(data.Values))
		for i, v := range data.Values {
			values[i] = v
		}
		return &model.ArrayData{
			Values:   values,
			DataType: "float",
			Size:     len(values),
			Unit:     data.Unit,
		}, nil

	case *model.LocationData:
		// 地理位置转数组 [lat, lng, alt]
		values := []interface{}{data.Latitude, data.Longitude}
		if data.Altitude != 0 {
			values = append(values, data.Altitude)
		}
		return &model.ArrayData{
			Values:   values,
			DataType: "float",
			Size:     len(values),
			Labels:   []string{"latitude", "longitude", "altitude"},
		}, nil

	case *model.Vector3D:
		// 3D向量转数组
		values := []interface{}{data.X, data.Y, data.Z}
		return &model.ArrayData{
			Values:   values,
			DataType: "float",
			Size:     3,
			Labels:   []string{"x", "y", "z"},
		}, nil

	case *model.ColorData:
		// 颜色转数组 [r, g, b, a]
		values := []interface{}{int(data.R), int(data.G), int(data.B)}
		if data.A != 255 {
			values = append(values, int(data.A))
		}
		return &model.ArrayData{
			Values:   values,
			DataType: "int",
			Size:     len(values),
			Labels:   []string{"red", "green", "blue", "alpha"},
		}, nil

	default:
		return nil, fmt.Errorf("不支持的复合数据类型转换为数组: %T", compositeData)
	}
}

// vectorTransformTransform 向量变换转换
func (h *TransformHandler) vectorTransformTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.VectorData)
	if !ok {
		return nil, fmt.Errorf("数据不是VectorData类型")
	}

	transform, ok := parameters["transform"].(string)
	if !ok {
		transform = "normalize" // 默认归一化
	}

	newValues := make([]float64, len(vectorData.Values))
	copy(newValues, vectorData.Values)

	switch transform {
	case "normalize":
		// 归一化向量
		magnitude := 0.0
		for _, val := range newValues {
			magnitude += val * val
		}
		magnitude = math.Sqrt(magnitude)
		
		if magnitude > 0 {
			for i := range newValues {
				newValues[i] /= magnitude
			}
		}

	case "scale":
		// 缩放向量
		if scaleParam, exists := parameters["scale"]; exists {
			var scale float64
			if s, ok := scaleParam.(float64); ok {
				scale = s
			} else if s, ok := scaleParam.(int); ok {
				scale = float64(s)
			} else {
				return nil, fmt.Errorf("缩放参数必须是数值")
			}
			
			for i := range newValues {
				newValues[i] *= scale
			}
		} else {
			return nil, fmt.Errorf("缺少缩放参数")
		}

	case "abs":
		// 绝对值
		for i := range newValues {
			newValues[i] = math.Abs(newValues[i])
		}

	case "clamp":
		// 限制值域
		var minVal, maxVal float64 = -math.MaxFloat64, math.MaxFloat64
		
		if minParam, exists := parameters["min"]; exists {
			if m, ok := minParam.(float64); ok {
				minVal = m
			} else if m, ok := minParam.(int); ok {
				minVal = float64(m)
			}
		}
		
		if maxParam, exists := parameters["max"]; exists {
			if m, ok := maxParam.(float64); ok {
				maxVal = m
			} else if m, ok := maxParam.(int); ok {
				maxVal = float64(m)
			}
		}
		
		for i := range newValues {
			if newValues[i] < minVal {
				newValues[i] = minVal
			}
			if newValues[i] > maxVal {
				newValues[i] = maxVal
			}
		}

	case "offset":
		// 偏移
		if offsetParam, exists := parameters["offset"]; exists {
			var offset float64
			if o, ok := offsetParam.(float64); ok {
				offset = o
			} else if o, ok := offsetParam.(int); ok {
				offset = float64(o)
			} else {
				return nil, fmt.Errorf("偏移参数必须是数值")
			}
			
			for i := range newValues {
				newValues[i] += offset
			}
		} else {
			return nil, fmt.Errorf("缺少偏移参数")
		}

	default:
		return nil, fmt.Errorf("不支持的向量变换类型: %s", transform)
	}

	// 返回变换后的向量
	return &model.VectorData{
		Values:    newValues,
		Dimension: len(newValues),
		Labels:    vectorData.Labels,
		Unit:      vectorData.Unit,
	}, nil
}

// geoGeofenceTransform 地理围栏检查
func (h *TransformHandler) geoGeofenceTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	locationData, ok := compositeData.(*model.LocationData)
	if !ok {
		return nil, fmt.Errorf("数据不是LocationData类型")
	}

	centerLat, ok := params["center_lat"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少中心点纬度参数")
	}

	centerLng, ok := params["center_lng"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少中心点经度参数")
	}

	radius, ok := params["radius"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少半径参数")
	}

	// 计算距离（使用Haversine公式）
	distance := h.haversineDistance(locationData.Latitude, locationData.Longitude, centerLat, centerLng)
	
	// 检查是否在围栏内
	inFence := distance <= radius
	
	return map[string]interface{}{
		"in_fence": inFence,
		"distance": distance,
		"center_lat": centerLat,
		"center_lng": centerLng,
		"radius": radius,
	}, nil
}

// geoCoordinateConvertTransform 坐标系转换
func (h *TransformHandler) geoCoordinateConvertTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	locationData, ok := compositeData.(*model.LocationData)
	if !ok {
		return nil, fmt.Errorf("数据不是LocationData类型")
	}

	sourceSystem, ok := params["source_coordinate_system"].(string)
	if !ok {
		sourceSystem = "WGS84" // 默认源坐标系
	}

	targetSystem, ok := params["target_coordinate_system"].(string)
	if !ok {
		return nil, fmt.Errorf("缺少目标坐标系参数")
	}

	// 简化的坐标系转换实现（实际项目中应使用专业的坐标转换库）
	var convertedLat, convertedLng float64
	
	switch sourceSystem + "_to_" + targetSystem {
	case "WGS84_to_GCJ02":
		// WGS84 转 GCJ02 (火星坐标系)
		convertedLat, convertedLng = h.wgs84ToGcj02(locationData.Latitude, locationData.Longitude)
	case "GCJ02_to_WGS84":
		// GCJ02 转 WGS84
		convertedLat, convertedLng = h.gcj02ToWgs84(locationData.Latitude, locationData.Longitude)
	case "WGS84_to_BD09":
		// WGS84 转 BD09 (百度坐标系)
		gcjLat, gcjLng := h.wgs84ToGcj02(locationData.Latitude, locationData.Longitude)
		convertedLat, convertedLng = h.gcj02ToBd09(gcjLat, gcjLng)
	case "BD09_to_WGS84":
		// BD09 转 WGS84
		gcjLat, gcjLng := h.bd09ToGcj02(locationData.Latitude, locationData.Longitude)
		convertedLat, convertedLng = h.gcj02ToWgs84(gcjLat, gcjLng)
	default:
		// 同一坐标系或不支持的转换
		convertedLat = locationData.Latitude
		convertedLng = locationData.Longitude
	}

	return &model.LocationData{
		Latitude:  convertedLat,
		Longitude: convertedLng,
		Altitude:  locationData.Altitude,
		Accuracy:  locationData.Accuracy,
	}, nil
}

// vectorProjectionTransform 向量投影
func (h *TransformHandler) vectorProjectionTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.VectorData)
	if !ok {
		return nil, fmt.Errorf("数据不是VectorData类型")
	}

	// 获取参考向量
	refX, ok := params["reference_x"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量X分量")
	}
	refY, ok := params["reference_y"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量Y分量")
	}
	refZ, ok := params["reference_z"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量Z分量")
	}

	if len(vectorData.Values) < 3 {
		return nil, fmt.Errorf("向量维度不足3维")
	}

	// 计算投影: proj = (v·u / |u|²) × u
	vx, vy, vz := vectorData.Values[0], vectorData.Values[1], vectorData.Values[2]
	
	// 点积 v·u
	dotProduct := vx*refX + vy*refY + vz*refZ
	
	// |u|²
	refMagnitudeSquared := refX*refX + refY*refY + refZ*refZ
	
	if refMagnitudeSquared == 0 {
		return nil, fmt.Errorf("参考向量不能为零向量")
	}
	
	// 投影比例
	projectionScale := dotProduct / refMagnitudeSquared
	
	// 投影向量
	projX := projectionScale * refX
	projY := projectionScale * refY
	projZ := projectionScale * refZ

	return &model.VectorData{
		Values:    []float64{projX, projY, projZ},
		Dimension: 3,
		Labels:    vectorData.Labels,
		Unit:      vectorData.Unit,
	}, nil
}

// vectorCrossTransform 向量叉积
func (h *TransformHandler) vectorCrossTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.VectorData)
	if !ok {
		return nil, fmt.Errorf("数据不是VectorData类型")
	}

	// 获取参考向量
	refX, ok := params["reference_x"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量X分量")
	}
	refY, ok := params["reference_y"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量Y分量")
	}
	refZ, ok := params["reference_z"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量Z分量")
	}

	if len(vectorData.Values) < 3 {
		return nil, fmt.Errorf("向量维度不足3维")
	}

	// 计算叉积: v × u = (vy*uz - vz*uy, vz*ux - vx*uz, vx*uy - vy*ux)
	vx, vy, vz := vectorData.Values[0], vectorData.Values[1], vectorData.Values[2]
	
	crossX := vy*refZ - vz*refY
	crossY := vz*refX - vx*refZ
	crossZ := vx*refY - vy*refX

	return &model.VectorData{
		Values:    []float64{crossX, crossY, crossZ},
		Dimension: 3,
		Labels:    vectorData.Labels,
		Unit:      vectorData.Unit,
	}, nil
}

// vectorDotTransform 向量点积
func (h *TransformHandler) vectorDotTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	vectorData, ok := compositeData.(*model.VectorData)
	if !ok {
		return nil, fmt.Errorf("数据不是VectorData类型")
	}

	// 获取参考向量
	refX, ok := params["reference_x"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量X分量")
	}
	refY, ok := params["reference_y"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量Y分量")
	}
	refZ, ok := params["reference_z"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考向量Z分量")
	}

	if len(vectorData.Values) < 3 {
		return nil, fmt.Errorf("向量维度不足3维")
	}

	// 计算点积: v·u = vx*ux + vy*uy + vz*uz
	vx, vy, vz := vectorData.Values[0], vectorData.Values[1], vectorData.Values[2]
	dotProduct := vx*refX + vy*refY + vz*refZ

	return dotProduct, nil
}

// colorSimilarityTransform 颜色相似度计算
func (h *TransformHandler) colorSimilarityTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	colorData, ok := compositeData.(*model.ColorData)
	if !ok {
		return nil, fmt.Errorf("数据不是ColorData类型")
	}

	// 获取参考颜色
	refR, ok := params["reference_r"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考颜色R分量")
	}
	refG, ok := params["reference_g"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考颜色G分量")
	}
	refB, ok := params["reference_b"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少参考颜色B分量")
	}

	// 使用欧几里得距离计算颜色相似度
	distance := math.Sqrt(
		math.Pow(float64(colorData.R)-refR, 2) +
		math.Pow(float64(colorData.G)-refG, 2) +
		math.Pow(float64(colorData.B)-refB, 2),
	)

	// 归一化相似度 (0-1)，距离越小相似度越高
	maxDistance := math.Sqrt(3 * 255 * 255) // RGB最大距离
	similarity := 1.0 - (distance / maxDistance)

	return map[string]interface{}{
		"similarity": similarity,
		"distance":   distance,
		"reference":  map[string]float64{"r": refR, "g": refG, "b": refB},
	}, nil
}

// colorExtractDominantTransform 主色调提取
func (h *TransformHandler) colorExtractDominantTransform(compositeData model.CompositeData, params map[string]interface{}) (interface{}, error) {
	colorData, ok := compositeData.(*model.ColorData)
	if !ok {
		return nil, fmt.Errorf("数据不是ColorData类型")
	}

	// 简化的主色调提取：找到RGB中的最大分量
	r, g, b := float64(colorData.R), float64(colorData.G), float64(colorData.B)
	
	var dominantColor string
	var dominantValue float64
	
	if r >= g && r >= b {
		dominantColor = "red"
		dominantValue = r
	} else if g >= r && g >= b {
		dominantColor = "green"
		dominantValue = g
	} else {
		dominantColor = "blue"
		dominantValue = b
	}

	// 计算颜色强度
	intensity := (r + g + b) / 3
	
	// 计算饱和度
	maxVal := math.Max(r, math.Max(g, b))
	minVal := math.Min(r, math.Min(g, b))
	saturation := 0.0
	if maxVal > 0 {
		saturation = (maxVal - minVal) / maxVal
	}

	return map[string]interface{}{
		"dominant_color": dominantColor,
		"dominant_value": dominantValue,
		"intensity":      intensity,
		"saturation":     saturation,
		"original":       map[string]float64{"r": r, "g": g, "b": b},
	}, nil
}

// 坐标转换辅助函数

// wgs84ToGcj02 WGS84转GCJ02坐标系
func (h *TransformHandler) wgs84ToGcj02(lat, lng float64) (float64, float64) {
	const a = 6378245.0
	const ee = 0.00669342162296594323
	
	dLat := h.transformLat(lng-105.0, lat-35.0)
	dLng := h.transformLng(lng-105.0, lat-35.0)
	
	radLat := lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - ee*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = (dLat * 180.0) / ((a * (1 - ee)) / (magic * sqrtMagic) * math.Pi)
	dLng = (dLng * 180.0) / (a / sqrtMagic * math.Cos(radLat) * math.Pi)
	
	return lat + dLat, lng + dLng
}

// gcj02ToWgs84 GCJ02转WGS84坐标系
func (h *TransformHandler) gcj02ToWgs84(lat, lng float64) (float64, float64) {
	gLat, gLng := h.wgs84ToGcj02(lat, lng)
	return lat*2 - gLat, lng*2 - gLng
}

// gcj02ToBd09 GCJ02转BD09坐标系
func (h *TransformHandler) gcj02ToBd09(lat, lng float64) (float64, float64) {
	const x_pi = 3.14159265358979324 * 3000.0 / 180.0
	z := math.Sqrt(lng*lng+lat*lat) + 0.00002*math.Sin(lat*x_pi)
	theta := math.Atan2(lat, lng) + 0.000003*math.Cos(lng*x_pi)
	return z*math.Sin(theta) + 0.006, z*math.Cos(theta) + 0.0065
}

// bd09ToGcj02 BD09转GCJ02坐标系
func (h *TransformHandler) bd09ToGcj02(lat, lng float64) (float64, float64) {
	const x_pi = 3.14159265358979324 * 3000.0 / 180.0
	x := lng - 0.0065
	y := lat - 0.006
	z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*x_pi)
	theta := math.Atan2(y, x) - 0.000003*math.Cos(x*x_pi)
	return z*math.Sin(theta), z*math.Cos(theta)
}

// transformLat 纬度转换辅助函数
func (h *TransformHandler) transformLat(lng, lat float64) float64 {
	ret := -100.0 + 2.0*lng + 3.0*lat + 0.2*lat*lat + 0.1*lng*lat + 0.2*math.Sqrt(math.Abs(lng))
	ret += (20.0*math.Sin(6.0*lng*math.Pi) + 20.0*math.Sin(2.0*lng*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(lat*math.Pi) + 40.0*math.Sin(lat/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(lat/12.0*math.Pi) + 320*math.Sin(lat*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

// transformLng 经度转换辅助函数
func (h *TransformHandler) transformLng(lng, lat float64) float64 {
	ret := 300.0 + lng + 2.0*lat + 0.1*lng*lng + 0.1*lng*lat + 0.1*math.Sqrt(math.Abs(lng))
	ret += (20.0*math.Sin(6.0*lng*math.Pi) + 20.0*math.Sin(2.0*lng*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(lng*math.Pi) + 40.0*math.Sin(lng/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(lng/12.0*math.Pi) + 300.0*math.Sin(lng/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}

// 时间序列分析算法实现

// performTrendAnalysis 趋势分析
func (h *TransformHandler) performTrendAnalysis(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	if len(timeseriesData.Values) < 2 {
		return nil, fmt.Errorf("数据点不足，无法进行趋势分析")
	}

	trendMethod, ok := parameters["trend_method"].(string)
	if !ok {
		trendMethod = "linear"
	}

	windowSize, ok := parameters["window_size"].(float64)
	if !ok {
		windowSize = 10
	}
	window := int(windowSize)

	switch trendMethod {
	case "linear":
		// 线性趋势分析
		slope, intercept := h.calculateLinearTrend(timeseriesData.Values)
		return map[string]interface{}{
			"trend_direction": h.getTrendDirection(slope),
			"slope":          slope,
			"intercept":      intercept,
			"strength":       math.Abs(slope),
		}, nil

	case "polynomial":
		// 简化的多项式趋势分析
		return h.calculatePolynomialTrend(timeseriesData.Values, window)

	case "exponential":
		// 指数趋势分析
		return h.calculateExponentialTrend(timeseriesData.Values)

	case "seasonal":
		// 季节性趋势分析
		return h.calculateSeasonalTrend(timeseriesData.Values, window)

	default:
		return nil, fmt.Errorf("不支持的趋势分析方法: %s", trendMethod)
	}
}

// performSeasonalDecompose 季节性分解
func (h *TransformHandler) performSeasonalDecompose(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	if len(timeseriesData.Values) < 4 {
		return nil, fmt.Errorf("数据点不足，无法进行季节性分解")
	}

	seasonalPeriod, ok := parameters["seasonal_period"].(float64)
	if !ok {
		seasonalPeriod = 12
	}
	period := int(seasonalPeriod)

	decomposeModel, ok := parameters["decompose_model"].(string)
	if !ok {
		decomposeModel = "additive"
	}

	switch decomposeModel {
	case "additive":
		// 加法模型: Y = Trend + Seasonal + Random
		return h.performAdditiveDecomposition(timeseriesData.Values, period)

	case "multiplicative":
		// 乘法模型: Y = Trend × Seasonal × Random
		return h.performMultiplicativeDecomposition(timeseriesData.Values, period)

	default:
		return nil, fmt.Errorf("不支持的分解模型: %s", decomposeModel)
	}
}

// performMovingAverage 移动平均
func (h *TransformHandler) performMovingAverage(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	windowSize, ok := parameters["window_size"].(float64)
	if !ok {
		windowSize = 5
	}
	window := int(windowSize)

	windowType, ok := parameters["window_type"].(string)
	if !ok {
		windowType = "sliding"
	}

	if window <= 0 || window > len(timeseriesData.Values) {
		return nil, fmt.Errorf("窗口大小无效: %d", window)
	}

	switch windowType {
	case "sliding":
		return h.calculateSlidingAverage(timeseriesData.Values, window)
	case "expanding":
		return h.calculateExpandingAverage(timeseriesData.Values)
	case "fixed":
		return h.calculateFixedWindowAverage(timeseriesData.Values, window)
	default:
		return nil, fmt.Errorf("不支持的窗口类型: %s", windowType)
	}
}

// performDifferencing 差分运算
func (h *TransformHandler) performDifferencing(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	if len(timeseriesData.Values) < 2 {
		return nil, fmt.Errorf("数据点不足，无法进行差分")
	}

	diffOrder, ok := parameters["diff_order"].(float64)
	if !ok {
		diffOrder = 1
	}
	order := int(diffOrder)

	diffSeasonal, ok := parameters["diff_seasonal"].(bool)
	if !ok {
		diffSeasonal = false
	}

	result := make([]float64, len(timeseriesData.Values))
	copy(result, timeseriesData.Values)

	// 普通差分
	for i := 0; i < order; i++ {
		result = h.calculateFirstDifference(result)
	}

	// 季节性差分
	if diffSeasonal {
		seasonalPeriod, ok := parameters["seasonal_period"].(float64)
		if !ok {
			seasonalPeriod = 12
		}
		result = h.calculateSeasonalDifference(result, int(seasonalPeriod))
	}

	return &model.TimeSeriesData{
		Values: result,
		Unit:   timeseriesData.Unit,
	}, nil
}

// performResampling 重采样
func (h *TransformHandler) performResampling(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	resampleFrequency, ok := parameters["resample_frequency"].(string)
	if !ok {
		resampleFrequency = "hour"
	}

	resampleMethod, ok := parameters["resample_method"].(string)
	if !ok {
		resampleMethod = "mean"
	}

	// 根据重采样频率和方法处理数据
	switch resampleFrequency {
	case "minute":
		return h.resampleToMinute(timeseriesData.Values, resampleMethod)
	case "hour":
		return h.resampleToHour(timeseriesData.Values, resampleMethod)
	case "day":
		return h.resampleToDay(timeseriesData.Values, resampleMethod)
	case "week":
		return h.resampleToWeek(timeseriesData.Values, resampleMethod)
	case "month":
		return h.resampleToMonth(timeseriesData.Values, resampleMethod)
	default:
		return nil, fmt.Errorf("不支持的重采样频率: %s", resampleFrequency)
	}
}

// performAnomalyDetection 异常检测
func (h *TransformHandler) performAnomalyDetection(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	anomalyMethod, ok := parameters["anomaly_method"].(string)
	if !ok {
		anomalyMethod = "zscore"
	}

	anomalyThreshold, ok := parameters["anomaly_threshold"].(float64)
	if !ok {
		anomalyThreshold = 2.5
	}

	switch anomalyMethod {
	case "zscore":
		return h.detectAnomaliesZScore(timeseriesData.Values, anomalyThreshold)
	case "iqr":
		return h.detectAnomaliesIQR(timeseriesData.Values)
	case "isolation_forest":
		return h.detectAnomaliesIsolationForest(timeseriesData.Values)
	case "local_outlier":
		return h.detectAnomaliesLocalOutlier(timeseriesData.Values, anomalyThreshold)
	default:
		return nil, fmt.Errorf("不支持的异常检测方法: %s", anomalyMethod)
	}
}

// performForecasting 时序预测
func (h *TransformHandler) performForecasting(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	if len(timeseriesData.Values) < 3 {
		return nil, fmt.Errorf("数据点不足，无法进行预测")
	}

	forecastSteps, ok := parameters["forecast_steps"].(float64)
	if !ok {
		forecastSteps = 5
	}
	steps := int(forecastSteps)

	forecastMethod, ok := parameters["forecast_method"].(string)
	if !ok {
		forecastMethod = "linear"
	}

	switch forecastMethod {
	case "linear":
		return h.forecastLinear(timeseriesData.Values, steps)
	case "exponential_smoothing":
		return h.forecastExponentialSmoothing(timeseriesData.Values, steps)
	case "seasonal_naive":
		return h.forecastSeasonalNaive(timeseriesData.Values, steps)
	case "arima":
		return h.forecastARIMA(timeseriesData.Values, steps)
	default:
		return nil, fmt.Errorf("不支持的预测方法: %s", forecastMethod)
	}
}

// performCorrelationAnalysis 相关性分析
func (h *TransformHandler) performCorrelationAnalysis(timeseriesData *model.TimeSeriesData, parameters map[string]interface{}) (interface{}, error) {
	correlationLag, ok := parameters["correlation_lag"].(float64)
	if !ok {
		correlationLag = 1
	}
	lag := int(correlationLag)

	if lag >= len(timeseriesData.Values) {
		return nil, fmt.Errorf("滞后期数过大: %d", lag)
	}

	// 计算自相关
	autocorrelation := h.calculateAutocorrelation(timeseriesData.Values, lag)

	return map[string]interface{}{
		"autocorrelation": autocorrelation,
		"lag":            lag,
		"significance":   h.getCorrelationSignificance(autocorrelation),
	}, nil
}

// 趋势分析辅助函数
func (h *TransformHandler) calculateLinearTrend(values []float64) (slope, intercept float64) {
	n := float64(len(values))
	if n < 2 {
		return 0, 0
	}

	sumX, sumY, sumXY, sumXX := 0.0, 0.0, 0.0, 0.0
	for i, y := range values {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}

	slope = (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	intercept = (sumY - slope*sumX) / n
	return slope, intercept
}

func (h *TransformHandler) getTrendDirection(slope float64) string {
	if slope > 0.001 {
		return "上升"
	} else if slope < -0.001 {
		return "下降"
	}
	return "稳定"
}

func (h *TransformHandler) calculatePolynomialTrend(values []float64, window int) (interface{}, error) {
	// 简化的多项式趋势分析
	if window > len(values) {
		window = len(values)
	}
	
	recentValues := values[len(values)-window:]
	slope, intercept := h.calculateLinearTrend(recentValues)
	
	return map[string]interface{}{
		"trend_direction": h.getTrendDirection(slope),
		"slope":          slope,
		"intercept":      intercept,
		"window_size":    window,
	}, nil
}

func (h *TransformHandler) calculateExponentialTrend(values []float64) (interface{}, error) {
	// 简化的指数趋势分析
	if len(values) < 2 {
		return nil, fmt.Errorf("数据点不足")
	}
	
	// 计算指数增长率
	growthRate := (values[len(values)-1] - values[0]) / values[0]
	
	return map[string]interface{}{
		"growth_rate":     growthRate,
		"trend_direction": h.getTrendDirection(growthRate),
		"exponential":     true,
	}, nil
}

func (h *TransformHandler) calculateSeasonalTrend(values []float64, period int) (interface{}, error) {
	if len(values) < period*2 {
		return nil, fmt.Errorf("数据点不足，需要至少%d个点", period*2)
	}
	
	// 简化的季节性趋势分析
	seasonalMeans := make([]float64, period)
	for i := 0; i < period; i++ {
		sum := 0.0
		count := 0
		for j := i; j < len(values); j += period {
			sum += values[j]
			count++
		}
		if count > 0 {
			seasonalMeans[i] = sum / float64(count)
		}
	}
	
	return map[string]interface{}{
		"seasonal_means": seasonalMeans,
		"period":        period,
		"seasonal":      true,
	}, nil
}

// 季节性分解辅助函数
func (h *TransformHandler) performAdditiveDecomposition(values []float64, period int) (interface{}, error) {
	n := len(values)
	if n < period*2 {
		return nil, fmt.Errorf("数据点不足进行季节性分解")
	}

	// 计算趋势分量（使用移动平均）
	trend := h.calculateMovingAverageForDecomposition(values, period)
	
	// 计算季节性分量
	seasonal := h.calculateSeasonalComponent(values, trend, period, "additive")
	
	// 计算随机分量
	random := make([]float64, n)
	for i := 0; i < n; i++ {
		random[i] = values[i] - trend[i] - seasonal[i%period]
	}

	return map[string]interface{}{
		"trend":    trend,
		"seasonal": seasonal,
		"random":   random,
		"model":    "additive",
	}, nil
}

func (h *TransformHandler) performMultiplicativeDecomposition(values []float64, period int) (interface{}, error) {
	n := len(values)
	if n < period*2 {
		return nil, fmt.Errorf("数据点不足进行季节性分解")
	}

	// 计算趋势分量
	trend := h.calculateMovingAverageForDecomposition(values, period)
	
	// 计算季节性分量
	seasonal := h.calculateSeasonalComponent(values, trend, period, "multiplicative")
	
	// 计算随机分量
	random := make([]float64, n)
	for i := 0; i < n; i++ {
		if trend[i] != 0 && seasonal[i%period] != 0 {
			random[i] = values[i] / (trend[i] * seasonal[i%period])
		} else {
			random[i] = 1.0
		}
	}

	return map[string]interface{}{
		"trend":    trend,
		"seasonal": seasonal,
		"random":   random,
		"model":    "multiplicative",
	}, nil
}

// 移动平均辅助函数
func (h *TransformHandler) calculateSlidingAverage(values []float64, window int) (interface{}, error) {
	n := len(values)
	result := make([]float64, n-window+1)
	
	for i := 0; i <= n-window; i++ {
		sum := 0.0
		for j := i; j < i+window; j++ {
			sum += values[j]
		}
		result[i] = sum / float64(window)
	}
	
	return &model.TimeSeriesData{
		Values: result,
	}, nil
}

func (h *TransformHandler) calculateExpandingAverage(values []float64) (interface{}, error) {
	n := len(values)
	result := make([]float64, n)
	sum := 0.0
	
	for i := 0; i < n; i++ {
		sum += values[i]
		result[i] = sum / float64(i+1)
	}
	
	return &model.TimeSeriesData{
		Values: result,
	}, nil
}

func (h *TransformHandler) calculateFixedWindowAverage(values []float64, window int) (interface{}, error) {
	n := len(values)
	result := make([]float64, 0)
	
	for i := 0; i < n; i += window {
		end := i + window
		if end > n {
			end = n
		}
		
		sum := 0.0
		for j := i; j < end; j++ {
			sum += values[j]
		}
		result = append(result, sum/float64(end-i))
	}
	
	return &model.TimeSeriesData{
		Values: result,
	}, nil
}

// 差分辅助函数
func (h *TransformHandler) calculateFirstDifference(values []float64) []float64 {
	if len(values) <= 1 {
		return []float64{}
	}
	
	result := make([]float64, len(values)-1)
	for i := 1; i < len(values); i++ {
		result[i-1] = values[i] - values[i-1]
	}
	return result
}

func (h *TransformHandler) calculateSeasonalDifference(values []float64, period int) []float64 {
	if len(values) <= period {
		return []float64{}
	}
	
	result := make([]float64, len(values)-period)
	for i := period; i < len(values); i++ {
		result[i-period] = values[i] - values[i-period]
	}
	return result
}

// 重采样辅助函数
func (h *TransformHandler) resampleToHour(values []float64, method string) (interface{}, error) {
	// 简化实现：假设原始数据是分钟级别，重采样到小时级别
	hourlyValues := make([]float64, 0)
	windowSize := 60 // 60分钟
	
	for i := 0; i < len(values); i += windowSize {
		end := i + windowSize
		if end > len(values) {
			end = len(values)
		}
		
		var aggregated float64
		switch method {
		case "mean":
			sum := 0.0
			for j := i; j < end; j++ {
				sum += values[j]
			}
			aggregated = sum / float64(end-i)
		case "sum":
			for j := i; j < end; j++ {
				aggregated += values[j]
			}
		case "max":
			aggregated = values[i]
			for j := i; j < end; j++ {
				if values[j] > aggregated {
					aggregated = values[j]
				}
			}
		case "min":
			aggregated = values[i]
			for j := i; j < end; j++ {
				if values[j] < aggregated {
					aggregated = values[j]
				}
			}
		case "first":
			aggregated = values[i]
		case "last":
			aggregated = values[end-1]
		default:
			return nil, fmt.Errorf("不支持的重采样方法: %s", method)
		}
		hourlyValues = append(hourlyValues, aggregated)
	}
	
	return &model.TimeSeriesData{
		Values: hourlyValues,
	}, nil
}

// 占位符实现（为了编译通过）
func (h *TransformHandler) resampleToMinute(values []float64, method string) (interface{}, error) {
	return h.resampleToHour(values, method) // 简化实现
}

func (h *TransformHandler) resampleToDay(values []float64, method string) (interface{}, error) {
	return h.resampleToHour(values, method) // 简化实现
}

func (h *TransformHandler) resampleToWeek(values []float64, method string) (interface{}, error) {
	return h.resampleToHour(values, method) // 简化实现
}

func (h *TransformHandler) resampleToMonth(values []float64, method string) (interface{}, error) {
	return h.resampleToHour(values, method) // 简化实现
}

// 异常检测辅助函数
func (h *TransformHandler) detectAnomaliesZScore(values []float64, threshold float64) (interface{}, error) {
	n := len(values)
	if n < 2 {
		return nil, fmt.Errorf("数据点不足")
	}

	// 计算均值和标准差
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(n-1))

	// 检测异常点
	anomalies := make([]map[string]interface{}, 0)
	for i, v := range values {
		zscore := math.Abs(v-mean) / stddev
		if zscore > threshold {
			anomalies = append(anomalies, map[string]interface{}{
				"index":  i,
				"value":  v,
				"zscore": zscore,
			})
		}
	}

	return map[string]interface{}{
		"anomalies":     anomalies,
		"total_points":  n,
		"anomaly_count": len(anomalies),
		"threshold":     threshold,
		"method":        "zscore",
	}, nil
}

// 占位符实现（简化版本）
func (h *TransformHandler) detectAnomaliesIQR(values []float64) (interface{}, error) {
	return h.detectAnomaliesZScore(values, 2.5) // 简化实现
}

func (h *TransformHandler) detectAnomaliesIsolationForest(values []float64) (interface{}, error) {
	return h.detectAnomaliesZScore(values, 3.0) // 简化实现
}

func (h *TransformHandler) detectAnomaliesLocalOutlier(values []float64, threshold float64) (interface{}, error) {
	return h.detectAnomaliesZScore(values, threshold) // 简化实现
}

// 预测辅助函数
func (h *TransformHandler) forecastLinear(values []float64, steps int) (interface{}, error) {
	slope, intercept := h.calculateLinearTrend(values)
	
	forecast := make([]float64, steps)
	n := float64(len(values))
	
	for i := 0; i < steps; i++ {
		forecast[i] = slope*(n+float64(i)) + intercept
	}
	
	return map[string]interface{}{
		"forecast": forecast,
		"method":   "linear",
		"steps":    steps,
	}, nil
}

func (h *TransformHandler) forecastExponentialSmoothing(values []float64, steps int) (interface{}, error) {
	if len(values) < 2 {
		return nil, fmt.Errorf("数据点不足")
	}
	
	alpha := 0.3 // 平滑参数
	forecast := make([]float64, steps)
	
	// 初始值
	smoothed := values[0]
	for i := 1; i < len(values); i++ {
		smoothed = alpha*values[i] + (1-alpha)*smoothed
	}
	
	// 预测
	for i := 0; i < steps; i++ {
		forecast[i] = smoothed
	}
	
	return map[string]interface{}{
		"forecast": forecast,
		"method":   "exponential_smoothing",
		"alpha":    alpha,
	}, nil
}

func (h *TransformHandler) forecastSeasonalNaive(values []float64, steps int) (interface{}, error) {
	seasonalPeriod := 12 // 默认周期
	if len(values) < seasonalPeriod {
		seasonalPeriod = len(values)
	}
	
	forecast := make([]float64, steps)
	for i := 0; i < steps; i++ {
		idx := len(values) - seasonalPeriod + (i % seasonalPeriod)
		if idx >= 0 && idx < len(values) {
			forecast[i] = values[idx]
		}
	}
	
	return map[string]interface{}{
		"forecast": forecast,
		"method":   "seasonal_naive",
		"period":   seasonalPeriod,
	}, nil
}

func (h *TransformHandler) forecastARIMA(values []float64, steps int) (interface{}, error) {
	// 简化的ARIMA实现（实际应使用专业库）
	return h.forecastLinear(values, steps) // 简化为线性预测
}

// 相关性分析辅助函数
func (h *TransformHandler) calculateAutocorrelation(values []float64, lag int) float64 {
	n := len(values)
	if lag >= n || n < 2 {
		return 0
	}

	// 计算均值
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(n)

	// 计算自相关系数
	numerator := 0.0
	denominator := 0.0

	for i := 0; i < n-lag; i++ {
		numerator += (values[i] - mean) * (values[i+lag] - mean)
	}

	for _, v := range values {
		denominator += (v - mean) * (v - mean)
	}

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

func (h *TransformHandler) getCorrelationSignificance(correlation float64) string {
	abs_corr := math.Abs(correlation)
	if abs_corr > 0.7 {
		return "强相关"
	} else if abs_corr > 0.3 {
		return "中等相关"
	} else if abs_corr > 0.1 {
		return "弱相关"
	}
	return "无相关"
}

// 季节性分解辅助函数
func (h *TransformHandler) calculateMovingAverageForDecomposition(values []float64, period int) []float64 {
	n := len(values)
	trend := make([]float64, n)
	
	for i := 0; i < n; i++ {
		if i < period/2 || i >= n-period/2 {
			trend[i] = values[i] // 边界值直接使用原值
		} else {
			sum := 0.0
			for j := i - period/2; j <= i + period/2; j++ {
				sum += values[j]
			}
			trend[i] = sum / float64(period)
		}
	}
	
	return trend
}

func (h *TransformHandler) calculateSeasonalComponent(values, trend []float64, period int, model string) []float64 {
	seasonal := make([]float64, period)
	counts := make([]int, period)
	
	for i := 0; i < len(values); i++ {
		seasonIndex := i % period
		if model == "additive" {
			seasonal[seasonIndex] += values[i] - trend[i]
		} else { // multiplicative
			if trend[i] != 0 {
				seasonal[seasonIndex] += values[i] / trend[i]
			}
		}
		counts[seasonIndex]++
	}
	
	// 平均化季节性分量
	for i := 0; i < period; i++ {
		if counts[i] > 0 {
			seasonal[i] /= float64(counts[i])
		}
	}
	
	// 标准化季节性分量
	if model == "additive" {
		// 加法模型：确保季节性分量和为0
		mean := 0.0
		for _, s := range seasonal {
			mean += s
		}
		mean /= float64(period)
		for i := range seasonal {
			seasonal[i] -= mean
		}
	} else {
		// 乘法模型：确保季节性分量乘积为1
		product := 1.0
		for _, s := range seasonal {
			if s != 0 {
				product *= s
			}
		}
		adjustment := math.Pow(product, -1.0/float64(period))
		for i := range seasonal {
			seasonal[i] *= adjustment
		}
	}
	
	return seasonal
}

// arrayFilterTransform 数组过滤操作转换
func (h *TransformHandler) arrayFilterTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	filterType, ok := parameters["filter_type"].(string)
	if !ok {
		filterType = "value_range" // 默认数值范围过滤
	}

	switch filterType {
	case "value_range":
		// 数值范围过滤
		minVal, hasMin := parameters["min_value"]
		maxVal, hasMax := parameters["max_value"]
		
		var min, max float64
		if hasMin {
			if minFloat, ok := minVal.(float64); ok {
				min = minFloat
			} else if minInt, ok := minVal.(int); ok {
				min = float64(minInt)
			}
		}
		if hasMax {
			if maxFloat, ok := maxVal.(float64); ok {
				max = maxFloat
			} else if maxInt, ok := maxVal.(int); ok {
				max = float64(maxInt)
			}
		}

		filteredValues := make([]interface{}, 0)
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				keep := true
				if hasMin && num < min {
					keep = false
				}
				if hasMax && num > max {
					keep = false
				}
				if keep {
					filteredValues = append(filteredValues, val)
				}
			}
		}
		
		return &model.ArrayData{
			Values:   filteredValues,
			DataType: arrayData.DataType,
			Size:     len(filteredValues),
			Unit:     arrayData.Unit,
			Labels:   arrayData.Labels,
		}, nil

	case "outliers":
		// 异常值过滤
		method, ok := parameters["outlier_method"].(string)
		if !ok {
			method = "zscore"
		}
		
		threshold := 3.0
		if t, exists := parameters["outlier_threshold"]; exists {
			if tFloat, ok := t.(float64); ok {
				threshold = tFloat
			}
		}

		// 转换为数值数组
		numericValues := make([]float64, 0)
		valueMap := make(map[int]interface{})
		
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				numericValues = append(numericValues, num)
				valueMap[len(numericValues)-1] = val
			}
		}

		if len(numericValues) == 0 {
			return arrayData, nil
		}

		// 根据方法检测异常值
		var isOutlier []bool
		
		switch method {
		case "zscore":
			isOutlier = h.detectOutliersZScore(numericValues, threshold)
		case "iqr":
			isOutlier = h.detectOutliersIQR(numericValues)
		case "percentile":
			isOutlier = h.detectOutliersPercentile(numericValues, threshold)
		default:
			isOutlier = h.detectOutliersZScore(numericValues, threshold)
		}

		// 过滤异常值
		filteredValues := make([]interface{}, 0)
		for i := 0; i < len(numericValues); i++ {
			if !isOutlier[i] {
				filteredValues = append(filteredValues, valueMap[i])
			}
		}

		return &model.ArrayData{
			Values:   filteredValues,
			DataType: arrayData.DataType,
			Size:     len(filteredValues),
			Unit:     arrayData.Unit,
			Labels:   arrayData.Labels,
		}, nil

	case "expression":
		// 表达式过滤
		condition, ok := parameters["filter_condition"].(string)
		if !ok {
			return nil, fmt.Errorf("缺少过滤表达式")
		}

		filteredValues := make([]interface{}, 0)
		for _, val := range arrayData.Values {
			if num, err := h.toFloat64(val); err == nil {
				// 简化的表达式求值（实际应该使用表达式引擎）
				if h.evaluateFilterExpression(condition, num) {
					filteredValues = append(filteredValues, val)
				}
			}
		}

		return &model.ArrayData{
			Values:   filteredValues,
			DataType: arrayData.DataType,
			Size:     len(filteredValues),
			Unit:     arrayData.Unit,
			Labels:   arrayData.Labels,
		}, nil

	default:
		return nil, fmt.Errorf("不支持的过滤类型: %s", filterType)
	}
}

// arraySortTransform 数组排序操作转换
func (h *TransformHandler) arraySortTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	sortBy, ok := parameters["sort_by"].(string)
	if !ok {
		sortBy = "value"
	}

	sortOrder, ok := parameters["sort_order"].(string)
	if !ok {
		sortOrder = "asc"
	}

	// 创建索引-值对
	type IndexValue struct {
		Index int
		Value interface{}
		NumVal float64
		AbsVal float64
	}

	pairs := make([]IndexValue, len(arrayData.Values))
	for i, val := range arrayData.Values {
		numVal, _ := h.toFloat64(val)
		absVal := math.Abs(numVal)
		pairs[i] = IndexValue{
			Index: i,
			Value: val,
			NumVal: numVal,
			AbsVal: absVal,
		}
	}

	// 排序
	sort.Slice(pairs, func(i, j int) bool {
		var compareVal bool
		
		switch sortBy {
		case "value":
			compareVal = pairs[i].NumVal < pairs[j].NumVal
		case "abs_value":
			compareVal = pairs[i].AbsVal < pairs[j].AbsVal
		case "index":
			compareVal = pairs[i].Index < pairs[j].Index
		default:
			compareVal = pairs[i].NumVal < pairs[j].NumVal
		}

		if sortOrder == "desc" {
			return !compareVal
		}
		return compareVal
	})

	// 提取排序后的值
	sortedValues := make([]interface{}, len(pairs))
	for i, pair := range pairs {
		sortedValues[i] = pair.Value
	}

	return &model.ArrayData{
		Values:   sortedValues,
		DataType: arrayData.DataType,
		Size:     len(sortedValues),
		Unit:     arrayData.Unit,
		Labels:   arrayData.Labels,
	}, nil
}

// arraySliceTransform 数组切片操作转换
func (h *TransformHandler) arraySliceTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	start := 0
	if s, exists := parameters["slice_start"]; exists {
		if startInt, ok := s.(int); ok {
			start = startInt
		} else if startFloat, ok := s.(float64); ok {
			start = int(startFloat)
		}
	}

	end := len(arrayData.Values)
	if e, exists := parameters["slice_end"]; exists {
		if endInt, ok := e.(int); ok {
			end = endInt
		} else if endFloat, ok := e.(float64); ok {
			end = int(endFloat)
		}
	}

	step := 1
	if st, exists := parameters["slice_step"]; exists {
		if stepInt, ok := st.(int); ok {
			step = stepInt
		} else if stepFloat, ok := st.(float64); ok {
			step = int(stepFloat)
		}
	}

	// 边界检查
	if start < 0 {
		start = 0
	}
	if end > len(arrayData.Values) {
		end = len(arrayData.Values)
	}
	if start >= end || step <= 0 {
		return &model.ArrayData{
			Values:   []interface{}{},
			DataType: arrayData.DataType,
			Size:     0,
			Unit:     arrayData.Unit,
			Labels:   arrayData.Labels,
		}, nil
	}

	// 执行切片
	slicedValues := make([]interface{}, 0, (end-start+step-1)/step)
	for i := start; i < end; i += step {
		slicedValues = append(slicedValues, arrayData.Values[i])
	}

	return &model.ArrayData{
		Values:   slicedValues,
		DataType: arrayData.DataType,
		Size:     len(slicedValues),
		Unit:     arrayData.Unit,
		Labels:   arrayData.Labels,
	}, nil
}

// arraySmoothTransform 数组平滑操作转换
func (h *TransformHandler) arraySmoothTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	method, ok := parameters["smooth_method"].(string)
	if !ok {
		method = "moving_average"
	}

	window := 5
	if w, exists := parameters["smooth_window"]; exists {
		if windowInt, ok := w.(int); ok {
			window = windowInt
		} else if windowFloat, ok := w.(float64); ok {
			window = int(windowFloat)
		}
	}

	// 转换为数值数组
	numericValues := make([]float64, 0, len(arrayData.Values))
	for _, val := range arrayData.Values {
		if num, err := h.toFloat64(val); err == nil {
			numericValues = append(numericValues, num)
		}
	}

	if len(numericValues) == 0 {
		return arrayData, nil
	}

	var smoothedValues []float64

	switch method {
	case "moving_average":
		smoothedValues = h.movingAverage(numericValues, window)
	case "gaussian":
		smoothedValues = h.gaussianSmooth(numericValues, window)
	case "savgol":
		smoothedValues = h.savgolSmooth(numericValues, window)
	default:
		smoothedValues = h.movingAverage(numericValues, window)
	}

	// 转换回interface{}类型
	resultValues := make([]interface{}, len(smoothedValues))
	for i, val := range smoothedValues {
		resultValues[i] = val
	}

	return &model.ArrayData{
		Values:   resultValues,
		DataType: arrayData.DataType,
		Size:     len(resultValues),
		Unit:     arrayData.Unit,
		Labels:   arrayData.Labels,
	}, nil
}

// arrayNormalizeTransform 数组归一化操作转换
func (h *TransformHandler) arrayNormalizeTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	method, ok := parameters["normalize_method"].(string)
	if !ok {
		method = "minmax"
	}

	// 转换为数值数组
	numericValues := make([]float64, 0, len(arrayData.Values))
	for _, val := range arrayData.Values {
		if num, err := h.toFloat64(val); err == nil {
			numericValues = append(numericValues, num)
		}
	}

	if len(numericValues) == 0 {
		return arrayData, nil
	}

	var normalizedValues []float64

	switch method {
	case "minmax":
		normalizedValues = h.minMaxNormalize(numericValues)
	case "zscore":
		normalizedValues = h.zScoreNormalize(numericValues)
	case "robust":
		normalizedValues = h.robustNormalize(numericValues)
	default:
		normalizedValues = h.minMaxNormalize(numericValues)
	}

	// 转换回interface{}类型
	resultValues := make([]interface{}, len(normalizedValues))
	for i, val := range normalizedValues {
		resultValues[i] = val
	}

	return &model.ArrayData{
		Values:   resultValues,
		DataType: arrayData.DataType,
		Size:     len(resultValues),
		Unit:     arrayData.Unit,
		Labels:   arrayData.Labels,
	}, nil
}

// arrayTransformTransform 数组变换操作转换
func (h *TransformHandler) arrayTransformTransform(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, error) {
	arrayData, ok := compositeData.(*model.ArrayData)
	if !ok {
		return nil, fmt.Errorf("数据不是ArrayData类型")
	}

	transformType, ok := parameters["transform_type"].(string)
	if !ok {
		transformType = "log" // 默认对数变换
	}

	// 转换为数值数组
	numericValues := make([]float64, 0, len(arrayData.Values))
	for _, val := range arrayData.Values {
		if num, err := h.toFloat64(val); err == nil {
			numericValues = append(numericValues, num)
		}
	}

	if len(numericValues) == 0 {
		return arrayData, nil
	}

	var transformedValues []float64

	switch transformType {
	case "log":
		transformedValues = make([]float64, len(numericValues))
		for i, val := range numericValues {
			if val > 0 {
				transformedValues[i] = math.Log(val)
			} else {
				transformedValues[i] = math.Log(1e-10) // 避免log(0)
			}
		}
	case "sqrt":
		transformedValues = make([]float64, len(numericValues))
		for i, val := range numericValues {
			if val >= 0 {
				transformedValues[i] = math.Sqrt(val)
			} else {
				transformedValues[i] = 0
			}
		}
	case "square":
		transformedValues = make([]float64, len(numericValues))
		for i, val := range numericValues {
			transformedValues[i] = val * val
		}
	case "exp":
		transformedValues = make([]float64, len(numericValues))
		for i, val := range numericValues {
			transformedValues[i] = math.Exp(val)
		}
	case "abs":
		transformedValues = make([]float64, len(numericValues))
		for i, val := range numericValues {
			transformedValues[i] = math.Abs(val)
		}
	default:
		transformedValues = numericValues
	}

	// 转换回interface{}类型
	resultValues := make([]interface{}, len(transformedValues))
	for i, val := range transformedValues {
		resultValues[i] = val
	}

	return &model.ArrayData{
		Values:   resultValues,
		DataType: arrayData.DataType,
		Size:     len(resultValues),
		Unit:     arrayData.Unit,
		Labels:   arrayData.Labels,
	}, nil
}

// 辅助函数：异常值检测
func (h *TransformHandler) detectOutliersZScore(values []float64, threshold float64) []bool {
	if len(values) < 2 {
		return make([]bool, len(values))
	}

	// 计算均值和标准差
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values) - 1)
	stddev := math.Sqrt(variance)

	if stddev == 0 {
		return make([]bool, len(values))
	}

	// 检测异常值
	isOutlier := make([]bool, len(values))
	for i, v := range values {
		zscore := math.Abs(v - mean) / stddev
		isOutlier[i] = zscore > threshold
	}

	return isOutlier
}

func (h *TransformHandler) detectOutliersIQR(values []float64) []bool {
	if len(values) < 4 {
		return make([]bool, len(values))
	}

	// 复制并排序
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// 计算四分位数
	n := len(sorted)
	q1 := sorted[n/4]
	q3 := sorted[3*n/4]
	iqr := q3 - q1

	// 计算异常值边界
	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr

	// 检测异常值
	isOutlier := make([]bool, len(values))
	for i, v := range values {
		isOutlier[i] = v < lowerBound || v > upperBound
	}

	return isOutlier
}

func (h *TransformHandler) detectOutliersPercentile(values []float64, threshold float64) []bool {
	if len(values) < 4 {
		return make([]bool, len(values))
	}

	// 复制并排序
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// 计算百分位数边界
	n := len(sorted)
	lowerIndex := int((100-threshold*100)/200 * float64(n))
	upperIndex := int((100+threshold*100)/200 * float64(n))
	
	if lowerIndex < 0 {
		lowerIndex = 0
	}
	if upperIndex >= n {
		upperIndex = n - 1
	}

	lowerBound := sorted[lowerIndex]
	upperBound := sorted[upperIndex]

	// 检测异常值
	isOutlier := make([]bool, len(values))
	for i, v := range values {
		isOutlier[i] = v < lowerBound || v > upperBound
	}

	return isOutlier
}

// 辅助函数：简化表达式求值
func (h *TransformHandler) evaluateFilterExpression(expr string, value float64) bool {
	// 这是一个简化实现，实际应该使用完整的表达式引擎
	expr = strings.ReplaceAll(expr, "value", fmt.Sprintf("%f", value))
	expr = strings.TrimSpace(expr)

	// 处理简单的比较操作
	if strings.Contains(expr, ">=") {
		parts := strings.Split(expr, ">=")
		if len(parts) == 2 {
			if threshold, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				return value >= threshold
			}
		}
	} else if strings.Contains(expr, "<=") {
		parts := strings.Split(expr, "<=")
		if len(parts) == 2 {
			if threshold, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				return value <= threshold
			}
		}
	} else if strings.Contains(expr, ">") {
		parts := strings.Split(expr, ">")
		if len(parts) == 2 {
			if threshold, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				return value > threshold
			}
		}
	} else if strings.Contains(expr, "<") {
		parts := strings.Split(expr, "<")
		if len(parts) == 2 {
			if threshold, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				return value < threshold
			}
		}
	} else if strings.Contains(expr, "==") {
		parts := strings.Split(expr, "==")
		if len(parts) == 2 {
			if threshold, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
				return math.Abs(value - threshold) < 1e-10
			}
		}
	}

	return true // 默认通过
}

// 辅助函数：平滑算法
func (h *TransformHandler) movingAverage(values []float64, window int) []float64 {
	if len(values) == 0 || window <= 0 {
		return values
	}

	result := make([]float64, len(values))
	
	for i := range values {
		start := max(0, i-window/2)
		end := min(len(values), i+window/2+1)
		
		sum := 0.0
		count := 0
		for j := start; j < end; j++ {
			sum += values[j]
			count++
		}
		
		if count > 0 {
			result[i] = sum / float64(count)
		} else {
			result[i] = values[i]
		}
	}
	
	return result
}

func (h *TransformHandler) gaussianSmooth(values []float64, window int) []float64 {
	if len(values) == 0 || window <= 0 {
		return values
	}

	// 简化的高斯平滑（使用固定权重）
	result := make([]float64, len(values))
	sigma := float64(window) / 6.0 // 标准差
	
	for i := range values {
		weightedSum := 0.0
		totalWeight := 0.0
		
		for j := max(0, i-window); j < min(len(values), i+window+1); j++ {
			distance := float64(i - j)
			weight := math.Exp(-(distance*distance)/(2*sigma*sigma))
			weightedSum += values[j] * weight
			totalWeight += weight
		}
		
		if totalWeight > 0 {
			result[i] = weightedSum / totalWeight
		} else {
			result[i] = values[i]
		}
	}
	
	return result
}

func (h *TransformHandler) savgolSmooth(values []float64, window int) []float64 {
	// 简化的Savitzky-Golay滤波（使用多项式拟合的简化版本）
	if len(values) == 0 || window <= 0 {
		return values
	}

	result := make([]float64, len(values))
	halfWindow := window / 2
	
	for i := range values {
		start := max(0, i-halfWindow)
		end := min(len(values), i+halfWindow+1)
		
		// 简化为移动平均（实际Savgol需要多项式拟合）
		sum := 0.0
		count := 0
		for j := start; j < end; j++ {
			sum += values[j]
			count++
		}
		
		if count > 0 {
			result[i] = sum / float64(count)
		} else {
			result[i] = values[i]
		}
	}
	
	return result
}

// 辅助函数：归一化算法
func (h *TransformHandler) minMaxNormalize(values []float64) []float64 {
	if len(values) == 0 {
		return values
	}

	min := values[0]
	max := values[0]
	
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	if max == min {
		return values
	}

	result := make([]float64, len(values))
	for i, v := range values {
		result[i] = (v - min) / (max - min)
	}

	return result
}

func (h *TransformHandler) zScoreNormalize(values []float64) []float64 {
	if len(values) < 2 {
		return values
	}

	// 计算均值
	mean := 0.0
	for _, v := range values {
		mean += v
	}
	mean /= float64(len(values))

	// 计算标准差
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values) - 1)
	stddev := math.Sqrt(variance)

	if stddev == 0 {
		return values
	}

	result := make([]float64, len(values))
	for i, v := range values {
		result[i] = (v - mean) / stddev
	}

	return result
}

func (h *TransformHandler) robustNormalize(values []float64) []float64 {
	if len(values) < 2 {
		return values
	}

	// 复制并排序
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	// 计算中位数和四分位数
	n := len(sorted)
	median := sorted[n/2]
	q1 := sorted[n/4]
	q3 := sorted[3*n/4]
	iqr := q3 - q1

	if iqr == 0 {
		return values
	}

	result := make([]float64, len(values))
	for i, v := range values {
		result[i] = (v - median) / iqr
	}

	return result
}

// 辅助函数
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// calculatePercentile 计算百分位数
func (h *TransformHandler) calculatePercentile(arrayData *model.ArrayData, percentile float64) (float64, error) {
	numericValues := make([]float64, 0)
	for _, val := range arrayData.Values {
		if num, err := h.toFloat64(val); err == nil {
			numericValues = append(numericValues, num)
		}
	}
	
	if len(numericValues) == 0 {
		return 0.0, nil
	}

	// 排序
	sort.Float64s(numericValues)
	n := len(numericValues)

	// 计算百分位数位置
	pos := percentile / 100.0 * float64(n-1)
	
	if pos == float64(int(pos)) {
		// 整数位置
		return numericValues[int(pos)], nil
	} else {
		// 插值计算
		lower := int(math.Floor(pos))
		upper := int(math.Ceil(pos))
		
		if upper >= n {
			return numericValues[n-1], nil
		}
		
		weight := pos - float64(lower)
		return numericValues[lower]*(1-weight) + numericValues[upper]*weight, nil
	}
}

// GeoTransformDispatch GPS通用变换调度器，根据sub_type参数调用对应的GPS操作函数
func (h *TransformHandler) GeoTransformDispatch(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, model.DataType, error) {
	// 获取sub_type参数
	subType, ok := parameters["sub_type"].(string)
	if !ok {
		return nil, model.TypeFloat, fmt.Errorf("缺少sub_type参数")
	}

	// 转换前端参数格式到后端期望的格式
	convertedParams := make(map[string]interface{})
	for k, v := range parameters {
		convertedParams[k] = v
	}

	// 处理reference_point参数转换
	if refPoint, ok := parameters["reference_point"].(map[string]interface{}); ok {
		if lat, exists := refPoint["latitude"]; exists {
			convertedParams["target_latitude"] = lat
			convertedParams["center_lat"] = lat // 地理围栏用
		}
		if lng, exists := refPoint["longitude"]; exists {
			convertedParams["target_longitude"] = lng
			convertedParams["center_lng"] = lng // 地理围栏用
		}
	}

	// 处理单位参数转换（前端使用完整单词，后端使用缩写）
	if unit, ok := parameters["unit"].(string); ok {
		switch unit {
		case "kilometers":
			convertedParams["unit"] = "km"
		case "meters":
			convertedParams["unit"] = "m"
		case "miles":
			convertedParams["unit"] = "mi"
		default:
			convertedParams["unit"] = unit
		}
	}

	// 根据sub_type调用对应的GPS操作函数
	switch subType {
	case "distance":
		// 距离计算
		result, err := h.geoDistanceTransform(compositeData, convertedParams)
		return result, model.TypeFloat, err
		
	case "bearing":
		// 方位角计算
		result, err := h.geoBearingTransform(compositeData, convertedParams)
		return result, model.TypeFloat, err
		
	case "geofence":
		// 地理围栏检查
		result, err := h.geoGeofenceTransform(compositeData, convertedParams)
		return result, model.TypeFloat, err
		
	case "coordinate_convert":
		// 坐标系转换
		result, err := h.geoCoordinateConvertTransform(compositeData, convertedParams)
		return result, model.TypeLocation, err
		
	default:
		return nil, model.TypeFloat, fmt.Errorf("不支持的GPS操作类型: %s", subType)
	}
}

// VectorTransformDispatch 3D向量操作调度器
func (h *TransformHandler) VectorTransformDispatch(compositeData model.CompositeData, parameters map[string]interface{}) (interface{}, model.DataType, error) {
	// 获取sub_type参数
	subType, ok := parameters["sub_type"].(string)
	if !ok {
		return nil, model.TypeFloat, fmt.Errorf("缺少sub_type参数")
	}

	// 转换前端参数格式到后端期望的格式
	convertedParams := make(map[string]interface{})
	for k, v := range parameters {
		convertedParams[k] = v
	}

	// 处理reference_vector参数转换
	if refVector, ok := parameters["reference_vector"].(map[string]interface{}); ok {
		if x, exists := refVector["x"]; exists {
			convertedParams["reference_x"] = x
		}
		if y, exists := refVector["y"]; exists {
			convertedParams["reference_y"] = y
		}
		if z, exists := refVector["z"]; exists {
			convertedParams["reference_z"] = z
		}
	}

	// 处理custom_axis参数转换
	if customAxis, ok := parameters["custom_axis"].(map[string]interface{}); ok {
		if x, exists := customAxis["x"]; exists {
			convertedParams["axis_x"] = x
		}
		if y, exists := customAxis["y"]; exists {
			convertedParams["axis_y"] = y
		}
		if z, exists := customAxis["z"]; exists {
			convertedParams["axis_z"] = z
		}
	}

	// 统一数据类型处理：将Vector3D转换为VectorData（如果需要）
	vectorData, err := h.normalizeVectorData(compositeData)
	if err != nil {
		return nil, model.TypeFloat, err
	}

	// 根据sub_type调用对应的向量操作函数
	switch subType {
	case "magnitude":
		// 向量模长计算
		result, err := h.vectorMagnitudeFromVectorData(vectorData, convertedParams)
		return result, model.TypeFloat, err
		
	case "normalize":
		// 向量归一化
		result, err := h.vectorNormalizeFromVectorData(vectorData, convertedParams)
		return result, model.TypeVector3D, err
		
	case "projection":
		// 向量投影
		result, err := h.vectorProjectionTransform(vectorData, convertedParams)
		return result, model.TypeVector3D, err
		
	case "cross_product":
		// 向量叉积
		result, err := h.vectorCrossTransform(vectorData, convertedParams)
		return result, model.TypeVector3D, err
		
	case "dot_product":
		// 向量点积
		result, err := h.vectorDotTransform(vectorData, convertedParams)
		return result, model.TypeFloat, err
		
	case "rotation":
		// 向量旋转
		result, err := h.vectorRotationTransform(vectorData, convertedParams)
		return result, model.TypeVector3D, err
		
	default:
		return nil, model.TypeFloat, fmt.Errorf("不支持的向量操作类型: %s", subType)
	}
}

// normalizeVectorData 统一向量数据格式
func (h *TransformHandler) normalizeVectorData(compositeData model.CompositeData) (*model.VectorData, error) {
	// 如果已经是VectorData，直接返回
	if vectorData, ok := compositeData.(*model.VectorData); ok {
		return vectorData, nil
	}

	// 如果是Vector3D，转换为VectorData
	if vector3D, ok := compositeData.(*model.Vector3D); ok {
		return &model.VectorData{
			Values:    []float64{vector3D.X, vector3D.Y, vector3D.Z},
			Dimension: 3,
			Labels:    []string{"x", "y", "z"},
			Unit:      "",
		}, nil
	}

	return nil, fmt.Errorf("数据类型不是向量数据")
}

// vectorMagnitudeFromVectorData 从VectorData计算模长
func (h *TransformHandler) vectorMagnitudeFromVectorData(vectorData *model.VectorData, params map[string]interface{}) (interface{}, error) {
	if len(vectorData.Values) < 3 {
		return nil, fmt.Errorf("向量维度不足3维")
	}

	x, y, z := vectorData.Values[0], vectorData.Values[1], vectorData.Values[2]
	magnitude := math.Sqrt(x*x + y*y + z*z)
	return magnitude, nil
}

// vectorNormalizeFromVectorData 从VectorData进行归一化
func (h *TransformHandler) vectorNormalizeFromVectorData(vectorData *model.VectorData, params map[string]interface{}) (interface{}, error) {
	if len(vectorData.Values) < 3 {
		return nil, fmt.Errorf("向量维度不足3维")
	}

	x, y, z := vectorData.Values[0], vectorData.Values[1], vectorData.Values[2]
	magnitude := math.Sqrt(x*x + y*y + z*z)
	
	if magnitude == 0 {
		return &model.VectorData{
			Values:    []float64{0, 0, 0},
			Dimension: 3,
			Labels:    vectorData.Labels,
			Unit:      vectorData.Unit,
		}, nil
	}

	// 获取目标模长（默认为1.0）
	targetMagnitude := 1.0
	if normMag, ok := params["normalize_magnitude"].(float64); ok && normMag > 0 {
		targetMagnitude = normMag
	}

	scale := targetMagnitude / magnitude
	return &model.VectorData{
		Values:    []float64{x * scale, y * scale, z * scale},
		Dimension: 3,
		Labels:    vectorData.Labels,
		Unit:      vectorData.Unit,
	}, nil
}

// vectorRotationTransform 向量旋转操作
func (h *TransformHandler) vectorRotationTransform(vectorData *model.VectorData, params map[string]interface{}) (interface{}, error) {
	if len(vectorData.Values) < 3 {
		return nil, fmt.Errorf("向量维度不足3维")
	}

	x, y, z := vectorData.Values[0], vectorData.Values[1], vectorData.Values[2]

	// 获取旋转角度（度）
	angle, ok := params["rotation_angle"].(float64)
	if !ok {
		return nil, fmt.Errorf("缺少旋转角度参数")
	}
	
	// 转换为弧度
	angleRad := angle * math.Pi / 180.0

	// 获取旋转轴
	axis, ok := params["rotation_axis"].(string)
	if !ok {
		axis = "z" // 默认Z轴
	}

	var newX, newY, newZ float64

	switch axis {
	case "x":
		// 绕X轴旋转
		newX = x
		newY = y*math.Cos(angleRad) - z*math.Sin(angleRad)
		newZ = y*math.Sin(angleRad) + z*math.Cos(angleRad)
	case "y":
		// 绕Y轴旋转
		newX = x*math.Cos(angleRad) + z*math.Sin(angleRad)
		newY = y
		newZ = -x*math.Sin(angleRad) + z*math.Cos(angleRad)
	case "z":
		// 绕Z轴旋转
		newX = x*math.Cos(angleRad) - y*math.Sin(angleRad)
		newY = x*math.Sin(angleRad) + y*math.Cos(angleRad)
		newZ = z
	case "custom":
		// 绕自定义轴旋转（使用罗德里格旋转公式）
		axisX, ok1 := params["axis_x"].(float64)
		axisY, ok2 := params["axis_y"].(float64)
		axisZ, ok3 := params["axis_z"].(float64)
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("缺少自定义轴参数")
		}

		// 归一化旋转轴
		axisLength := math.Sqrt(axisX*axisX + axisY*axisY + axisZ*axisZ)
		if axisLength == 0 {
			return nil, fmt.Errorf("自定义轴不能为零向量")
		}
		axisX /= axisLength
		axisY /= axisLength
		axisZ /= axisLength

		// 罗德里格旋转公式
		cosAngle := math.Cos(angleRad)
		sinAngle := math.Sin(angleRad)
		oneMinusCos := 1 - cosAngle

		// 点积 v·k
		dotProduct := x*axisX + y*axisY + z*axisZ

		// 叉积 k×v
		crossX := axisY*z - axisZ*y
		crossY := axisZ*x - axisX*z
		crossZ := axisX*y - axisY*x

		// v_rot = v*cos(θ) + (k×v)*sin(θ) + k*(k·v)*(1-cos(θ))
		newX = x*cosAngle + crossX*sinAngle + axisX*dotProduct*oneMinusCos
		newY = y*cosAngle + crossY*sinAngle + axisY*dotProduct*oneMinusCos
		newZ = z*cosAngle + crossZ*sinAngle + axisZ*dotProduct*oneMinusCos
	default:
		return nil, fmt.Errorf("不支持的旋转轴: %s", axis)
	}

	return &model.VectorData{
		Values:    []float64{newX, newY, newZ},
		Dimension: 3,
		Labels:    vectorData.Labels,
		Unit:      vectorData.Unit,
	}, nil
}

// parseTemplateString 解析模板字符串，支持{{.Key}}等占位符
func (h *TransformHandler) parseTemplateString(templateStr string, point model.Point) string {
	if templateStr == "" {
		return templateStr
	}
	
	result := templateStr
	
	// 替换基本变量
	replacements := map[string]string{
		"{{.DeviceID}}":  point.DeviceID,
		"{{.Key}}":       point.Key,
		"{{.Value}}":     fmt.Sprintf("%v", point.Value),
		"{{.Type}}":      string(point.Type),
		"{{.Timestamp}}": point.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}
	
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	
	// 处理标签模板
	pointTags := point.GetTagsSafe()
	for key, value := range pointTags {
		placeholder := fmt.Sprintf("{{.Tags.%s}}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}
	
	return result
}
