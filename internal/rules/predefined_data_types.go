package rules

import "strings"

// PredefinedDataTypes 预设的数据类型定义
var PredefinedDataTypes = map[string]*DataTypeDefinition{
	"location": {
		Type: "location",
		Fields: map[string]FieldDefinition{
			"location": {
				Type: "gps_coordinate",
				Properties: map[string]PropertyDefinition{
					"latitude":  {Type: "float", Range: []float64{-90, 90}, Unit: "degrees"},
					"longitude": {Type: "float", Range: []float64{-180, 180}, Unit: "degrees"},
					"altitude":  {Type: "float", Unit: "meters", Optional: true},
				},
			},
		},
		CoordinateSystem: "WGS84",
	},
	"vector3d": {
		Type: "vector3d",
		Fields: map[string]FieldDefinition{
			"acceleration": {
				Type: "vector3d",
				Properties: map[string]PropertyDefinition{
					"x":         {Type: "float", Unit: "m/s²"},
					"y":         {Type: "float", Unit: "m/s²"},
					"z":         {Type: "float", Unit: "m/s²"},
					"magnitude": {Type: "float", Unit: "m/s²", Computed: true},
				},
			},
			"velocity": {
				Type: "vector3d",
				Properties: map[string]PropertyDefinition{
					"x":         {Type: "float", Unit: "m/s"},
					"y":         {Type: "float", Unit: "m/s"},
					"z":         {Type: "float", Unit: "m/s"},
					"magnitude": {Type: "float", Unit: "m/s", Computed: true},
				},
			},
		},
	},
	"color": {
		Type: "color",
		Fields: map[string]FieldDefinition{
			"color": {
				Type: "color_data",
				Properties: map[string]PropertyDefinition{
					"r":          {Type: "int", Range: []float64{0, 255}, Unit: "rgb"},
					"g":          {Type: "int", Range: []float64{0, 255}, Unit: "rgb"},
					"b":          {Type: "int", Range: []float64{0, 255}, Unit: "rgb"},
					"a":          {Type: "int", Range: []float64{0, 255}, Unit: "alpha", Optional: true},
					"hue":        {Type: "float", Range: []float64{0, 360}, Unit: "degrees", Computed: true},
					"saturation": {Type: "float", Range: []float64{0, 1}, Unit: "percentage", Computed: true},
					"lightness":  {Type: "float", Range: []float64{0, 1}, Unit: "percentage", Computed: true},
				},
			},
		},
		ColorSpace: "RGB",
	},
	"array": {
		Type: "array",
		Fields: map[string]FieldDefinition{
			"sensor_readings": {
				Type: "numeric_array",
				Properties: map[string]PropertyDefinition{
					"values":         {Type: "array[float]", MinLength: 0, MaxLength: 1000},
					"length":         {Type: "int", Computed: true},
					"size":           {Type: "int", Computed: true},
					"sum":            {Type: "float", Computed: true},
					"average":        {Type: "float", Computed: true},
					"mean":           {Type: "float", Computed: true},
					"min":            {Type: "float", Computed: true},
					"max":            {Type: "float", Computed: true},
					"range":          {Type: "float", Computed: true},
					"data_type":      {Type: "string", Computed: true},
					"unit":           {Type: "string", Optional: true},
				},
			},
			"data_array": {
				Type: "generic_array",
				Properties: map[string]PropertyDefinition{
					"values":    {Type: "array[interface]", MinLength: 0, MaxLength: 1000},
					"length":    {Type: "int", Computed: true},
					"size":      {Type: "int", Computed: true},
					"data_type": {Type: "string", Computed: true},
				},
			},
		},
		ArrayType: "numeric",
	},
	"timeseries": {
		Type: "timeseries",
		Fields: map[string]FieldDefinition{
			"series": {
				Type: "time_series",
				Properties: map[string]PropertyDefinition{
					"timestamps": {Type: "array[timestamp]"},
					"values":     {Type: "array[float]"},
					"interval":   {Type: "duration", Unit: "seconds", Computed: true},
					"trend":      {Type: "float", Computed: true},
					"length":     {Type: "int", Computed: true},
					"duration":   {Type: "duration", Unit: "seconds", Computed: true},
				},
			},
		},
		TimeUnit: "seconds",
	},
	"matrix": {
		Type: "matrix",
		Fields: map[string]FieldDefinition{
			"matrix": {
				Type: "numeric_matrix",
				Properties: map[string]PropertyDefinition{
					"rows":        {Type: "int", Computed: true},
					"cols":        {Type: "int", Computed: true},
					"values":      {Type: "array[array[float]]"},
					"determinant": {Type: "float", Computed: true},
					"rank":        {Type: "int", Computed: true},
				},
			},
		},
		MatrixType: "dense",
	},
	// 别名支持，保持向后兼容和前端友好
	"gps": {
		Type: "location",  // 映射到model中的location类型
		Fields: map[string]FieldDefinition{
			"location": {
				Type: "gps_coordinate",
				Properties: map[string]PropertyDefinition{
					"latitude":  {Type: "float", Range: []float64{-90, 90}, Unit: "degrees"},
					"longitude": {Type: "float", Range: []float64{-180, 180}, Unit: "degrees"},
					"altitude":  {Type: "float", Unit: "meters", Optional: true},
				},
			},
		},
		CoordinateSystem: "WGS84",
	},
	"3d_vector": {
		Type: "vector3d",  // 映射到model中的vector3d类型
		Fields: map[string]FieldDefinition{
			"acceleration": {
				Type: "vector3d",
				Properties: map[string]PropertyDefinition{
					"x":         {Type: "float", Unit: "m/s²"},
					"y":         {Type: "float", Unit: "m/s²"},
					"z":         {Type: "float", Unit: "m/s²"},
					"magnitude": {Type: "float", Unit: "m/s²", Computed: true},
				},
			},
			"velocity": {
				Type: "vector3d",
				Properties: map[string]PropertyDefinition{
					"x":         {Type: "float", Unit: "m/s"},
					"y":         {Type: "float", Unit: "m/s"},
					"z":         {Type: "float", Unit: "m/s"},
					"magnitude": {Type: "float", Unit: "m/s", Computed: true},
				},
			},
		},
	},
}

// DetectDataTypeFromFields 根据字段访问模式自动检测数据类型
func DetectDataTypeFromFields(rule *Rule) string {
	// 如果已经有明确的数据类型定义，直接返回
	if rule.DataType != nil {
		if dataTypeName, ok := rule.DataType.(string); ok {
			return dataTypeName
		}
		if dataTypeMap, ok := rule.DataType.(map[string]interface{}); ok {
			if typeValue, exists := dataTypeMap["type"]; exists {
				if typeName, ok := typeValue.(string); ok {
					return typeName
				}
			}
		}
	}

	// 收集条件和动作中的字段访问模式
	fields := collectFieldPatterns(rule)
	
	// 根据字段模式匹配数据类型
	for dataType, definition := range PredefinedDataTypes {
		if matchesFieldPattern(fields, definition) {
			return dataType
		}
	}

	return "unknown"
}

// collectFieldPatterns 收集规则中的字段访问模式
func collectFieldPatterns(rule *Rule) []string {
	fields := make(map[string]bool)
	
	// 从条件中收集字段
	if rule.Conditions != nil {
		collectFieldsFromCondition(rule.Conditions, fields)
	}
	
	// 从动作中收集字段
	for _, action := range rule.Actions {
		collectFieldsFromAction(&action, fields)
	}
	
	// 转换为切片
	result := make([]string, 0, len(fields))
	for field := range fields {
		result = append(result, field)
	}
	
	return result
}

// collectFieldsFromCondition 从条件中收集字段
func collectFieldsFromCondition(condition *Condition, fields map[string]bool) {
	if condition == nil {
		return
	}

	if condition.Field != "" {
		fields[condition.Field] = true
	}
	
	if condition.Expression != "" {
		// 简单的表达式字段提取
		extractFieldsFromExpression(condition.Expression, fields)
	}

	// 递归处理复合条件
	for _, subCondition := range condition.And {
		collectFieldsFromCondition(subCondition, fields)
	}
	for _, subCondition := range condition.Or {
		collectFieldsFromCondition(subCondition, fields)
	}
	if condition.Not != nil {
		collectFieldsFromCondition(condition.Not, fields)
	}
}

// collectFieldsFromAction 从动作中收集字段
func collectFieldsFromAction(action *Action, fields map[string]bool) {
	if action.Config == nil {
		return
	}

	// 从消息模板中提取字段
	if message, ok := action.Config["message"].(string); ok {
		extractFieldsFromTemplate(message, fields)
	}
	
	// 从表达式中提取字段
	if params, ok := action.Config["parameters"].(map[string]interface{}); ok {
		if expr, ok := params["expression"].(string); ok {
			extractFieldsFromExpression(expr, fields)
		}
	}
}

// extractFieldsFromExpression 从表达式中提取字段
func extractFieldsFromExpression(expression string, fields map[string]bool) {
	// 简单的字段提取 - 查找点分割的字段名
	words := strings.Fields(expression)
	for _, word := range words {
		if strings.Contains(word, ".") && !strings.Contains(word, "(") {
			// 移除操作符和括号
			field := strings.TrimFunc(word, func(r rune) bool {
				return r == '(' || r == ')' || r == ',' || r == '>' || r == '<' || r == '=' || r == '!'
			})
			if field != "" {
				fields[field] = true
			}
		}
	}
}

// extractFieldsFromTemplate 从模板字符串中提取字段
func extractFieldsFromTemplate(template string, fields map[string]bool) {
	// 查找 {{field.name}} 模式
	start := 0
	for {
		startIdx := strings.Index(template[start:], "{{")
		if startIdx == -1 {
			break
		}
		startIdx += start
		
		endIdx := strings.Index(template[startIdx:], "}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx
		
		field := strings.TrimSpace(template[startIdx+2 : endIdx])
		if field != "" {
			fields[field] = true
		}
		
		start = endIdx + 2
	}
}

// matchesFieldPattern 检查字段模式是否匹配数据类型定义
func matchesFieldPattern(fields []string, definition *DataTypeDefinition) bool {
	if len(fields) == 0 {
		return false
	}

	matchCount := 0
	for _, field := range fields {
		if matchesDataTypeField(field, definition) {
			matchCount++
		}
	}

	// 如果至少有一半的字段匹配，认为是这种数据类型
	return float64(matchCount)/float64(len(fields)) >= 0.5
}

// matchesDataTypeField 检查单个字段是否匹配数据类型定义
func matchesDataTypeField(field string, definition *DataTypeDefinition) bool {
	parts := strings.Split(field, ".")
	if len(parts) < 2 {
		return false
	}

	fieldName := parts[0]
	propertyName := parts[1]

	if fieldDef, exists := definition.Fields[fieldName]; exists {
		if _, propertyExists := fieldDef.Properties[propertyName]; propertyExists {
			return true
		}
	}

	return false
}

// GetDataTypeDefinition 获取数据类型定义（预设或自定义）
func GetDataTypeDefinition(rule *Rule) *DataTypeDefinition {
	if rule.DataType == nil {
		// 尝试自动检测
		detectedType := DetectDataTypeFromFields(rule)
		if predefined, exists := PredefinedDataTypes[detectedType]; exists {
			return predefined
		}
		return nil
	}

	// 如果是字符串形式，查找预设定义
	if dataTypeName, ok := rule.DataType.(string); ok {
		if predefined, exists := PredefinedDataTypes[dataTypeName]; exists {
			return predefined
		}
		return nil
	}

	// 如果是详细定义形式，直接返回
	if dataTypeDef, ok := rule.DataType.(*DataTypeDefinition); ok {
		return dataTypeDef
	}

	// 尝试从map[string]interface{}中解析（JSON解码后的格式）
	if dataTypeMap, ok := rule.DataType.(map[string]interface{}); ok {
		// 如果只有type字段，查找预设定义
		if typeValue, exists := dataTypeMap["type"]; exists {
			if typeName, ok := typeValue.(string); ok {
				if len(dataTypeMap) == 1 {
					// 只有type字段，使用预设定义
					if predefined, exists := PredefinedDataTypes[typeName]; exists {
						return predefined
					}
				} else {
					// 有其他字段，解析完整定义
					return parseDataTypeFromMap(dataTypeMap)
				}
			}
		}
	}

	return nil
}

// parseDataTypeFromMap 从map解析数据类型定义
func parseDataTypeFromMap(dataMap map[string]interface{}) *DataTypeDefinition {
	def := &DataTypeDefinition{}
	
	if typeValue, ok := dataMap["type"].(string); ok {
		def.Type = typeValue
	}
	
	if coordSystem, ok := dataMap["coordinate_system"].(string); ok {
		def.CoordinateSystem = coordSystem
	}
	
	if colorSpace, ok := dataMap["color_space"].(string); ok {
		def.ColorSpace = colorSpace
	}
	
	if arrayType, ok := dataMap["array_type"].(string); ok {
		def.ArrayType = arrayType
	}
	
	if timeUnit, ok := dataMap["time_unit"].(string); ok {
		def.TimeUnit = timeUnit
	}
	
	if matrixType, ok := dataMap["matrix_type"].(string); ok {
		def.MatrixType = matrixType
	}
	
	// 解析字段定义 - 这里可以根据需要扩展
	if fieldsMap, ok := dataMap["fields"].(map[string]interface{}); ok {
		def.Fields = make(map[string]FieldDefinition)
		// TODO: 实现完整的字段解析
		_ = fieldsMap // 暂时忽略，使用预设定义
	}
	
	return def
}