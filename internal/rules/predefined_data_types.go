package rules

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
					"values":    {Type: "array[float]", MinLength: 0, MaxLength: 1000},
					"length":    {Type: "int", Computed: true},
					"size":      {Type: "int", Computed: true},
					"sum":       {Type: "float", Computed: true},
					"average":   {Type: "float", Computed: true},
					"mean":      {Type: "float", Computed: true},
					"min":       {Type: "float", Computed: true},
					"max":       {Type: "float", Computed: true},
					"range":     {Type: "float", Computed: true},
					"data_type": {Type: "string", Computed: true},
					"unit":      {Type: "string", Optional: true},
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
		Type: "location", // 映射到model中的location类型
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
		Type: "vector3d", // 映射到model中的vector3d类型
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
