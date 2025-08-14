package rules

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/rules/geo"
)

// ExpressionEngine 增强的表达式引擎
type ExpressionEngine struct {
	functions   map[string]ExprFunction
	variables   map[string]interface{}
	mu          sync.RWMutex // 保护variables map的读写锁
	geoProcessor *geo.GeoProcessor // 地理数据处理器
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
		functions:   make(map[string]ExprFunction),
		variables:   make(map[string]interface{}),
		geoProcessor: geo.NewGeoProcessor(),
	}
	
	// 初始化地理区域数据
	engine.initializeGeoData()
	
	// 注册内置函数
	engine.registerBuiltinFunctions()
	
	return engine
}

// Evaluate 评估表达式
func (e *ExpressionEngine) Evaluate(expression string, point model.Point) (interface{}, error) {
	// 设置当前数据点的变量
	e.setPointVariables(point)
	
	// 尝试使用Go表达式解析器（安全的表达式子集）
	result, goErr := e.evaluateGoExpression(expression)
	if goErr == nil {
		return result, nil
	}
	
	// 检查是否是语法错误，如果是则不回退
	if e.isSyntaxError(goErr) {
		return nil, fmt.Errorf("表达式语法错误: %v", goErr)
	}
	
	// 只有在不是语法错误时才回退到自定义表达式解析器
	result, customErr := e.evaluateCustomExpression(expression)
	return result, customErr
}

// isSyntaxError 判断是否为语法错误
func (e *ExpressionEngine) isSyntaxError(err error) bool {
	if err == nil {
		return false
	}
	
	errMsg := err.Error()
	// Go parser的典型语法错误模式
	syntaxPatterns := []string{
		"expected operand",
		"expected 'EOF'",
		"expected ')'",
		"expected '}'", 
		"expected ']'",
		"illegal character",
		"invalid character",
		"unterminated",
	}
	
	for _, pattern := range syntaxPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}
	
	return false
}

// setPointVariables 设置数据点变量
func (e *ExpressionEngine) setPointVariables(point model.Point) {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	e.variables["device_id"] = point.DeviceID
	e.variables["key"] = point.Key
	e.variables["value"] = point.Value
	e.variables["type"] = string(point.Type)
	e.variables["timestamp"] = point.Timestamp
	e.variables["quality"] = point.Quality
	
	// 增强的时间戳字段访问
	if !point.Timestamp.IsZero() {
		e.variables["timestamp_unix"] = point.Timestamp.Unix()
		e.variables["timestamp_string"] = point.Timestamp.Format(time.RFC3339)
		e.variables["timestamp_year"] = point.Timestamp.Year()
		e.variables["timestamp_month"] = int(point.Timestamp.Month())
		e.variables["timestamp_day"] = point.Timestamp.Day()
		e.variables["timestamp_hour"] = point.Timestamp.Hour()
		e.variables["timestamp_minute"] = point.Timestamp.Minute()
		e.variables["timestamp_second"] = point.Timestamp.Second()
	}
	
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
	
	// 设置标签变量（用于tags['key']访问）
	// 使用GetTagsCopy()来获取实际的标签数据，支持SafeTags
	actualTags := point.GetTagsCopy()
	if len(actualTags) > 0 {
		e.variables["tags"] = actualTags
		// 增强的标签变量支持 - 支持深层访问
		e.setDeepTagVariables(actualTags, "tag")
	} else {
		e.variables["tags"] = make(map[string]string)
	}
	
	// 处理复合数据字段变量
	e.setCompositeDataVariables(point)
	
	// 添加常用变量的默认值
	e.variables["emergency_threshold"] = 100.0
	e.variables["avg_threshold"] = 50.0
	e.variables["nil"] = nil
	
	// 模拟历史数据用于统计函数
	// 在实际应用中，这些应该来自历史数据存储
	if numValue, ok := toFloat64(point.Value); ok {
		// 生成一些示例历史数据
		last_values := make([]interface{}, 5)
		for i := 0; i < 5; i++ {
			// 在实际值附近生成模拟数据
			variance := numValue * 0.1 * (float64(i%3) - 1) // -10% to +10% variance
			last_values[i] = numValue + variance
		}
		e.variables["last_values"] = last_values
	}
}

// setDeepTagVariables 递归设置深层标签变量
func (e *ExpressionEngine) setDeepTagVariables(tags map[string]string, prefix string) {
	defer func() {
		if r := recover(); r != nil {
			// 防止并发访问map导致的panic
		}
	}()
	
	for k, v := range tags {
		// 基础标签访问
		e.variables[prefix+"_"+k] = v
		
		// 支持嵌套访问（如果值是JSON格式）
		if strings.HasPrefix(v, "{") || strings.HasPrefix(v, "[") {
			if nestedMap, ok := e.parseJSONValue(v); ok {
				e.setNestedVariables(nestedMap, prefix+"_"+k)
			}
		}
		
		// 支持点分隔符访问
		if strings.Contains(k, ".") {
			parts := strings.Split(k, ".")
			currentPrefix := prefix
			for _, part := range parts {
				currentPrefix = currentPrefix + "_" + part
				e.variables[currentPrefix] = v
			}
		}
	}
}

// parseJSONValue 尝试解析JSON值
func (e *ExpressionEngine) parseJSONValue(jsonStr string) (interface{}, bool) {
	var result interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
		return result, true
	}
	return nil, false
}

// setCompositeDataVariables 设置复合数据字段变量
func (e *ExpressionEngine) setCompositeDataVariables(point model.Point) {
	// 只处理复合数据类型
	if !isCompositeDataType(point.Type) {
		return
	}
	
	compositeData, err := point.GetCompositeData()
	if err != nil {
		return // 无法获取复合数据，忽略错误继续
	}
	
	// 根据数据类型设置相应的字段变量
	switch point.Type {
	case model.TypeLocation:
		if locationData, ok := compositeData.(*model.LocationData); ok {
			// 使用key作为前缀，例如 "location.latitude"
			prefix := point.Key
			e.variables[prefix+".latitude"] = locationData.Latitude
			e.variables[prefix+".longitude"] = locationData.Longitude
			e.variables[prefix+".altitude"] = locationData.Altitude
			e.variables[prefix+".accuracy"] = locationData.Accuracy
			e.variables[prefix+".speed"] = locationData.Speed
			e.variables[prefix+".heading"] = locationData.Heading
		}
		
	case model.TypeVector3D:
		if vectorData, ok := compositeData.(*model.Vector3D); ok {
			prefix := point.Key
			e.variables[prefix+".x"] = vectorData.X
			e.variables[prefix+".y"] = vectorData.Y
			e.variables[prefix+".z"] = vectorData.Z
		}
		
	case model.TypeVector:
		if vectorData, ok := compositeData.(*model.VectorData); ok {
			prefix := point.Key
			e.variables[prefix+".dimension"] = vectorData.Dimension
			e.variables[prefix+".length"] = len(vectorData.Values)
			e.variables[prefix+".unit"] = vectorData.Unit
			
			// 设置向量元素访问（限制前10个元素）
			for i, value := range vectorData.Values {
				if i >= 10 {
					break
				}
				e.variables[fmt.Sprintf("%s.%d", prefix, i)] = value
			}
			
			// 设置标签访问（如果有）
			for i, label := range vectorData.Labels {
				if i >= len(vectorData.Values) || i >= 10 {
					break
				}
				if label != "" {
					e.variables[prefix+"."+label] = vectorData.Values[i]
				}
			}
			
			// 设置向量对象本身，用于函数调用
			e.variables[prefix] = vectorData
		}
		
	case model.TypeColor:
		if colorData, ok := compositeData.(*model.ColorData); ok {
			prefix := point.Key
			e.variables[prefix+".r"] = int(colorData.R)
			e.variables[prefix+".g"] = int(colorData.G)
			e.variables[prefix+".b"] = int(colorData.B)
			e.variables[prefix+".a"] = int(colorData.A)
		}
		
	case model.TypeArray:
		if arrayData, ok := compositeData.(*model.ArrayData); ok {
			prefix := point.Key
			e.variables[prefix+".size"] = arrayData.Size
			e.variables[prefix+".length"] = len(arrayData.Values)
			e.variables[prefix+".data_type"] = arrayData.DataType
			e.variables[prefix+".unit"] = arrayData.Unit
			
			// 设置数组元素访问（限制前10个元素）
			for i, value := range arrayData.Values {
				if i >= 10 {
					break
				}
				e.variables[fmt.Sprintf("%s.%d", prefix, i)] = value
			}
			
			// 设置标签访问（如果有）
			for i, label := range arrayData.Labels {
				if i >= len(arrayData.Values) || i >= 10 {
					break
				}
				if label != "" {
					e.variables[prefix+"."+label] = arrayData.Values[i]
				}
			}
			
			// 设置数组对象本身，用于函数调用
			e.variables[prefix] = arrayData
		}
		
	case model.TypeMatrix:
		if matrixData, ok := compositeData.(*model.MatrixData); ok {
			prefix := point.Key
			e.variables[prefix+".rows"] = matrixData.Rows
			e.variables[prefix+".cols"] = matrixData.Cols
			e.variables[prefix+".unit"] = matrixData.Unit
			
			// 设置矩阵元素访问（格式：matrix.0_0）
			for i := 0; i < matrixData.Rows && i < 5; i++ { // 限制5x5
				for j := 0; j < matrixData.Cols && j < 5; j++ {
					if i < len(matrixData.Values) && j < len(matrixData.Values[i]) {
						e.variables[fmt.Sprintf("%s.%d_%d", prefix, i, j)] = matrixData.Values[i][j]
					}
				}
			}
		}
		
	case model.TypeTimeSeries:
		if timeSeriesData, ok := compositeData.(*model.TimeSeriesData); ok {
			prefix := point.Key
			e.variables[prefix+".length"] = len(timeSeriesData.Values)
			e.variables[prefix+".unit"] = timeSeriesData.Unit
			e.variables[prefix+".interval"] = timeSeriesData.Interval.Seconds()
			
			// 设置特殊值访问
			if len(timeSeriesData.Values) > 0 {
				e.variables[prefix+".first_value"] = timeSeriesData.Values[0]
				e.variables[prefix+".last_value"] = timeSeriesData.Values[len(timeSeriesData.Values)-1]
			}
			
			// 设置索引访问和负索引访问（限制前10个）
			for i, value := range timeSeriesData.Values {
				if i >= 10 {
					break
				}
				e.variables[fmt.Sprintf("%s.%d", prefix, i)] = value
			}
			
			// 负索引访问
			if len(timeSeriesData.Values) > 0 {
				e.variables[prefix+".-1"] = timeSeriesData.Values[len(timeSeriesData.Values)-1]
				if len(timeSeriesData.Values) > 1 {
					e.variables[prefix+".-2"] = timeSeriesData.Values[len(timeSeriesData.Values)-2]
				}
			}
		}
	}
}

// isCompositeDataType 检查是否为复合数据类型
func isCompositeDataType(dataType model.DataType) bool {
	return dataType == model.TypeLocation ||
		dataType == model.TypeVector3D ||
		dataType == model.TypeVector ||
		dataType == model.TypeColor ||
		dataType == model.TypeArray ||
		dataType == model.TypeMatrix ||
		dataType == model.TypeTimeSeries
}

// setNestedVariables 设置嵌套变量
func (e *ExpressionEngine) setNestedVariables(data interface{}, prefix string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, value := range v {
			newPrefix := prefix + "_" + key
			e.variables[newPrefix] = value
			
			// 递归处理嵌套结构
			if nestedMap, ok := value.(map[string]interface{}); ok {
				e.setNestedVariables(nestedMap, newPrefix)
			}
			if nestedArray, ok := value.([]interface{}); ok && len(nestedArray) > 0 {
				// 数组访问支持
				e.variables[newPrefix+"_length"] = len(nestedArray)
				for i, item := range nestedArray {
					if i < 10 { // 限制数组索引访问数量，防止变量爆炸
						e.variables[fmt.Sprintf("%s_%d", newPrefix, i)] = item
					}
				}
			}
		}
	case []interface{}:
		e.variables[prefix+"_length"] = len(v)
		for i, item := range v {
			if i < 10 { // 限制数组索引访问数量
				e.variables[fmt.Sprintf("%s_%d", prefix, i)] = item
				if nestedMap, ok := item.(map[string]interface{}); ok {
					e.setNestedVariables(nestedMap, fmt.Sprintf("%s_%d", prefix, i))
				}
			}
		}
	}
}

// preprocessArrayLiterals 预处理数组字面量，将其转换为函数调用
func (e *ExpressionEngine) preprocessArrayLiterals(expression string) string {
	// 将 [a, b, c] 转换为 __array__(a, b, c)
	// 这样Go解析器可以解析，然后我们在evaluateCallExpr中特殊处理
	
	result := expression
	
	// 简单的数组字面量匹配和替换（使用缓存）
	re, err := GetCompiledRegex(`\[([^\]]+)\]`)
	if err != nil {
		return result // 降级处理，返回原表达式
	}
	result = re.ReplaceAllStringFunc(result, func(match string) string {
		// 去掉方括号
		content := match[1 : len(match)-1]
		// 转换为函数调用形式
		return fmt.Sprintf("__array__(%s)", content)
	})
	
	return result
}

// preprocessTagsAccess 预处理标签访问语法
func (e *ExpressionEngine) preprocessTagsAccess(expression string) string {
	// 将 tags['key'] 或 tags["key"] 转换为 __tag_access__("key")
	// 将 tags.key 转换为 __tag_access__("key")
	
	result := expression
	
	// 处理 tags['key'] 和 tags["key"] 语法（使用缓存）
	re1, err := GetCompiledRegex(`tags\[['"]([^'"]+)['"]\]`)
	if err != nil {
		return result // 降级处理
	}
	result = re1.ReplaceAllStringFunc(result, func(match string) string {
		// 提取键名
		matches := re1.FindStringSubmatch(match)
		if len(matches) > 1 {
			return fmt.Sprintf(`__tag_access__("%s")`, matches[1])
		}
		return match
	})
	
	// 处理 tags.key 语法（使用缓存）
	re2, err := GetCompiledRegex(`tags\.([a-zA-Z_][a-zA-Z0-9_]*)`)
	if err != nil {
		return result // 降级处理
	}
	result = re2.ReplaceAllStringFunc(result, func(match string) string {
		// 提取键名
		matches := re2.FindStringSubmatch(match)
		if len(matches) > 1 {
			return fmt.Sprintf(`__tag_access__("%s")`, matches[1])
		}
		return match
	})
	
	return result
}

// normalizeSingleQuotes 将单引号字符串转换为双引号字符串
func (e *ExpressionEngine) normalizeSingleQuotes(expression string) string {
	// 简单的单引号到双引号转换
	// 这个实现假设单引号总是用于字符串字面量
	result := ""
	inSingleQuote := false
	escaped := false
	
	for _, char := range expression {
		if escaped {
			result += string(char)
			escaped = false
			continue
		}
		
		if char == '\\' {
			result += string(char)
			escaped = true
			continue
		}
		
		if char == '\'' {
			if !inSingleQuote {
				// 开始单引号字符串
				result += "\""
				inSingleQuote = true
			} else {
				// 结束单引号字符串
				result += "\""
				inSingleQuote = false
			}
		} else {
			result += string(char)
		}
	}
	
	return result
}

// evaluateGoExpression 使用Go解析器评估安全表达式
func (e *ExpressionEngine) evaluateGoExpression(expression string) (interface{}, error) {
	// 预处理：将单引号字符串转换为双引号字符串
	normalizedExpr := e.normalizeSingleQuotes(expression)
	
	// 预处理：处理标签访问语法
	tagsProcessedExpr := e.preprocessTagsAccess(normalizedExpr)
	
	// 预处理：处理数组字面量
	processedExpr := e.preprocessArrayLiterals(tagsProcessedExpr)
	
	// 仅支持安全的表达式类型，不允许函数调用和复杂语句
	expr, err := parser.ParseExpr(processedExpr)
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
		// 处理布尔字面量
		if n.Name == "true" {
			return true, nil
		}
		if n.Name == "false" {
			return false, nil
		}
		
		e.mu.RLock()
		value, exists := e.variables[n.Name]
		e.mu.RUnlock()
		if exists {
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
	case *ast.SelectorExpr:
		return e.evaluateSelectorExpr(n)
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

// applyBinaryOperator 应用二元操作符，包含优先级和结果验证
func (e *ExpressionEngine) applyBinaryOperator(left, right interface{}, op token.Token) (interface{}, error) {
	// 预处理：对于逻辑操作符，支持短路评估
	switch op {
	case token.LAND:
		// 短路评估：如果左操作数为false，直接返回false
		if !toBool(left) {
			return false, nil
		}
		return toBool(right), nil
	case token.LOR:
		// 短路评估：如果左操作数为true，直接返回true
		if toBool(left) {
			return true, nil
		}
		return toBool(right), nil
	}
	
	// 处理其他操作符
	switch op {
	case token.ADD:
		result, err := e.add(left, right)
		if err != nil {
			return nil, fmt.Errorf("加法操作失败: %v", err)
		}
		return result, nil
	case token.SUB:
		result, err := e.subtract(left, right)
		if err != nil {
			return nil, fmt.Errorf("减法操作失败: %v", err)
		}
		return result, nil
	case token.MUL:
		result, err := e.multiply(left, right)
		if err != nil {
			return nil, fmt.Errorf("乘法操作失败: %v", err)
		}
		return result, nil
	case token.QUO:
		result, err := e.divide(left, right)
		if err != nil {
			return nil, fmt.Errorf("除法操作失败: %v", err)
		}
		return result, nil
	case token.REM:
		result, err := e.modulo(left, right)
		if err != nil {
			return nil, fmt.Errorf("取模操作失败: %v", err)
		}
		return result, nil
	case token.EQL:
		return e.equal(left, right), nil
	case token.NEQ:
		return !e.equal(left, right), nil
	case token.LSS:
		result, err := e.less(left, right)
		if err != nil {
			return nil, fmt.Errorf("小于比较失败: %v", err)
		}
		return result, nil
	case token.GTR:
		result, err := e.greater(left, right)
		if err != nil {
			return nil, fmt.Errorf("大于比较失败: %v", err)
		}
		return result, nil
	case token.LEQ:
		result, err := e.lessEqual(left, right)
		if err != nil {
			return nil, fmt.Errorf("小于等于比较失败: %v", err)
		}
		return result, nil
	case token.GEQ:
		result, err := e.greaterEqual(left, right)
		if err != nil {
			return nil, fmt.Errorf("大于等于比较失败: %v", err)
		}
		return result, nil
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

// evaluateSelectorExpr 评估选择器表达式（如 acceleration.x）
func (e *ExpressionEngine) evaluateSelectorExpr(expr *ast.SelectorExpr) (interface{}, error) {
	// 构造完整的字段名，如 "acceleration.x"
	var baseName string
	if ident, ok := expr.X.(*ast.Ident); ok {
		baseName = ident.Name
	} else {
		return nil, fmt.Errorf("不支持的选择器基础类型: %T", expr.X)
	}
	
	fieldName := baseName + "." + expr.Sel.Name
	
	// 查找变量
	e.mu.RLock()
	value, exists := e.variables[fieldName]
	e.mu.RUnlock()
	
	if exists {
		return value, nil
	}
	
	// 提供更多诊断信息
	availableVars := make([]string, 0)
	for varName := range e.variables {
		availableVars = append(availableVars, varName)
	}
	
	if len(availableVars) == 0 {
		return nil, fmt.Errorf("未定义的字段: %s (当前无任何可用变量)", fieldName)
	}
	
	return nil, fmt.Errorf("未定义的字段: %s (可用变量: %v)", fieldName, availableVars)
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
	
	// 处理特殊函数
	if funcName == "__array__" {
		// 处理数组字面量
		var array []interface{}
		for _, arg := range expr.Args {
			value, err := e.evaluateASTNode(arg)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		return array, nil
	}
	
	if funcName == "__tag_access__" {
		// 处理标签访问
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("__tag_access__函数需要1个参数")
		}
		
		keyValue, err := e.evaluateASTNode(expr.Args[0])
		if err != nil {
			return nil, err
		}
		
		keyStr, ok := keyValue.(string)
		if !ok {
			return nil, fmt.Errorf("标签键必须是字符串")
		}
		
		// 从当前数据点的tags中获取值
		e.mu.RLock()
		if tags, exists := e.variables["tags"]; exists {
			if tagsMap, ok := tags.(map[string]string); ok {
				value, found := tagsMap[keyStr]
				e.mu.RUnlock()
				if found {
					return value, nil
				}
				return "", nil // 标签不存在时返回空字符串
			}
		}
		e.mu.RUnlock()
		return "", nil
	}
	
	// 查找普通函数
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
	e.mu.RLock()
	fieldValue, exists := e.variables[fieldName]
	e.mu.RUnlock()
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
	
	startHour, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("开始小时解析失败: %v", err)
	}
	
	endHour, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("结束小时解析失败: %v", err)
	}
	
	// 验证小时范围
	if startHour < 0 || startHour >= 24 || endHour < 0 || endHour >= 24 {
		return nil, fmt.Errorf("小时值必须在0-23范围内")
	}
	
	now := time.Now()
	currentHour := float64(now.Hour()) + float64(now.Minute())/60.0
	
	// 修正跨夜逻辑
	if startHour <= endHour {
		// 同一天内的时间范围
		if endHour == 24.0 {
			// 特殊处理24小时的情况
			return currentHour >= startHour, nil
		}
		return currentHour >= startHour && currentHour < endHour, nil
	} else {
		// 跨越午夜的情况（如 22:00 - 6:00）
		return currentHour >= startHour || currentHour < endHour, nil
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
	result := leftNum / rightNum
	
	// 检查结果是否为有效数值
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return nil, fmt.Errorf("除法运算产生无效结果")
	}
	
	return result, nil
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
	result := compareValues(left, right)
	if result == -2 { // NaN参与比较，根据IEEE 754标准返回false
		return false
	}
	return result == 0
}

func (e *ExpressionEngine) less(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 { // NaN参与比较，根据IEEE 754标准返回false
		return false, nil
	}
	return result < 0, nil
}

func (e *ExpressionEngine) greater(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 { // NaN参与比较，根据IEEE 754标准返回false
		return false, nil
	}
	return result > 0, nil
}

func (e *ExpressionEngine) lessEqual(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 { // NaN参与比较，根据IEEE 754标准返回false
		return false, nil
	}
	return result <= 0, nil
}

func (e *ExpressionEngine) greaterEqual(left, right interface{}) (bool, error) {
	result := compareValues(left, right)
	if result == -2 { // NaN参与比较，根据IEEE 754标准返回false
		return false, nil
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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.variables[name] = value
}

// initializeGeoData 初始化地理区域数据
func (e *ExpressionEngine) initializeGeoData() {
	// 添加一些常用的中国城市区域
	e.geoProcessor.AddRegion(&geo.Region{
		Name: "北京",
		Center: geo.Coordinate{
			Latitude:  39.9042,
			Longitude: 116.4074,
		},
		Radius: 50, // 50公里半径
	})
	
	e.geoProcessor.AddRegion(&geo.Region{
		Name: "上海",
		Center: geo.Coordinate{
			Latitude:  31.2304,
			Longitude: 121.4737,
		},
		Radius: 40,
	})
	
	e.geoProcessor.AddRegion(&geo.Region{
		Name: "深圳",
		Center: geo.Coordinate{
			Latitude:  22.5431,
			Longitude: 114.0579,
		},
		Radius: 30,
	})
	
	e.geoProcessor.AddRegion(&geo.Region{
		Name: "广州",
		Center: geo.Coordinate{
			Latitude:  23.1291,
			Longitude: 113.2644,
		},
		Radius: 35,
	})
	
	e.geoProcessor.AddRegion(&geo.Region{
		Name: "杭州",
		Center: geo.Coordinate{
			Latitude:  30.2741,
			Longitude: 120.1551,
		},
		Radius: 25,
	})
}

// registerBuiltinFunctions 注册内置函数
func (e *ExpressionEngine) registerBuiltinFunctions() {
	// 数学函数 - 使用优化版本
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
	
	// 数据检查函数
	e.RegisterFunction(&ExistsFunction{engine: e})
	
	// 时间函数
	e.RegisterFunction(&TimeNowFunction{})
	e.RegisterFunction(&TimeFormatFunction{})
	e.RegisterFunction(&TimeDiffFunction{})
	e.RegisterFunction(&TimeRangeFunction{})
	
	// 类型转换函数
	e.RegisterFunction(&ConvertToStringFunction{})
	e.RegisterFunction(&ConvertToNumberFunction{})
	e.RegisterFunction(&ConvertToBoolFunction{})
	
	// 统计函数
	e.RegisterFunction(&AvgFunction{})
	e.RegisterFunction(&StddevFunction{})
	
	// 数据质量检测函数
	e.RegisterFunction(&IsNaNFunction{})
	e.RegisterFunction(&IsInfFunction{})
	e.RegisterFunction(&IsFiniteFunction{})
	
	// 模式匹配函数
	e.RegisterFunction(&RegexFunction{})
	
	// 地理数据处理函数
	if e.geoProcessor != nil {
		e.RegisterFunction(geo.NewDistanceFunction(e.geoProcessor))
		e.RegisterFunction(geo.NewInRegionFunction(e.geoProcessor))
		e.RegisterFunction(geo.NewNearestRegionFunction(e.geoProcessor))
		e.RegisterFunction(geo.NewBearingFunction(e.geoProcessor))
		e.RegisterFunction(geo.NewValidCoordinateFunction(e.geoProcessor))
	}
	
	// 向量函数
	e.RegisterFunction(&VectorMagnitudeFunction{})
	e.RegisterFunction(&VectorDotProductFunction{})
	e.RegisterFunction(&VectorCrossProductFunction{})
	e.RegisterFunction(&VectorNormalizeFunction{})
	e.RegisterFunction(&VectorAngleFunction{})
	e.RegisterFunction(&VectorDistanceFunction{})
	
	// 通用复合数据函数
	e.RegisterFunction(&GenericVectorMagnitudeFunction{})
	e.RegisterFunction(&GenericVectorSumFunction{})
	e.RegisterFunction(&GenericVectorMeanFunction{})
	e.RegisterFunction(&GenericVectorMinFunction{})
	e.RegisterFunction(&GenericVectorMaxFunction{})
	e.RegisterFunction(&GenericVectorDotProductFunction{})
	e.RegisterFunction(&GenericVectorNormalizeFunction{})
	
	// 数组函数
	e.RegisterFunction(&ArrayLengthFunction{})
	e.RegisterFunction(&ArraySumFunction{})
	e.RegisterFunction(&ArrayMeanFunction{})
	e.RegisterFunction(&ArrayMinFunction{})
	e.RegisterFunction(&ArrayMaxFunction{})
	e.RegisterFunction(&ArrayCountFunction{})
	e.RegisterFunction(&ArrayGetFunction{})
	
	// 矩阵函数
	e.RegisterFunction(&MatrixTraceFunction{})
	e.RegisterFunction(&MatrixDeterminantFunction{})
	e.RegisterFunction(&MatrixSumFunction{})
	e.RegisterFunction(&MatrixMeanFunction{})
	e.RegisterFunction(&MatrixGetFunction{})
	
	// 时间序列函数
	e.RegisterFunction(&TimeSeriesLengthFunction{})
	e.RegisterFunction(&TimeSeriesMeanFunction{})
	e.RegisterFunction(&TimeSeriesMinFunction{})
	e.RegisterFunction(&TimeSeriesMaxFunction{})
	e.RegisterFunction(&TimeSeriesTrendFunction{})
	e.RegisterFunction(&TimeSeriesVarianceFunction{})
	e.RegisterFunction(&TimeSeriesStdDevFunction{})
	
	// Vector3D专用函数
	e.RegisterFunction(&Vector3DMagnitudeFunction{})
	e.RegisterFunction(&Vector3DDotProductFunction{})
	e.RegisterFunction(&Vector3DCrossProductFunction{})
	
	// 通用复合数据实用函数
	e.RegisterFunction(&CompositeDataTypeFunction{})
	e.RegisterFunction(&CompositeDataSizeFunction{})
	e.RegisterFunction(&CompositeDataValidateFunction{})
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
	if len(args) < 1 {
		return nil, fmt.Errorf("max函数需要至少1个参数")
	}
	
	// 如果第一个参数是数组，计算数组元素的最大值
	if arr, ok := args[0].([]interface{}); ok {
		if len(arr) == 0 {
			return nil, fmt.Errorf("max函数不能处理空数组")
		}
		
		maxVal, ok := toFloat64(arr[0])
		if !ok {
			return nil, fmt.Errorf("max函数数组元素必须是数值")
		}
		
		for i := 1; i < len(arr); i++ {
			if num, ok := toFloat64(arr[i]); ok {
				maxVal = math.Max(maxVal, num)
			} else {
				return nil, fmt.Errorf("max函数数组第%d个元素必须是数值", i+1)
			}
		}
		return maxVal, nil
	}
	
	// 否则至少需要2个参数
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
	if len(args) < 1 {
		return nil, fmt.Errorf("min函数需要至少1个参数")
	}
	
	// 如果第一个参数是数组，计算数组元素的最小值
	if arr, ok := args[0].([]interface{}); ok {
		if len(arr) == 0 {
			return nil, fmt.Errorf("min函数不能处理空数组")
		}
		
		minVal, ok := toFloat64(arr[0])
		if !ok {
			return nil, fmt.Errorf("min函数数组元素必须是数值")
		}
		
		for i := 1; i < len(arr); i++ {
			if num, ok := toFloat64(arr[i]); ok {
				minVal = math.Min(minVal, num)
			} else {
				return nil, fmt.Errorf("min函数数组第%d个元素必须是数值", i+1)
			}
		}
		return minVal, nil
	}
	
	// 否则至少需要2个参数
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

// 数据检查函数
type ExistsFunction struct {
	engine *ExpressionEngine // 引用表达式引擎以访问变量
}
func (f *ExistsFunction) Name() string { return "exists" }
func (f *ExistsFunction) Description() string { return "检查指定字段是否存在" }
func (f *ExistsFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("exists函数需要1个参数")
	}
	
	// 特殊处理：如果参数是一个map（如tags），检查是否非空
	if tagMap, ok := args[0].(map[string]string); ok {
		return len(tagMap) > 0, nil
	}
	
	fieldName := fmt.Sprintf("%v", args[0])
	
	// 检查当前数据点中是否存在该字段
	if f.engine != nil {
		f.engine.mu.RLock()
		_, exists := f.engine.variables[fieldName]
		f.engine.mu.RUnlock()
		if exists {
			return true, nil
		}
		
		// 检查是否是当前数据点的key
		if currentKey, ok := f.engine.variables["key"]; ok && currentKey == fieldName {
			return true, nil
		}
	}
	
	// 简单实现：检查常见字段名
	commonFields := []string{"temperature", "humidity", "pressure", "voltage", "current", "power", "rotation_speed", "vibration"}
	for _, field := range commonFields {
		if field == fieldName {
			return true, nil
		}
	}
	
	return false, nil
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
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0.0
	case float64:
		return v != 0.0
	}
	return value != nil
}

// 时间范围函数
type TimeRangeFunction struct{}
func (f *TimeRangeFunction) Name() string { return "time_range" }
func (f *TimeRangeFunction) Description() string { return "检查当前时间是否在指定小时范围内" }
func (f *TimeRangeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("time_range函数需要2个参数")
	}
	
	startHour, ok1 := toFloat64(args[0])
	endHour, ok2 := toFloat64(args[1])
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("time_range参数必须是数值")
	}
	
	// 验证小时范围（允许24表示一天结束）
	if startHour < 0 || startHour >= 24 || endHour < 0 || endHour > 24 {
		return nil, fmt.Errorf("小时值必须在0-24范围内")
	}
	
	now := time.Now()
	currentHour := float64(now.Hour()) + float64(now.Minute())/60.0
	
	// 处理跨夜情况的修正逻辑
	if startHour <= endHour {
		// 同一天内的时间范围，如 9:00 - 17:00
		if endHour == 24.0 {
			// 特殊处理24小时的情况
			return currentHour >= startHour, nil
		}
		return currentHour >= startHour && currentHour < endHour, nil
	} else {
		// 跨越午夜的情况，如 22:00 - 6:00
		// 当前时间在开始时间之后（今天晚上）或在结束时间之前（明天早上）
		return currentHour >= startHour || currentHour < endHour, nil
	}
}

// 平均值函数
type AvgFunction struct{}
func (f *AvgFunction) Name() string { return "avg" }
func (f *AvgFunction) Description() string { return "计算数组的平均值" }
func (f *AvgFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("avg函数需要至少1个参数")
	}
	
	// 如果第一个参数是数组，计算数组元素的平均值
	if arr, ok := args[0].([]interface{}); ok {
		if len(arr) == 0 {
			return 0.0, nil
		}
		
		var sum float64
		count := 0
		for _, v := range arr {
			if num, ok := toFloat64(v); ok {
				sum += num
				count++
			}
		}
		
		if count == 0 {
			return 0.0, nil
		}
		return sum / float64(count), nil
	}
	
	// 否则计算所有参数的平均值
	var sum float64
	count := 0
	for _, arg := range args {
		if num, ok := toFloat64(arg); ok {
			sum += num
			count++
		}
	}
	
	if count == 0 {
		return 0.0, nil
	}
	return sum / float64(count), nil
}

// 标准差函数
type StddevFunction struct{}
func (f *StddevFunction) Name() string { return "stddev" }
func (f *StddevFunction) Description() string { return "计算数组的标准差" }
func (f *StddevFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("stddev函数需要至少1个参数")
	}
	
	var values []float64
	
	// 如果第一个参数是数组，使用数组元素
	if arr, ok := args[0].([]interface{}); ok {
		for _, v := range arr {
			if num, ok := toFloat64(v); ok {
				values = append(values, num)
			}
		}
	} else {
		// 否则使用所有参数
		for _, arg := range args {
			if num, ok := toFloat64(arg); ok {
				values = append(values, num)
			}
		}
	}
	
	if len(values) == 0 {
		return 0.0, nil
	}
	
	// 计算平均值
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	
	// 计算方差
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))
	
	// 返回标准差
	return math.Sqrt(variance), nil
}

// 正则表达式匹配函数
type RegexFunction struct{}
func (f *RegexFunction) Name() string { return "regex" }
func (f *RegexFunction) Description() string { return "使用正则表达式匹配字符串" }
func (f *RegexFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("regex函数需要2个参数")
	}
	
	pattern, ok1 := args[0].(string)
	text, ok2 := args[1].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("regex函数的参数必须是字符串")
	}
	
	matched, err := regexp.MatchString(pattern, text)
	if err != nil {
		return nil, fmt.Errorf("正则表达式错误: %v", err)
	}
	
	return matched, nil
}

// 数据质量检测函数
type IsNaNFunction struct{}
func (f *IsNaNFunction) Name() string { return "isNaN" }
func (f *IsNaNFunction) Description() string { return "检查数值是否为NaN(非数字)" }
func (f *IsNaNFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isNaN函数需要1个参数")
	}
	
	switch v := args[0].(type) {
	case float64:
		return math.IsNaN(v), nil
	case float32:
		return math.IsNaN(float64(v)), nil
	default:
		// 非数值类型不是NaN
		return false, nil
	}
}

type IsInfFunction struct{}
func (f *IsInfFunction) Name() string { return "isInf" }
func (f *IsInfFunction) Description() string { return "检查数值是否为无穷大" }
func (f *IsInfFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isInf函数需要1个参数")
	}
	
	switch v := args[0].(type) {
	case float64:
		return math.IsInf(v, 0), nil
	case float32:
		return math.IsInf(float64(v), 0), nil
	default:
		// 非数值类型不是无穷大
		return false, nil
	}
}

type IsFiniteFunction struct{}
func (f *IsFiniteFunction) Name() string { return "isFinite" }
func (f *IsFiniteFunction) Description() string { return "检查数值是否为有限数" }
func (f *IsFiniteFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("isFinite函数需要1个参数")
	}
	
	switch v := args[0].(type) {
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0), nil
	case float32:
		f64 := float64(v)
		return !math.IsNaN(f64) && !math.IsInf(f64, 0), nil
	default:
		// 非数值类型按有限数处理
		return true, nil
	}
}

// 向量函数实现

// VectorMagnitudeFunction 计算向量模长
type VectorMagnitudeFunction struct{}
func (f *VectorMagnitudeFunction) Name() string { return "vectorMagnitude" }
func (f *VectorMagnitudeFunction) Description() string { return "计算3D向量的模长/大小" }
func (f *VectorMagnitudeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) == 1 {
		// 单参数：Vector3D对象
		if vector, ok := args[0].(*model.Vector3D); ok {
			return math.Sqrt(vector.X*vector.X + vector.Y*vector.Y + vector.Z*vector.Z), nil
		}
		return nil, fmt.Errorf("参数必须是Vector3D类型")
	} else if len(args) == 3 {
		// 三参数：x, y, z坐标
		x, ok1 := toFloat64(args[0])
		y, ok2 := toFloat64(args[1])
		z, ok3 := toFloat64(args[2])
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("向量坐标必须是数值")
		}
		return math.Sqrt(x*x + y*y + z*z), nil
	}
	return nil, fmt.Errorf("vectorMagnitude函数需要1个Vector3D参数或3个数值参数")
}

// VectorDotProductFunction 计算向量点积
type VectorDotProductFunction struct{}
func (f *VectorDotProductFunction) Name() string { return "vectorDot" }
func (f *VectorDotProductFunction) Description() string { return "计算两个3D向量的点积" }
func (f *VectorDotProductFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) == 2 {
		// 两个Vector3D对象
		v1, ok1 := args[0].(*model.Vector3D)
		v2, ok2 := args[1].(*model.Vector3D)
		if ok1 && ok2 {
			return v1.X*v2.X + v1.Y*v2.Y + v1.Z*v2.Z, nil
		}
	} else if len(args) == 6 {
		// 六个坐标参数：x1, y1, z1, x2, y2, z2
		coords := make([]float64, 6)
		for i, arg := range args {
			val, ok := toFloat64(arg)
			if !ok {
				return nil, fmt.Errorf("向量坐标必须是数值")
			}
			coords[i] = val
		}
		return coords[0]*coords[3] + coords[1]*coords[4] + coords[2]*coords[5], nil
	}
	return nil, fmt.Errorf("vectorDot函数需要2个Vector3D参数或6个数值参数")
}

// VectorCrossProductFunction 计算向量叉积
type VectorCrossProductFunction struct{}
func (f *VectorCrossProductFunction) Name() string { return "vectorCross" }
func (f *VectorCrossProductFunction) Description() string { return "计算两个3D向量的叉积，返回新向量" }
func (f *VectorCrossProductFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) == 2 {
		// 两个Vector3D对象
		v1, ok1 := args[0].(*model.Vector3D)
		v2, ok2 := args[1].(*model.Vector3D)
		if ok1 && ok2 {
			return &model.Vector3D{
				X: v1.Y*v2.Z - v1.Z*v2.Y,
				Y: v1.Z*v2.X - v1.X*v2.Z,
				Z: v1.X*v2.Y - v1.Y*v2.X,
			}, nil
		}
	} else if len(args) == 6 {
		// 六个坐标参数
		coords := make([]float64, 6)
		for i, arg := range args {
			val, ok := toFloat64(arg)
			if !ok {
				return nil, fmt.Errorf("向量坐标必须是数值")
			}
			coords[i] = val
		}
		return &model.Vector3D{
			X: coords[1]*coords[5] - coords[2]*coords[4],
			Y: coords[2]*coords[3] - coords[0]*coords[5],
			Z: coords[0]*coords[4] - coords[1]*coords[3],
		}, nil
	}
	return nil, fmt.Errorf("vectorCross函数需要2个Vector3D参数或6个数值参数")
}

// VectorNormalizeFunction 向量归一化
type VectorNormalizeFunction struct{}
func (f *VectorNormalizeFunction) Name() string { return "vectorNormalize" }
func (f *VectorNormalizeFunction) Description() string { return "将3D向量归一化为单位向量" }
func (f *VectorNormalizeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) == 1 {
		// 单参数：Vector3D对象
		if vector, ok := args[0].(*model.Vector3D); ok {
			magnitude := math.Sqrt(vector.X*vector.X + vector.Y*vector.Y + vector.Z*vector.Z)
			if magnitude == 0 {
				return &model.Vector3D{X: 0, Y: 0, Z: 0}, nil
			}
			return &model.Vector3D{
				X: vector.X / magnitude,
				Y: vector.Y / magnitude,
				Z: vector.Z / magnitude,
			}, nil
		}
		return nil, fmt.Errorf("参数必须是Vector3D类型")
	} else if len(args) == 3 {
		// 三参数：x, y, z坐标
		x, ok1 := toFloat64(args[0])
		y, ok2 := toFloat64(args[1])
		z, ok3 := toFloat64(args[2])
		if !ok1 || !ok2 || !ok3 {
			return nil, fmt.Errorf("向量坐标必须是数值")
		}
		magnitude := math.Sqrt(x*x + y*y + z*z)
		if magnitude == 0 {
			return &model.Vector3D{X: 0, Y: 0, Z: 0}, nil
		}
		return &model.Vector3D{
			X: x / magnitude,
			Y: y / magnitude,
			Z: z / magnitude,
		}, nil
	}
	return nil, fmt.Errorf("vectorNormalize函数需要1个Vector3D参数或3个数值参数")
}

// VectorAngleFunction 计算两向量夹角（弧度）
type VectorAngleFunction struct{}
func (f *VectorAngleFunction) Name() string { return "vectorAngle" }
func (f *VectorAngleFunction) Description() string { return "计算两个3D向量的夹角（弧度）" }
func (f *VectorAngleFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) == 2 {
		// 两个Vector3D对象
		v1, ok1 := args[0].(*model.Vector3D)
		v2, ok2 := args[1].(*model.Vector3D)
		if ok1 && ok2 {
			// 计算点积
			dot := v1.X*v2.X + v1.Y*v2.Y + v1.Z*v2.Z
			// 计算模长
			mag1 := math.Sqrt(v1.X*v1.X + v1.Y*v1.Y + v1.Z*v1.Z)
			mag2 := math.Sqrt(v2.X*v2.X + v2.Y*v2.Y + v2.Z*v2.Z)
			
			if mag1 == 0 || mag2 == 0 {
				return 0.0, nil // 零向量与任何向量夹角为0
			}
			
			cosAngle := dot / (mag1 * mag2)
			// 处理浮点精度问题
			if cosAngle > 1.0 {
				cosAngle = 1.0
			} else if cosAngle < -1.0 {
				cosAngle = -1.0
			}
			
			return math.Acos(cosAngle), nil
		}
	} else if len(args) == 6 {
		// 六个坐标参数
		coords := make([]float64, 6)
		for i, arg := range args {
			val, ok := toFloat64(arg)
			if !ok {
				return nil, fmt.Errorf("向量坐标必须是数值")
			}
			coords[i] = val
		}
		
		// 计算点积
		dot := coords[0]*coords[3] + coords[1]*coords[4] + coords[2]*coords[5]
		// 计算模长
		mag1 := math.Sqrt(coords[0]*coords[0] + coords[1]*coords[1] + coords[2]*coords[2])
		mag2 := math.Sqrt(coords[3]*coords[3] + coords[4]*coords[4] + coords[5]*coords[5])
		
		if mag1 == 0 || mag2 == 0 {
			return 0.0, nil
		}
		
		cosAngle := dot / (mag1 * mag2)
		if cosAngle > 1.0 {
			cosAngle = 1.0
		} else if cosAngle < -1.0 {
			cosAngle = -1.0
		}
		
		return math.Acos(cosAngle), nil
	}
	return nil, fmt.Errorf("vectorAngle函数需要2个Vector3D参数或6个数值参数")
}

// VectorDistanceFunction 计算两点间距离
type VectorDistanceFunction struct{}
func (f *VectorDistanceFunction) Name() string { return "vectorDistance" }
func (f *VectorDistanceFunction) Description() string { return "计算两个3D点之间的欧几里得距离" }
func (f *VectorDistanceFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) == 2 {
		// 两个Vector3D对象
		v1, ok1 := args[0].(*model.Vector3D)
		v2, ok2 := args[1].(*model.Vector3D)
		if ok1 && ok2 {
			dx := v1.X - v2.X
			dy := v1.Y - v2.Y
			dz := v1.Z - v2.Z
			return math.Sqrt(dx*dx + dy*dy + dz*dz), nil
		}
	} else if len(args) == 6 {
		// 六个坐标参数
		coords := make([]float64, 6)
		for i, arg := range args {
			val, ok := toFloat64(arg)
			if !ok {
				return nil, fmt.Errorf("坐标必须是数值")
			}
			coords[i] = val
		}
		
		dx := coords[0] - coords[3]
		dy := coords[1] - coords[4]
		dz := coords[2] - coords[5]
		return math.Sqrt(dx*dx + dy*dy + dz*dz), nil
	}
	return nil, fmt.Errorf("vectorDistance函数需要2个Vector3D参数或6个数值参数")
}

// =======================
// 通用复合数据函数实现
// =======================

// GenericVectorMagnitudeFunction 通用向量模长计算
type GenericVectorMagnitudeFunction struct{}
func (f *GenericVectorMagnitudeFunction) Name() string { return "genericVectorMagnitude" }
func (f *GenericVectorMagnitudeFunction) Description() string { return "计算通用向量的模长" }
func (f *GenericVectorMagnitudeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorMagnitude函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.VectorData); ok {
		if len(vector.Values) == 0 {
			return 0.0, nil
		}
		
		sumSquares := 0.0
		for _, val := range vector.Values {
			sumSquares += val * val
		}
		return math.Sqrt(sumSquares), nil
	}
	
	return nil, fmt.Errorf("参数必须是VectorData类型")
}

// GenericVectorSumFunction 通用向量元素求和
type GenericVectorSumFunction struct{}
func (f *GenericVectorSumFunction) Name() string { return "vectorSum" }
func (f *GenericVectorSumFunction) Description() string { return "计算通用向量所有元素的和" }
func (f *GenericVectorSumFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorSum函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.VectorData); ok {
		sum := 0.0
		for _, val := range vector.Values {
			sum += val
		}
		return sum, nil
	}
	
	return nil, fmt.Errorf("参数必须是VectorData类型")
}

// GenericVectorMeanFunction 通用向量均值计算
type GenericVectorMeanFunction struct{}
func (f *GenericVectorMeanFunction) Name() string { return "vectorMean" }
func (f *GenericVectorMeanFunction) Description() string { return "计算通用向量元素的平均值" }
func (f *GenericVectorMeanFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorMean函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.VectorData); ok {
		if len(vector.Values) == 0 {
			return 0.0, nil
		}
		
		sum := 0.0
		for _, val := range vector.Values {
			sum += val
		}
		return sum / float64(len(vector.Values)), nil
	}
	
	return nil, fmt.Errorf("参数必须是VectorData类型")
}

// GenericVectorMinFunction 通用向量最小值
type GenericVectorMinFunction struct{}
func (f *GenericVectorMinFunction) Name() string { return "vectorMin" }
func (f *GenericVectorMinFunction) Description() string { return "查找通用向量中的最小值" }
func (f *GenericVectorMinFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorMin函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.VectorData); ok {
		if len(vector.Values) == 0 {
			return nil, fmt.Errorf("向量为空")
		}
		
		min := vector.Values[0]
		for _, val := range vector.Values[1:] {
			if val < min {
				min = val
			}
		}
		return min, nil
	}
	
	return nil, fmt.Errorf("参数必须是VectorData类型")
}

// GenericVectorMaxFunction 通用向量最大值
type GenericVectorMaxFunction struct{}
func (f *GenericVectorMaxFunction) Name() string { return "vectorMax" }
func (f *GenericVectorMaxFunction) Description() string { return "查找通用向量中的最大值" }
func (f *GenericVectorMaxFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorMax函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.VectorData); ok {
		if len(vector.Values) == 0 {
			return nil, fmt.Errorf("向量为空")
		}
		
		max := vector.Values[0]
		for _, val := range vector.Values[1:] {
			if val > max {
				max = val
			}
		}
		return max, nil
	}
	
	return nil, fmt.Errorf("参数必须是VectorData类型")
}

// GenericVectorDotProductFunction 通用向量点积
type GenericVectorDotProductFunction struct{}
func (f *GenericVectorDotProductFunction) Name() string { return "vectorDotProduct" }
func (f *GenericVectorDotProductFunction) Description() string { return "计算两个通用向量的点积" }
func (f *GenericVectorDotProductFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("vectorDotProduct函数需要2个参数")
	}
	
	v1, ok1 := args[0].(*model.VectorData)
	v2, ok2 := args[1].(*model.VectorData)
	
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("参数必须都是VectorData类型")
	}
	
	if len(v1.Values) != len(v2.Values) {
		return nil, fmt.Errorf("向量维度不匹配: %d vs %d", len(v1.Values), len(v2.Values))
	}
	
	if len(v1.Values) == 0 {
		return 0.0, nil
	}
	
	dotProduct := 0.0
	for i := 0; i < len(v1.Values); i++ {
		dotProduct += v1.Values[i] * v2.Values[i]
	}
	
	return dotProduct, nil
}

// GenericVectorNormalizeFunction 通用向量归一化
type GenericVectorNormalizeFunction struct{}
func (f *GenericVectorNormalizeFunction) Name() string { return "vectorNormalize" }
func (f *GenericVectorNormalizeFunction) Description() string { return "将通用向量归一化为单位向量" }
func (f *GenericVectorNormalizeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorNormalize函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.VectorData); ok {
		if len(vector.Values) == 0 {
			return nil, fmt.Errorf("向量为空")
		}
		
		// 计算模长
		sumSquares := 0.0
		for _, val := range vector.Values {
			sumSquares += val * val
		}
		magnitude := math.Sqrt(sumSquares)
		
		if magnitude == 0 {
			// 零向量保持不变
			normalizedValues := make([]float64, len(vector.Values))
			return &model.VectorData{
				Values:    normalizedValues,
				Dimension: vector.Dimension,
				Labels:    vector.Labels,
				Unit:      vector.Unit,
			}, nil
		}
		
		// 归一化
		normalizedValues := make([]float64, len(vector.Values))
		for i, val := range vector.Values {
			normalizedValues[i] = val / magnitude
		}
		
		return &model.VectorData{
			Values:    normalizedValues,
			Dimension: len(normalizedValues),
			Labels:    vector.Labels,
			Unit:      vector.Unit,
		}, nil
	}
	
	return nil, fmt.Errorf("参数必须是VectorData类型")
}

// =======================
// 数组函数实现
// =======================

// ArrayLengthFunction 数组长度
type ArrayLengthFunction struct{}
func (f *ArrayLengthFunction) Name() string { return "arrayLength" }
func (f *ArrayLengthFunction) Description() string { return "获取数组的长度" }
func (f *ArrayLengthFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("arrayLength函数需要1个参数")
	}
	
	if array, ok := args[0].(*model.ArrayData); ok {
		return len(array.Values), nil
	}
	
	return nil, fmt.Errorf("参数必须是ArrayData类型")
}

// ArraySumFunction 数组数值元素求和
type ArraySumFunction struct{}
func (f *ArraySumFunction) Name() string { return "arraySum" }
func (f *ArraySumFunction) Description() string { return "计算数组中数值元素的和" }
func (f *ArraySumFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("arraySum函数需要1个参数")
	}
	
	if array, ok := args[0].(*model.ArrayData); ok {
		sum := 0.0
		count := 0
		
		for _, val := range array.Values {
			if num, ok := toFloat64(val); ok {
				sum += num
				count++
			}
		}
		
		if count == 0 {
			return nil, fmt.Errorf("数组中没有数值类型元素")
		}
		
		return sum, nil
	}
	
	return nil, fmt.Errorf("参数必须是ArrayData类型")
}

// ArrayMeanFunction 数组数值元素平均值
type ArrayMeanFunction struct{}
func (f *ArrayMeanFunction) Name() string { return "arrayMean" }
func (f *ArrayMeanFunction) Description() string { return "计算数组中数值元素的平均值" }
func (f *ArrayMeanFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("arrayMean函数需要1个参数")
	}
	
	if array, ok := args[0].(*model.ArrayData); ok {
		sum := 0.0
		count := 0
		
		for _, val := range array.Values {
			if num, ok := toFloat64(val); ok {
				sum += num
				count++
			}
		}
		
		if count == 0 {
			return nil, fmt.Errorf("数组中没有数值类型元素")
		}
		
		return sum / float64(count), nil
	}
	
	return nil, fmt.Errorf("参数必须是ArrayData类型")
}

// ArrayMinFunction 数组数值元素最小值
type ArrayMinFunction struct{}
func (f *ArrayMinFunction) Name() string { return "arrayMin" }
func (f *ArrayMinFunction) Description() string { return "查找数组中数值元素的最小值" }
func (f *ArrayMinFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("arrayMin函数需要1个参数")
	}
	
	if array, ok := args[0].(*model.ArrayData); ok {
		var min *float64
		
		for _, val := range array.Values {
			if num, ok := toFloat64(val); ok {
				if min == nil || num < *min {
					min = &num
				}
			}
		}
		
		if min == nil {
			return nil, fmt.Errorf("数组中没有数值类型元素")
		}
		
		return *min, nil
	}
	
	return nil, fmt.Errorf("参数必须是ArrayData类型")
}

// ArrayMaxFunction 数组数值元素最大值
type ArrayMaxFunction struct{}
func (f *ArrayMaxFunction) Name() string { return "arrayMax" }
func (f *ArrayMaxFunction) Description() string { return "查找数组中数值元素的最大值" }
func (f *ArrayMaxFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("arrayMax函数需要1个参数")
	}
	
	if array, ok := args[0].(*model.ArrayData); ok {
		var max *float64
		
		for _, val := range array.Values {
			if num, ok := toFloat64(val); ok {
				if max == nil || num > *max {
					max = &num
				}
			}
		}
		
		if max == nil {
			return nil, fmt.Errorf("数组中没有数值类型元素")
		}
		
		return *max, nil
	}
	
	return nil, fmt.Errorf("参数必须是ArrayData类型")
}

// ArrayCountFunction 数组元素计数
type ArrayCountFunction struct{}
func (f *ArrayCountFunction) Name() string { return "arrayCount" }
func (f *ArrayCountFunction) Description() string { return "计算数组中满足条件的元素数量" }
func (f *ArrayCountFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("arrayCount函数需要1-2个参数")
	}
	
	if array, ok := args[0].(*model.ArrayData); ok {
		if len(args) == 1 {
			// 只计算非空元素
			count := 0
			for _, val := range array.Values {
				if val != nil {
					count++
				}
			}
			return count, nil
		} else {
			// 计算等于指定值的元素
			target := args[1]
			count := 0
			for _, val := range array.Values {
				if compareValues(val, target) == 0 {
					count++
				}
			}
			return count, nil
		}
	}
	
	return nil, fmt.Errorf("参数必须是ArrayData类型")
}

// ArrayGetFunction 获取数组指定位置的元素
type ArrayGetFunction struct{}
func (f *ArrayGetFunction) Name() string { return "arrayGet" }
func (f *ArrayGetFunction) Description() string { return "获取数组指定索引位置的元素" }
func (f *ArrayGetFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("arrayGet函数需要2个参数")
	}
	
	array, ok1 := args[0].(*model.ArrayData)
	if !ok1 {
		return nil, fmt.Errorf("第一个参数必须是ArrayData类型")
	}
	
	index, ok := toFloat64(args[1])
	if !ok {
		return nil, fmt.Errorf("第二个参数必须是数值类型")
	}
	
	idx := int(index)
	if idx < 0 || idx >= len(array.Values) {
		return nil, fmt.Errorf("索引超出范围: %d", idx)
	}
	
	return array.Values[idx], nil
}

// =======================
// 矩阵函数实现
// =======================

// MatrixTraceFunction 矩阵迹计算
type MatrixTraceFunction struct{}
func (f *MatrixTraceFunction) Name() string { return "matrixTrace" }
func (f *MatrixTraceFunction) Description() string { return "计算方阵的迹（对角线元素之和）" }
func (f *MatrixTraceFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("matrixTrace函数需要1个参数")
	}
	
	if matrix, ok := args[0].(*model.MatrixData); ok {
		if matrix.Rows != matrix.Cols {
			return nil, fmt.Errorf("只有方阵才能计算迹")
		}
		
		trace := 0.0
		for i := 0; i < matrix.Rows; i++ {
			trace += matrix.Values[i][i]
		}
		
		return trace, nil
	}
	
	return nil, fmt.Errorf("参数必须是MatrixData类型")
}

// MatrixDeterminantFunction 矩阵行列式计算（仅支持小规模矩阵）
type MatrixDeterminantFunction struct{}
func (f *MatrixDeterminantFunction) Name() string { return "matrixDeterminant" }
func (f *MatrixDeterminantFunction) Description() string { return "计算小规模方阵的行列式" }
func (f *MatrixDeterminantFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("matrixDeterminant函数需要1个参数")
	}
	
	if matrix, ok := args[0].(*model.MatrixData); ok {
		if matrix.Rows != matrix.Cols {
			return nil, fmt.Errorf("只有方阵才能计算行列式")
		}
		
		if matrix.Rows > 3 {
			return nil, fmt.Errorf("仅支持3x3及以下矩阵的行列式计算")
		}
		
		switch matrix.Rows {
		case 1:
			return matrix.Values[0][0], nil
		case 2:
			return matrix.Values[0][0]*matrix.Values[1][1] - matrix.Values[0][1]*matrix.Values[1][0], nil
		case 3:
			a := matrix.Values[0][0] * (matrix.Values[1][1]*matrix.Values[2][2] - matrix.Values[1][2]*matrix.Values[2][1])
			b := matrix.Values[0][1] * (matrix.Values[1][0]*matrix.Values[2][2] - matrix.Values[1][2]*matrix.Values[2][0])
			c := matrix.Values[0][2] * (matrix.Values[1][0]*matrix.Values[2][1] - matrix.Values[1][1]*matrix.Values[2][0])
			return a - b + c, nil
		default:
			return nil, fmt.Errorf("不支持的矩阵大小")
		}
	}
	
	return nil, fmt.Errorf("参数必须是MatrixData类型")
}

// MatrixSumFunction 矩阵元素求和
type MatrixSumFunction struct{}
func (f *MatrixSumFunction) Name() string { return "matrixSum" }
func (f *MatrixSumFunction) Description() string { return "计算矩阵所有元素的和" }
func (f *MatrixSumFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("matrixSum函数需要1个参数")
	}
	
	if matrix, ok := args[0].(*model.MatrixData); ok {
		sum := 0.0
		for _, row := range matrix.Values {
			for _, val := range row {
				sum += val
			}
		}
		return sum, nil
	}
	
	return nil, fmt.Errorf("参数必须是MatrixData类型")
}

// MatrixMeanFunction 矩阵元素平均值
type MatrixMeanFunction struct{}
func (f *MatrixMeanFunction) Name() string { return "matrixMean" }
func (f *MatrixMeanFunction) Description() string { return "计算矩阵所有元素的平均值" }
func (f *MatrixMeanFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("matrixMean函数需要1个参数")
	}
	
	if matrix, ok := args[0].(*model.MatrixData); ok {
		sum := 0.0
		count := 0
		for _, row := range matrix.Values {
			for _, val := range row {
				sum += val
				count++
			}
		}
		
		if count == 0 {
			return 0.0, nil
		}
		
		return sum / float64(count), nil
	}
	
	return nil, fmt.Errorf("参数必须是MatrixData类型")
}

// MatrixGetFunction 获取矩阵指定位置的元素
type MatrixGetFunction struct{}
func (f *MatrixGetFunction) Name() string { return "matrixGet" }
func (f *MatrixGetFunction) Description() string { return "获取矩阵指定行列位置的元素" }
func (f *MatrixGetFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("matrixGet函数需要3个参数")
	}
	
	matrix, ok1 := args[0].(*model.MatrixData)
	if !ok1 {
		return nil, fmt.Errorf("第一个参数必须是MatrixData类型")
	}
	
	row, ok1 := toFloat64(args[1])
	col, ok2 := toFloat64(args[2])
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("行列索引必须是数值类型")
	}
	
	r, c := int(row), int(col)
	if r < 0 || r >= matrix.Rows || c < 0 || c >= matrix.Cols {
		return nil, fmt.Errorf("索引超出范围: [%d,%d]", r, c)
	}
	
	return matrix.Values[r][c], nil
}

// =======================
// 时间序列函数实现
// =======================

// TimeSeriesLengthFunction 时间序列长度
type TimeSeriesLengthFunction struct{}
func (f *TimeSeriesLengthFunction) Name() string { return "timeSeriesLength" }
func (f *TimeSeriesLengthFunction) Description() string { return "获取时间序列的长度" }
func (f *TimeSeriesLengthFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesLength函数需要1个参数")
	}
	
	if ts, ok := args[0].(*model.TimeSeriesData); ok {
		return len(ts.Values), nil
	}
	
	return nil, fmt.Errorf("参数必须是TimeSeriesData类型")
}

// TimeSeriesMeanFunction 时间序列平均值
type TimeSeriesMeanFunction struct{}
func (f *TimeSeriesMeanFunction) Name() string { return "timeSeriesMean" }
func (f *TimeSeriesMeanFunction) Description() string { return "计算时间序列数值的平均值" }
func (f *TimeSeriesMeanFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesMean函数需要1个参数")
	}
	
	if ts, ok := args[0].(*model.TimeSeriesData); ok {
		if len(ts.Values) == 0 {
			return 0.0, nil
		}
		
		sum := 0.0
		for _, val := range ts.Values {
			sum += val
		}
		
		return sum / float64(len(ts.Values)), nil
	}
	
	return nil, fmt.Errorf("参数必须是TimeSeriesData类型")
}

// TimeSeriesMinFunction 时间序列最小值
type TimeSeriesMinFunction struct{}
func (f *TimeSeriesMinFunction) Name() string { return "timeSeriesMin" }
func (f *TimeSeriesMinFunction) Description() string { return "查找时间序列中的最小值" }
func (f *TimeSeriesMinFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesMin函数需要1个参数")
	}
	
	if ts, ok := args[0].(*model.TimeSeriesData); ok {
		if len(ts.Values) == 0 {
			return nil, fmt.Errorf("时间序列为空")
		}
		
		min := ts.Values[0]
		for _, val := range ts.Values[1:] {
			if val < min {
				min = val
			}
		}
		
		return min, nil
	}
	
	return nil, fmt.Errorf("参数必须是TimeSeriesData类型")
}

// TimeSeriesMaxFunction 时间序列最大值
type TimeSeriesMaxFunction struct{}
func (f *TimeSeriesMaxFunction) Name() string { return "timeSeriesMax" }
func (f *TimeSeriesMaxFunction) Description() string { return "查找时间序列中的最大值" }
func (f *TimeSeriesMaxFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesMax函数需要1个参数")
	}
	
	if ts, ok := args[0].(*model.TimeSeriesData); ok {
		if len(ts.Values) == 0 {
			return nil, fmt.Errorf("时间序列为空")
		}
		
		max := ts.Values[0]
		for _, val := range ts.Values[1:] {
			if val > max {
				max = val
			}
		}
		
		return max, nil
	}
	
	return nil, fmt.Errorf("参数必须是TimeSeriesData类型")
}

// TimeSeriesTrendFunction 时间序列趋势分析
type TimeSeriesTrendFunction struct{}
func (f *TimeSeriesTrendFunction) Name() string { return "timeSeriesTrend" }
func (f *TimeSeriesTrendFunction) Description() string { return "计算时间序列的线性趋势斜率" }
func (f *TimeSeriesTrendFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesTrend函数需要1个参数")
	}
	
	if ts, ok := args[0].(*model.TimeSeriesData); ok {
		if len(ts.Values) < 2 {
			return 0.0, nil
		}
		
		n := len(ts.Values)
		sumX, sumY, sumXY, sumXX := 0.0, 0.0, 0.0, 0.0
		
		for i, val := range ts.Values {
			x := float64(i)
			y := val
			sumX += x
			sumY += y
			sumXY += x * y
			sumXX += x * x
		}
		
		nf := float64(n)
		numerator := nf*sumXY - sumX*sumY
		denominator := nf*sumXX - sumX*sumX
		
		if denominator == 0 {
			return 0.0, nil
		}
		
		return numerator / denominator, nil
	}
	
	return nil, fmt.Errorf("参数必须是TimeSeriesData类型")
}

// TimeSeriesVarianceFunction 时间序列方差
type TimeSeriesVarianceFunction struct{}
func (f *TimeSeriesVarianceFunction) Name() string { return "timeSeriesVariance" }
func (f *TimeSeriesVarianceFunction) Description() string { return "计算时间序列的方差" }
func (f *TimeSeriesVarianceFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesVariance函数需要1个参数")
	}
	
	if ts, ok := args[0].(*model.TimeSeriesData); ok {
		if len(ts.Values) < 2 {
			return 0.0, nil
		}
		
		// 计算均值
		sum := 0.0
		for _, val := range ts.Values {
			sum += val
		}
		mean := sum / float64(len(ts.Values))
		
		// 计算方差
		sumSquaredDiff := 0.0
		for _, val := range ts.Values {
			diff := val - mean
			sumSquaredDiff += diff * diff
		}
		
		return sumSquaredDiff / float64(len(ts.Values)-1), nil
	}
	
	return nil, fmt.Errorf("参数必须是TimeSeriesData类型")
}

// TimeSeriesStdDevFunction 时间序列标准差
type TimeSeriesStdDevFunction struct{}
func (f *TimeSeriesStdDevFunction) Name() string { return "timeSeriesStdDev" }
func (f *TimeSeriesStdDevFunction) Description() string { return "计算时间序列的标准差" }
func (f *TimeSeriesStdDevFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("timeSeriesStdDev函数需要1个参数")
	}
	
	// 复用方差函数
	varianceFunc := &TimeSeriesVarianceFunction{}
	variance, err := varianceFunc.Call(args...)
	if err != nil {
		return nil, err
	}
	
	if v, ok := variance.(float64); ok {
		return math.Sqrt(v), nil
	}
	
	return nil, fmt.Errorf("方差计算结果无效")
}

// =======================
// 通用复合数据实用函数
// =======================

// CompositeDataTypeFunction 获取复合数据类型
type CompositeDataTypeFunction struct{}
func (f *CompositeDataTypeFunction) Name() string { return "compositeType" }
func (f *CompositeDataTypeFunction) Description() string { return "获取复合数据的类型" }
func (f *CompositeDataTypeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("compositeType函数需要1个参数")
	}
	
	if composite, ok := args[0].(model.CompositeData); ok {
		return string(composite.Type()), nil
	}
	
	return nil, fmt.Errorf("参数必须是CompositeData类型")
}

// CompositeDataSizeFunction 获取复合数据大小
type CompositeDataSizeFunction struct{}
func (f *CompositeDataSizeFunction) Name() string { return "compositeSize" }
func (f *CompositeDataSizeFunction) Description() string { return "获取复合数据的大小" }
func (f *CompositeDataSizeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("compositeSize函数需要1个参数")
	}
	
	switch composite := args[0].(type) {
	case *model.VectorData:
		return len(composite.Values), nil
	case *model.ArrayData:
		return len(composite.Values), nil
	case *model.MatrixData:
		return composite.Rows * composite.Cols, nil
	case *model.TimeSeriesData:
		return len(composite.Values), nil
	default:
		return nil, fmt.Errorf("不支持的复合数据类型: %T", args[0])
	}
}

// CompositeDataValidateFunction 验证复合数据
type CompositeDataValidateFunction struct{}
func (f *CompositeDataValidateFunction) Name() string { return "compositeValidate" }
func (f *CompositeDataValidateFunction) Description() string { return "验证复合数据的有效性" }
func (f *CompositeDataValidateFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("compositeValidate函数需要1个参数")
	}
	
	if composite, ok := args[0].(model.CompositeData); ok {
		err := composite.Validate()
		return err == nil, nil
	}
	
	return nil, fmt.Errorf("参数必须是CompositeData类型")
}

// Vector3D专用函数
// Vector3DMagnitudeFunction Vector3D向量模长计算
type Vector3DMagnitudeFunction struct{}
func (f *Vector3DMagnitudeFunction) Name() string { return "vectorMagnitude3D" }
func (f *Vector3DMagnitudeFunction) Description() string { return "计算Vector3D的模长" }
func (f *Vector3DMagnitudeFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("vectorMagnitude3D函数需要1个参数")
	}
	
	if vector, ok := args[0].(*model.Vector3D); ok {
		magnitude := math.Sqrt(vector.X*vector.X + vector.Y*vector.Y + vector.Z*vector.Z)
		return magnitude, nil
	}
	
	return nil, fmt.Errorf("参数必须是Vector3D类型")
}

// Vector3DDotProductFunction Vector3D点积计算
type Vector3DDotProductFunction struct{}
func (f *Vector3DDotProductFunction) Name() string { return "vectorDotProduct3D" }
func (f *Vector3DDotProductFunction) Description() string { return "计算两个Vector3D的点积" }
func (f *Vector3DDotProductFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("vectorDotProduct3D函数需要2个参数")
	}
	
	vector1, ok1 := args[0].(*model.Vector3D)
	vector2, ok2 := args[1].(*model.Vector3D)
	
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("两个参数都必须是Vector3D类型")
	}
	
	dotProduct := vector1.X*vector2.X + vector1.Y*vector2.Y + vector1.Z*vector2.Z
	return dotProduct, nil
}

// Vector3DCrossProductFunction Vector3D叉积计算
type Vector3DCrossProductFunction struct{}
func (f *Vector3DCrossProductFunction) Name() string { return "vectorCross3D" }
func (f *Vector3DCrossProductFunction) Description() string { return "计算两个Vector3D的叉积" }
func (f *Vector3DCrossProductFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("vectorCross3D函数需要2个参数")
	}
	
	vector1, ok1 := args[0].(*model.Vector3D)
	vector2, ok2 := args[1].(*model.Vector3D)
	
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("两个参数都必须是Vector3D类型")
	}
	
	crossProduct := &model.Vector3D{
		X: vector1.Y*vector2.Z - vector1.Z*vector2.Y,
		Y: vector1.Z*vector2.X - vector1.X*vector2.Z,
		Z: vector1.X*vector2.Y - vector1.Y*vector2.X,
	}
	return crossProduct, nil
}

// =======================
// 辅助函数
// =======================