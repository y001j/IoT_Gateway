package geo

import (
	"fmt"
	"strconv"
)

// DistanceFunction 计算两点间距离的函数
type DistanceFunction struct {
	processor *GeoProcessor
}

func NewDistanceFunction(processor *GeoProcessor) *DistanceFunction {
	return &DistanceFunction{processor: processor}
}

func (f *DistanceFunction) Name() string {
	return "distance"
}

func (f *DistanceFunction) Description() string {
	return "计算两个地理坐标点之间的距离(公里): distance(lat1, lng1, lat2, lng2)"
}

func (f *DistanceFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("distance function requires 4 arguments (lat1, lng1, lat2, lng2)")
	}
	
	lat1, err := f.toFloat64(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid lat1: %v", err)
	}
	
	lng1, err := f.toFloat64(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid lng1: %v", err)
	}
	
	lat2, err := f.toFloat64(args[2])
	if err != nil {
		return nil, fmt.Errorf("invalid lat2: %v", err)
	}
	
	lng2, err := f.toFloat64(args[3])
	if err != nil {
		return nil, fmt.Errorf("invalid lng2: %v", err)
	}
	
	coord1 := Coordinate{Latitude: lat1, Longitude: lng1}
	coord2 := Coordinate{Latitude: lat2, Longitude: lng2}
	
	if !f.processor.IsValidCoordinate(lat1, lng1) {
		return nil, fmt.Errorf("invalid coordinate 1: lat=%f, lng=%f", lat1, lng1)
	}
	
	if !f.processor.IsValidCoordinate(lat2, lng2) {
		return nil, fmt.Errorf("invalid coordinate 2: lat=%f, lng=%f", lat2, lng2)
	}
	
	distance := f.processor.HaversineDistance(coord1, coord2)
	return distance, nil
}

func (f *DistanceFunction) toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

// InRegionFunction 判断坐标是否在区域内的函数
type InRegionFunction struct {
	processor *GeoProcessor
}

func NewInRegionFunction(processor *GeoProcessor) *InRegionFunction {
	return &InRegionFunction{processor: processor}
}

func (f *InRegionFunction) Name() string {
	return "in_region"
}

func (f *InRegionFunction) Description() string {
	return "判断坐标是否在指定区域内: in_region(lat, lng, region_name)"
}

func (f *InRegionFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 3 {
		return nil, fmt.Errorf("in_region function requires 3 arguments (lat, lng, region_name)")
	}
	
	lat, err := f.toFloat64(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %v", err)
	}
	
	lng, err := f.toFloat64(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %v", err)
	}
	
	regionName, ok := args[2].(string)
	if !ok {
		return nil, fmt.Errorf("region_name must be string")
	}
	
	if !f.processor.IsValidCoordinate(lat, lng) {
		return nil, fmt.Errorf("invalid coordinate: lat=%f, lng=%f", lat, lng)
	}
	
	coord := Coordinate{Latitude: lat, Longitude: lng}
	return f.processor.IsInRegion(coord, regionName), nil
}

func (f *InRegionFunction) toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

// NearestRegionFunction 获取最近区域的函数
type NearestRegionFunction struct {
	processor *GeoProcessor
}

func NewNearestRegionFunction(processor *GeoProcessor) *NearestRegionFunction {
	return &NearestRegionFunction{processor: processor}
}

func (f *NearestRegionFunction) Name() string {
	return "nearest_region"
}

func (f *NearestRegionFunction) Description() string {
	return "获取坐标最近的区域名称: nearest_region(lat, lng)"
}

func (f *NearestRegionFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("nearest_region function requires 2 arguments (lat, lng)")
	}
	
	lat, err := f.toFloat64(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %v", err)
	}
	
	lng, err := f.toFloat64(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %v", err)
	}
	
	if !f.processor.IsValidCoordinate(lat, lng) {
		return nil, fmt.Errorf("invalid coordinate: lat=%f, lng=%f", lat, lng)
	}
	
	coord := Coordinate{Latitude: lat, Longitude: lng}
	regionName, _ := f.processor.GetNearestRegion(coord)
	return regionName, nil
}

func (f *NearestRegionFunction) toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

// BearingFunction 计算方位角的函数
type BearingFunction struct {
	processor *GeoProcessor
}

func NewBearingFunction(processor *GeoProcessor) *BearingFunction {
	return &BearingFunction{processor: processor}
}

func (f *BearingFunction) Name() string {
	return "bearing"
}

func (f *BearingFunction) Description() string {
	return "计算两点间的方位角(度): bearing(lat1, lng1, lat2, lng2)"
}

func (f *BearingFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("bearing function requires 4 arguments (lat1, lng1, lat2, lng2)")
	}
	
	lat1, err := f.toFloat64(args[0])
	if err != nil {
		return nil, fmt.Errorf("invalid lat1: %v", err)
	}
	
	lng1, err := f.toFloat64(args[1])
	if err != nil {
		return nil, fmt.Errorf("invalid lng1: %v", err)
	}
	
	lat2, err := f.toFloat64(args[2])
	if err != nil {
		return nil, fmt.Errorf("invalid lat2: %v", err)
	}
	
	lng2, err := f.toFloat64(args[3])
	if err != nil {
		return nil, fmt.Errorf("invalid lng2: %v", err)
	}
	
	coord1 := Coordinate{Latitude: lat1, Longitude: lng1}
	coord2 := Coordinate{Latitude: lat2, Longitude: lng2}
	
	if !f.processor.IsValidCoordinate(lat1, lng1) {
		return nil, fmt.Errorf("invalid coordinate 1: lat=%f, lng=%f", lat1, lng1)
	}
	
	if !f.processor.IsValidCoordinate(lat2, lng2) {
		return nil, fmt.Errorf("invalid coordinate 2: lat=%f, lng=%f", lat2, lng2)
	}
	
	bearing := f.processor.Bearing(coord1, coord2)
	return bearing, nil
}

func (f *BearingFunction) toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}

// ValidCoordinateFunction 验证坐标有效性的函数
type ValidCoordinateFunction struct {
	processor *GeoProcessor
}

func NewValidCoordinateFunction(processor *GeoProcessor) *ValidCoordinateFunction {
	return &ValidCoordinateFunction{processor: processor}
}

func (f *ValidCoordinateFunction) Name() string {
	return "valid_coordinate"
}

func (f *ValidCoordinateFunction) Description() string {
	return "验证坐标是否有效: valid_coordinate(lat, lng)"
}

func (f *ValidCoordinateFunction) Call(args ...interface{}) (interface{}, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("valid_coordinate function requires 2 arguments (lat, lng)")
	}
	
	lat, err := f.toFloat64(args[0])
	if err != nil {
		return false, nil // 无法转换为数字，返回false但不报错
	}
	
	lng, err := f.toFloat64(args[1])
	if err != nil {
		return false, nil // 无法转换为数字，返回false但不报错
	}
	
	return f.processor.IsValidCoordinate(lat, lng), nil
}

func (f *ValidCoordinateFunction) toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", val)
	}
}