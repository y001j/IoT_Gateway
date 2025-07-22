package rules

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
)

// ExpressionEngine 增强的表达式引擎
type ExpressionEngine struct {
	functions map[string]ExprFunction
	variables map[string]interface{}
}

// ExprFunction 表达式函数接口
type ExprFunction interface {
	Name() string
	Call(args ...interface{}) (interface{}, error)
	Description() string
}

// NewExpressionEngine 创建表达式引擎
func NewExpressionEngine() *ExpressionEngine {
	engine := &ExpressionEngine{
		functions: make(map[string]ExprFunction),
		variables: make(map[string]interface{}),
	}
	
	// 注册内置函数
	engine.registerBuiltinFunctions()
	
	return engine
}

// Evaluate 评估表达式
func (e *ExpressionEngine) Evaluate(expression string, point model.Point) (interface{}, error) {
	// 设置当前数据点的变量
	e.setPointVariables(point)
	
	// 尝试使用Go表达式解析器（安全的表达式子集）
	if result, err := e.evaluateGoExpression(expression); err == nil {
		return result, nil
	}
	
	// 回退到自定义表达式解析器
	return e.evaluateCustomExpression(expression)
}

// setPointVariables 设置数据点变量
func (e *ExpressionEngine) setPointVariables(point model.Point) {
	e.variables["device_id"] = point.DeviceID
	e.variables["key"] = point.Key
	e.variables["value"] = point.Value
	e.variables["type"] = string(point.Type)
	e.variables["timestamp"] = point.Timestamp
	
	// 添加时间相关变量
	now := time.Now()
	e.variables["now"] = now
	e.variables["today"] = now.Truncate(24 * time.Hour)
	e.variables["hour"] = now.Hour()
	e.variables["minute"] = now.Minute()
	e.variables["weekday"] = int(now.Weekday())
	
	// 添加数值变量（如果value是数值）
	if numValue, ok := toFloat64(point.Value); ok {
		e.variables["num_value"] = numValue
	}
	
	// 添加标签变量
	if point.Tags != nil {
		for k, v := range point.Tags {
			e.variables["tag_"+k] = v
		}
	}
}

// evaluateGoExpression 使用Go解析器评估安全表达式
func (e *ExpressionEngine) evaluateGoExpression(expression string) (interface{}, error) {
	// 仅支持安全的表达式类型，不允许函数调用和复杂语句
	expr, err := parser.ParseExpr(expression)
	if err != nil {
		return nil, err
	}
	
	return e.evaluateASTNode(expr)
}

// evaluateASTNode 评估AST节点
func (e *ExpressionEngine) evaluateASTNode(node ast.Node) (interface{}, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		return e.parseBasicLit(n)
	case *ast.Ident:
		if value, exists := e.variables[n.Name]; exists {
			return value, nil
		}
		return nil, fmt.Errorf("未定义的变量: %s", n.Name)
	case *ast.BinaryExpr:
		return e.evaluateBinaryExpr(n)
	case *ast.UnaryExpr:
		return e.evaluateUnaryExpr(n)
	case *ast.ParenExpr:
		return e.evaluateASTNode(n.X)
	case *ast.CallExpr:
		return e.evaluateCallExpr(n)
	default:
		return nil, fmt.Errorf("不支持的表达式类型: %T", node)
	}
}

// parseBasicLit 解析基本字面量
func (e *ExpressionEngine) parseBasicLit(lit *ast.BasicLit) (interface{}, error) {
	switch lit.Kind {
	case token.INT:
		return strconv.ParseInt(lit.Value, 10, 64)
	case token.FLOAT:
		return strconv.ParseFloat(lit.Value, 64)
	case token.STRING:
		// 去掉引号
		return strconv.Unquote(lit.Value)
	default:
		return nil, fmt.Errorf("不支持的字面量类型: %s", lit.Kind)
	}
}

// evaluateBinaryExpr 评估二元表达式
func (e *ExpressionEngine) evaluateBinaryExpr(expr *ast.BinaryExpr) (interface{}, error) {
	left, err := e.evaluateASTNode(expr.X)
	if err != nil {
		return nil, err
	}
	
	right, err := e.evaluateASTNode(expr.Y)
	if err != nil {
		return nil, err
	}
	
	return e.applyBinaryOperator(left, right, expr.Op)
}

// applyBinaryOperator 应用二元操作符
func (e *ExpressionEngine) applyBinaryOperator(left, right interface{}, op token.Token) (interface{}, error) {
	switch op {
	case token.ADD:
		return e.add(left, right)
	case token.SUB:
		return e.subtract(left, right)
	case token.MUL:
		return e.multiply(left, right)
	case token.QUO:
		return e.divide(left, right)
	case token.REM:
		return e.modulo(left, right)
	case token.EQL:
		return e.equal(left, right), nil
	case token.NEQ:
		return !e.equal(left, right), nil
	case token.LSS:
		return e.less(left, right)
	case token.GTR:
		return e.greater(left, right)
	case token.LEQ:
		return e.lessEqual(left, right)
	case token.GEQ:
		return e.greaterEqual(left, right)
	case token.LAND:
		return e.logicalAnd(left, right), nil
	case token.LOR:
		return e.logicalOr(left, right), nil
	default:
		return nil, fmt.Errorf("不支持的二元操作符: %s", op)
	}
}

// evaluateUnaryExpr 评估一元表达式
func (e *ExpressionEngine) evaluateUnaryExpr(expr *ast.UnaryExpr) (interface{}, error) {
	operand, err := e.evaluateASTNode(expr.X)
	if err != nil {
		return nil, err
	}
	
	switch expr.Op {
	case token.SUB:
		if num, ok := toFloat64(operand); ok {
			return -num, nil
		}
		return nil, fmt.Errorf("无法对非数值类型应用负号")
	case token.NOT:
		return !toBool(operand), nil
	default:
		return nil, fmt.Errorf("不支持的一元操作符: %s", expr.Op)
	}
}

// evaluateCallExpr 评估函数调用表达式
func (e *ExpressionEngine) evaluateCallExpr(expr *ast.CallExpr) (interface{}, error) {
	// 获取函数名
	var funcName string
	if ident, ok := expr.Fun.(*ast.Ident); ok {
		funcName = ident.Name
	} else {
		return nil, fmt.Errorf("不支持的函数调用形式")
	}
	
	// 查找函数
	function, exists := e.functions[funcName]
	if !exists {
		return nil, fmt.Errorf("未定义的函数: %s", funcName)
	}
	
	// 评估参数
	var args []interface{}
	for _, arg := range expr.Args {
		value, err := e.evaluateASTNode(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	
	// 调用函数
	return function.Call(args...)
}

// evaluateCustomExpression 评估自定义表达式（复杂模式匹配等）
func (e *ExpressionEngine) evaluateCustomExpression(expression string) (interface{}, error) {
	// 处理正则表达式匹配
	if strings.HasPrefix(expression, "regex(") && strings.HasSuffix(expression, ")") {
		return e.evaluateRegexExpression(expression)
	}
	
	// 处理时间范围检查
	if strings.Contains(expression, "time_range") {
		return e.evaluateTimeRangeExpression(expression)
	}
	
	// 处理数组包含检查
	if strings.Contains(expression, "in_array") {
		return e.evaluateInArrayExpression(expression)
	}
	
	return nil, fmt.Errorf("无法解析自定义表达式: %s", expression)
}

// evaluateRegexExpression 评估正则表达式
func (e *ExpressionEngine) evaluateRegexExpression(expression string) (interface{}, error) {
	// 提取参数: regex(pattern, field)
	content := expression[6 : len(expression)-1] // 去掉 "regex(" 和 ")"
	parts := strings.Split(content, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("regex函数需要2个参数")
	}
	
	pattern := strings.Trim(strings.TrimSpace(parts[0]), "\"'")
	fieldName := strings.TrimSpace(parts[1])
	
	// 获取字段值
	fieldValue, exists := e.variables[fieldName]
	if !exists {
		return false, nil
	}
	
	fieldStr := fmt.Sprintf("%v", fieldValue)
	
	// 使用缓存的正则表达式匹配
	matched, err := MatchString(pattern, fieldStr)
	if err != nil {
		return nil, fmt.Errorf("正则表达式匹配失败: %v", err)
	}
	
	return matched, nil
}

// evaluateTimeRangeExpression 评估时间范围表达式
func (e *ExpressionEngine) evaluateTimeRangeExpression(expression string) (interface{}, error) {
	// 简化实现：time_range(start_hour, end_hour)
	// 检查当前时间是否在指定小时范围内
	if !strings.HasPrefix(expression, "time_range(") {
		return nil, fmt.Errorf("无效的时间范围表达式")
	}
	
	content := expression[11 : len(expression)-1]
	parts := strings.Split(content, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("time_range函数需要2个参数")
	}
	
	startHour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("开始小时解析失败: %v", err)
	}
	
	endHour, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("结束小时解析失败: %v", err)
	}
	
	currentHour := time.Now().Hour()
	
	if startHour <= endHour {
		return currentHour >= startHour && currentHour <= endHour, nil
	} else {
		// 跨越午夜的情况
		return currentHour >= startHour || currentHour <= endHour, nil
	}
}

// evaluateInArrayExpression 评估数组包含表达式
func (e *ExpressionEngine) evaluateInArrayExpression(expression string) (interface{}, error) {
	// 简化实现：in_array(value, [item1, item2, ...])
	return false, fmt.Errorf("in_array函数暂未实现")
}

// 数学运算函数
func (e *ExpressionEngine) add(left, right interface{}) (interface{}, error) {
	if leftNum, ok := toFloat64(left); ok {
		if rightNum, ok := toFloat64(right); ok {
			return leftNum + rightNum, nil
		}
	}
	// 字符串连接
	return fmt.Sprintf("%v%v", left, right), nil
}

func (e *ExpressionEngine) subtract(left, right interface{}) (interface{}, error) {
	leftNum, leftOk := toFloat64(left)
	rightNum, rightOk := toFloat64(right)
	if !leftOk || !rightOk {
		return nil, fmt.Errorf("减法操作需要数值类型")
	}
	return leftNum - rightNum, nil
}

func (e *ExpressionEngine) multiply(left, right interface{}) (interface{}, error) {
	leftNum, leftOk := toFloat64(left)
	rightNum, rightOk := toFloat64(right)
	if !leftOk || !rightOk {
		return nil, fmt.Errorf("乘法操作需要数值类型")
	}
	return leftNum * rightNum, nil
}

func (e *ExpressionEngine) divide(left, right interface{}) (interface{}, error) {
	leftNum, leftOk := toFloat64(left)
	rightNum, rightOk := toFloat64(right)
	if !leftOk || !rightOk {
		return nil, fmt.Errorf("除法操作需要数值类型")
	}
	if rightNum == 0 {
		return nil, fmt.Errorf("除零错误")
	}
	return leftNum / rightNum, nil
}

func (e *ExpressionEngine) modulo(left, right interface{}) (interface{}, error) {
	leftNum, leftOk := toFloat64(left)
	rightNum, rightOk := toFloat64(right)
	if !leftOk || !rightOk {
		return nil, fmt.Errorf("取模操作需要数值类型")
	}
	if rightNum == 0 {
		return nil, fmt.Errorf("除零错误")
	}
	return math.Mod(leftNum, rightNum), nil
}

// 比较函数
func (e *ExpressionEngine) equal(left, right interface{}) bool {
	return compareValues(left, right) == 0
}

func (e *ExpressionEngine) less(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 { // 无法比较
		return false, fmt.Errorf("无法比较类型 %T 和 %T", left, right)
	}
	return result < 0, nil
}

func (e *ExpressionEngine) greater(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 {
		return false, fmt.Errorf("无法比较类型 %T 和 %T", left, right)
	}
	return result > 0, nil
}

func (e *ExpressionEngine) lessEqual(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 {
		return false, fmt.Errorf("无法比较类型 %T 和 %T", left, right)
	}
	return result <= 0, nil
}

func (e *ExpressionEngine) greaterEqual(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 {
		return false, fmt.Errorf("无法比较类型 %T 和 %T", left, right)
	}
	return result >= 0, nil
}

// 逻辑函数
func (e *ExpressionEngine) logicalAnd(left, right interface{}) bool {
	return toBool(left) && toBool(right)
}

func (e *ExpressionEngine) logicalOr(left, right interface{}) bool {
	return toBool(left) || toBool(right)
}

// RegisterFunction 注册自定义函数
func (e *ExpressionEngine) RegisterFunction(fn ExprFunction) {
	e.functions[fn.Name()] = fn
}

// SetVariable 设置变量
func (e *ExpressionEngine) SetVariable(name string, value interface{}) {
	e.variables[name] = value
}

// registerBuiltinFunctions 注册内置函数
func (e *ExpressionEngine) registerBuiltinFunctions() {
	// 数学函数
	e.RegisterFunction(&MathAbsFunction{})
	e.RegisterFunction(&MathMaxFunction{})
	e.RegisterFunction(&MathMinFunction{})
	e.RegisterFunction(&MathSqrtFunction{})
	e.RegisterFunction(&MathPowFunction{})
	e.RegisterFunction(&MathFloorFunction{})
	e.RegisterFunction(&MathCeilFunction{})
	
	// 字符串函数
	e.RegisterFunction(&StringLenFunction{})
	e.RegisterFunction(&StringUpperFunction{})
	e.RegisterFunction(&StringLowerFunction{})
	e.RegisterFunction(&StringContainsFunction{})
	e.RegisterFunction(&StringStartsWithFunction{})
	e.RegisterFunction(&StringEndsWithFunction{})
	
	// 时间函数
	e.RegisterFunction(&TimeNowFunction{})
	e.RegisterFunction(&TimeFormatFunction{})
	e.RegisterFunction(&TimeDiffFunction{})
	
	// 类型转换函数
	e.RegisterFunction(&ConvertToStringFunction{})
	e.RegisterFunction(&ConvertToNumberFunction{})
	e.RegisterFunction(&ConvertToBoolFunction{})
}

// 内置函数实现

// 数学函数
type MathAbsFunction struct{}
func (f *MathAbsFunction) Name() string { return "abs" }
func (f *MathAbsFunction) Description() string { return "返回数值的绝对值" }
func (f *MathAbsFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("abs函数需要1个参数")
	}
	if num, ok := toFloat64(args[0]); ok {
		return math.Abs(num), nil
	}
	return nil, fmt.Errorf("abs函数参数必须是数值")
}

type MathMaxFunction struct{}
func (f *MathMaxFunction) Name() string { return "max" }
func (f *MathMaxFunction) Description() string { return "返回多个数值中的最大值" }
func (f *MathMaxFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("max函数至少需要2个参数")
	}
	maxVal, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("max函数参数必须是数值")
	}
	for i := 1; i < len(args); i++ {
		if num, ok := toFloat64(args[i]); ok {
			maxVal = math.Max(maxVal, num)
		} else {
			return nil, fmt.Errorf("max函数第%d个参数必须是数值", i+1)
		}
	}
	return maxVal, nil
}

type MathMinFunction struct{}
func (f *MathMinFunction) Name() string { return "min" }
func (f *MathMinFunction) Description() string { return "返回多个数值中的最小值" }
func (f *MathMinFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("min函数至少需要2个参数")
	}
	minVal, ok := toFloat64(args[0])
	if !ok {
		return nil, fmt.Errorf("min函数参数必须是数值")
	}
	for i := 1; i < len(args); i++ {
		if num, ok := toFloat64(args[i]); ok {
			minVal = math.Min(minVal, num)
		} else {
			return nil, fmt.Errorf("min函数第%d个参数必须是数值", i+1)
		}
	}
	return minVal, nil
}

type MathSqrtFunction struct{}
func (f *MathSqrtFunction) Name() string { return "sqrt" }
func (f *MathSqrtFunction) Description() string { return "返回数值的平方根" }
func (f *MathSqrtFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sqrt函数需要1个参数")
	}
	if num, ok := toFloat64(args[0]); ok {
		if num < 0 {
			return nil, fmt.Errorf("sqrt函数不支持负数")
		}
		return math.Sqrt(num), nil
	}
	return nil, fmt.Errorf("sqrt函数参数必须是数值")
}

type MathPowFunction struct{}
func (f *MathPowFunction) Name() string { return "pow" }
func (f *MathPowFunction) Description() string { return "返回x的y次幂" }
func (f *MathPowFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("pow函数需要2个参数")
	}
	x, xOk := toFloat64(args[0])
	y, yOk := toFloat64(args[1])
	if !xOk || !yOk {
		return nil, fmt.Errorf("pow函数参数必须是数值")
	}
	return math.Pow(x, y), nil
}

type MathFloorFunction struct{}
func (f *MathFloorFunction) Name() string { return "floor" }
func (f *MathFloorFunction) Description() string { return "返回不大于参数的最大整数" }
func (f *MathFloorFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("floor函数需要1个参数")
	}
	if num, ok := toFloat64(args[0]); ok {
		return math.Floor(num), nil
	}
	return nil, fmt.Errorf("floor函数参数必须是数值")
}

type MathCeilFunction struct{}
func (f *MathCeilFunction) Name() string { return "ceil" }
func (f *MathCeilFunction) Description() string { return "返回不小于参数的最小整数" }
func (f *MathCeilFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ceil函数需要1个参数")
	}
	if num, ok := toFloat64(args[0]); ok {
		return math.Ceil(num), nil
	}
	return nil, fmt.Errorf("ceil函数参数必须是数值")
}

// 字符串函数
type StringLenFunction struct{}
func (f *StringLenFunction) Name() string { return "len" }
func (f *StringLenFunction) Description() string { return "返回字符串的长度" }
func (f *StringLenFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("len函数需要1个参数")
	}
	str := fmt.Sprintf("%v", args[0])
	return len(str), nil
}

type StringUpperFunction struct{}
func (f *StringUpperFunction) Name() string { return "upper" }
func (f *StringUpperFunction) Description() string { return "将字符串转换为大写" }
func (f *StringUpperFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("upper函数需要1个参数")
	}
	str := fmt.Sprintf("%v", args[0])
	return strings.ToUpper(str), nil
}

type StringLowerFunction struct{}
func (f *StringLowerFunction) Name() string { return "lower" }
func (f *StringLowerFunction) Description() string { return "将字符串转换为小写" }
func (f *StringLowerFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("lower函数需要1个参数")
	}
	str := fmt.Sprintf("%v", args[0])
	return strings.ToLower(str), nil
}

type StringContainsFunction struct{}
func (f *StringContainsFunction) Name() string { return "contains" }
func (f *StringContainsFunction) Description() string { return "检查字符串是否包含子字符串" }
func (f *StringContainsFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("contains函数需要2个参数")
	}
	str := fmt.Sprintf("%v", args[0])
	substr := fmt.Sprintf("%v", args[1])
	return strings.Contains(str, substr), nil
}

type StringStartsWithFunction struct{}
func (f *StringStartsWithFunction) Name() string { return "startsWith" }
func (f *StringStartsWithFunction) Description() string { return "检查字符串是否以指定前缀开始" }
func (f *StringStartsWithFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("startsWith函数需要2个参数")
	}
	str := fmt.Sprintf("%v", args[0])
	prefix := fmt.Sprintf("%v", args[1])
	return strings.HasPrefix(str, prefix), nil
}

type StringEndsWithFunction struct{}
func (f *StringEndsWithFunction) Name() string { return "endsWith" }
func (f *StringEndsWithFunction) Description() string { return "检查字符串是否以指定后缀结束" }
func (f *StringEndsWithFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("endsWith函数需要2个参数")
	}
	str := fmt.Sprintf("%v", args[0])
	suffix := fmt.Sprintf("%v", args[1])
	return strings.HasSuffix(str, suffix), nil
}

// 时间函数
type TimeNowFunction struct{}
func (f *TimeNowFunction) Name() string { return "now" }
func (f *TimeNowFunction) Description() string { return "返回当前时间戳" }
func (f *TimeNowFunction) Call(args ...interface{}) (interface{}, error) {
	return time.Now().Unix(), nil
}

type TimeFormatFunction struct{}
func (f *TimeFormatFunction) Name() string { return "timeFormat" }
func (f *TimeFormatFunction) Description() string { return "格式化时间" }
func (f *TimeFormatFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("timeFormat函数需要2个参数")
	}
	
	var t time.Time
	switch v := args[0].(type) {
	case time.Time:
		t = v
	case int64:
		t = time.Unix(v, 0)
	case float64:
		t = time.Unix(int64(v), 0)
	default:
		return nil, fmt.Errorf("timeFormat函数第一个参数必须是时间或时间戳")
	}
	
	format := fmt.Sprintf("%v", args[1])
	return t.Format(format), nil
}

type TimeDiffFunction struct{}
func (f *TimeDiffFunction) Name() string { return "timeDiff" }
func (f *TimeDiffFunction) Description() string { return "计算两个时间的差值(秒)" }
func (f *TimeDiffFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("timeDiff函数需要2个参数")
	}
	
	var t1, t2 time.Time
	
	// 解析第一个时间
	switch v := args[0].(type) {
	case time.Time:
		t1 = v
	case int64:
		t1 = time.Unix(v, 0)
	case float64:
		t1 = time.Unix(int64(v), 0)
	default:
		return nil, fmt.Errorf("timeDiff函数第一个参数必须是时间或时间戳")
	}
	
	// 解析第二个时间
	switch v := args[1].(type) {
	case time.Time:
		t2 = v
	case int64:
		t2 = time.Unix(v, 0)
	case float64:
		t2 = time.Unix(int64(v), 0)
	default:
		return nil, fmt.Errorf("timeDiff函数第二个参数必须是时间或时间戳")
	}
	
	return t1.Sub(t2).Seconds(), nil
}

// 类型转换函数
type ConvertToStringFunction struct{}
func (f *ConvertToStringFunction) Name() string { return "toString" }
func (f *ConvertToStringFunction) Description() string { return "将值转换为字符串" }
func (f *ConvertToStringFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("toString函数需要1个参数")
	}
	return fmt.Sprintf("%v", args[0]), nil
}

type ConvertToNumberFunction struct{}
func (f *ConvertToNumberFunction) Name() string { return "toNumber" }
func (f *ConvertToNumberFunction) Description() string { return "将值转换为数值" }
func (f *ConvertToNumberFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("toNumber函数需要1个参数")
	}
	if num, ok := toFloat64(args[0]); ok {
		return num, nil
	}
	return nil, fmt.Errorf("无法转换为数值: %v", args[0])
}

type ConvertToBoolFunction struct{}
func (f *ConvertToBoolFunction) Name() string { return "toBool" }
func (f *ConvertToBoolFunction) Description() string { return "将值转换为布尔值" }
func (f *ConvertToBoolFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("toBool函数需要1个参数")
	}
	return toBool(args[0]), nil
}

// 辅助函数
func toBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false" && v != "0"
	case int, int32, int64:
		return v != 0
	case float32, float64:
		if num, ok := toFloat64(v); ok {
			return num != 0
		}
	}
	return value != nil
}