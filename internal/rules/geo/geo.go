package geo

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Coordinate 地理坐标
type Coordinate struct {
	Latitude  float64 `json:"latitude"`  // 纬度 (-90 to 90)
	Longitude float64 `json:"longitude"` // 经度 (-180 to 180)
}

// Region 地理区域
type Region struct {
	Name   string     `json:"name"`
	Center Coordinate `json:"center"`
	Radius float64    `json:"radius_km"` // 半径 (公里)
}

// BoundingBox 矩形边界框
type BoundingBox struct {
	NorthEast Coordinate `json:"north_east"`
	SouthWest Coordinate `json:"south_west"`
}

// GeoProcessor 地理数据处理器
type GeoProcessor struct {
	regions map[string]*Region
}

// NewGeoProcessor 创建地理数据处理器
func NewGeoProcessor() *GeoProcessor {
	return &GeoProcessor{
		regions: make(map[string]*Region),
	}
}

// ParseCoordinate 解析坐标字符串
func (gp *GeoProcessor) ParseCoordinate(latStr, lngStr string) (*Coordinate, error) {
	lat, err := strconv.ParseFloat(strings.TrimSpace(latStr), 64)
	if err != nil {
		return nil, errors.New("invalid latitude format")
	}
	
	lng, err := strconv.ParseFloat(strings.TrimSpace(lngStr), 64)
	if err != nil {
		return nil, errors.New("invalid longitude format")
	}
	
	if !gp.IsValidCoordinate(lat, lng) {
		return nil, errors.New("coordinate out of valid range")
	}
	
	return &Coordinate{
		Latitude:  lat,
		Longitude: lng,
	}, nil
}

// IsValidCoordinate 验证坐标有效性
func (gp *GeoProcessor) IsValidCoordinate(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

// HaversineDistance 使用Haversine公式计算两点间距离 (公里)
func (gp *GeoProcessor) HaversineDistance(coord1, coord2 Coordinate) float64 {
	const earthRadiusKm = 6371.0
	
	// 转换为弧度
	lat1Rad := coord1.Latitude * math.Pi / 180
	lat2Rad := coord2.Latitude * math.Pi / 180
	deltaLatRad := (coord2.Latitude - coord1.Latitude) * math.Pi / 180
	deltaLngRad := (coord2.Longitude - coord1.Longitude) * math.Pi / 180
	
	// Haversine公式
	a := math.Sin(deltaLatRad/2)*math.Sin(deltaLatRad/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
		math.Sin(deltaLngRad/2)*math.Sin(deltaLngRad/2)
	
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return earthRadiusKm * c
}

// VincentyDistance 使用Vincenty公式计算精确距离 (公里)
func (gp *GeoProcessor) VincentyDistance(coord1, coord2 Coordinate) float64 {
	const (
		a = 6378137.0     // 长半轴 (米)
		b = 6356752.314245 // 短半轴 (米)
		f = 1 / 298.257223563 // 扁率
	)
	
	lat1 := coord1.Latitude * math.Pi / 180
	lat2 := coord2.Latitude * math.Pi / 180
	deltaLng := (coord2.Longitude - coord1.Longitude) * math.Pi / 180
	
	// 简化的Vincenty公式实现
	U1 := math.Atan((1-f) * math.Tan(lat1))
	U2 := math.Atan((1-f) * math.Tan(lat2))
	
	sinU1, cosU1 := math.Sincos(U1)
	sinU2, cosU2 := math.Sincos(U2)
	
	lambda := deltaLng
	lambdaP := 2 * math.Pi
	iterLimit := 100
	
	var cosSqAlpha, sinSigma, cos2SigmaM, cosSigma, sigma float64
	
	for math.Abs(lambda-lambdaP) > 1e-12 && iterLimit > 0 {
		sinLambda, cosLambda := math.Sincos(lambda)
		
		sinSigma = math.Sqrt((cosU2*sinLambda)*(cosU2*sinLambda) +
			(cosU1*sinU2-sinU1*cosU2*cosLambda)*(cosU1*sinU2-sinU1*cosU2*cosLambda))
		
		if sinSigma == 0 {
			return 0 // 重合点
		}
		
		cosSigma = sinU1*sinU2 + cosU1*cosU2*cosLambda
		sigma = math.Atan2(sinSigma, cosSigma)
		
		sinAlpha := cosU1 * cosU2 * sinLambda / sinSigma
		cosSqAlpha = 1 - sinAlpha*sinAlpha
		
		if cosSqAlpha != 0 {
			cos2SigmaM = cosSigma - 2*sinU1*sinU2/cosSqAlpha
		} else {
			cos2SigmaM = 0 // 赤道线
		}
		
		C := f / 16 * cosSqAlpha * (4 + f*(4-3*cosSqAlpha))
		lambdaP = lambda
		lambda = deltaLng + (1-C)*f*sinAlpha*(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
		iterLimit--
	}
	
	uSq := cosSqAlpha * (a*a - b*b) / (b * b)
	A := 1 + uSq/16384*(4096+uSq*(-768+uSq*(320-175*uSq)))
	B := uSq / 1024 * (256 + uSq*(-128+uSq*(74-47*uSq)))
	
	deltaSigma := B * sinSigma * (cos2SigmaM + B/4*(cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)-
		B/6*cos2SigmaM*(-3+4*sinSigma*sinSigma)*(-3+4*cos2SigmaM*cos2SigmaM)))
	
	s := b * A * (sigma - deltaSigma)
	
	return s / 1000 // 转换为公里
}

// AddRegion 添加地理区域
func (gp *GeoProcessor) AddRegion(region *Region) {
	gp.regions[region.Name] = region
}

// IsInRegion 判断坐标是否在指定区域内
func (gp *GeoProcessor) IsInRegion(coord Coordinate, regionName string) bool {
	region, exists := gp.regions[regionName]
	if !exists {
		return false
	}
	
	distance := gp.HaversineDistance(coord, region.Center)
	return distance <= region.Radius
}

// IsInAnyRegion 判断坐标是否在任意区域内
func (gp *GeoProcessor) IsInAnyRegion(coord Coordinate) []string {
	var matchedRegions []string
	
	for name, region := range gp.regions {
		distance := gp.HaversineDistance(coord, region.Center)
		if distance <= region.Radius {
			matchedRegions = append(matchedRegions, name)
		}
	}
	
	return matchedRegions
}

// IsInBoundingBox 判断坐标是否在矩形边界框内
func (gp *GeoProcessor) IsInBoundingBox(coord Coordinate, bbox BoundingBox) bool {
	return coord.Latitude >= bbox.SouthWest.Latitude &&
		coord.Latitude <= bbox.NorthEast.Latitude &&
		coord.Longitude >= bbox.SouthWest.Longitude &&
		coord.Longitude <= bbox.NorthEast.Longitude
}

// GetNearestRegion 获取最近的区域
func (gp *GeoProcessor) GetNearestRegion(coord Coordinate) (string, float64) {
	var nearestRegion string
	minDistance := math.Inf(1)
	
	for name, region := range gp.regions {
		distance := gp.HaversineDistance(coord, region.Center)
		if distance < minDistance {
			minDistance = distance
			nearestRegion = name
		}
	}
	
	return nearestRegion, minDistance
}

// Bearing 计算两点间的方位角 (度)
func (gp *GeoProcessor) Bearing(from, to Coordinate) float64 {
	lat1 := from.Latitude * math.Pi / 180
	lat2 := to.Latitude * math.Pi / 180
	deltaLng := (to.Longitude - from.Longitude) * math.Pi / 180
	
	y := math.Sin(deltaLng) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLng)
	
	bearing := math.Atan2(y, x) * 180 / math.Pi
	
	// 转换为0-360度
	if bearing < 0 {
		bearing += 360
	}
	
	return bearing
}

// MidPoint 计算两点中点
func (gp *GeoProcessor) MidPoint(coord1, coord2 Coordinate) Coordinate {
	lat1 := coord1.Latitude * math.Pi / 180
	lat2 := coord2.Latitude * math.Pi / 180
	lng1 := coord1.Longitude * math.Pi / 180
	lng2 := coord2.Longitude * math.Pi / 180
	
	deltaLng := lng2 - lng1
	
	bx := math.Cos(lat2) * math.Cos(deltaLng)
	by := math.Cos(lat2) * math.Sin(deltaLng)
	
	lat3 := math.Atan2(math.Sin(lat1)+math.Sin(lat2),
		math.Sqrt((math.Cos(lat1)+bx)*(math.Cos(lat1)+bx)+by*by))
	lng3 := lng1 + math.Atan2(by, math.Cos(lat1)+bx)
	
	return Coordinate{
		Latitude:  lat3 * 180 / math.Pi,
		Longitude: lng3 * 180 / math.Pi,
	}
}

// DestinationPoint 根据起点、距离和方位角计算目标点
func (gp *GeoProcessor) DestinationPoint(start Coordinate, distance float64, bearing float64) Coordinate {
	const earthRadiusKm = 6371.0
	
	lat1 := start.Latitude * math.Pi / 180
	lng1 := start.Longitude * math.Pi / 180
	bearingRad := bearing * math.Pi / 180
	
	lat2 := math.Asin(math.Sin(lat1)*math.Cos(distance/earthRadiusKm) +
		math.Cos(lat1)*math.Sin(distance/earthRadiusKm)*math.Cos(bearingRad))
	
	lng2 := lng1 + math.Atan2(math.Sin(bearingRad)*math.Sin(distance/earthRadiusKm)*math.Cos(lat1),
		math.Cos(distance/earthRadiusKm)-math.Sin(lat1)*math.Sin(lat2))
	
	return Coordinate{
		Latitude:  lat2 * 180 / math.Pi,
		Longitude: lng2 * 180 / math.Pi,
	}
}

// GetRegionStats 获取区域统计信息
func (gp *GeoProcessor) GetRegionStats() map[string]interface{} {
	stats := make(map[string]interface{})
	
	stats["total_regions"] = len(gp.regions)
	
	regionList := make([]map[string]interface{}, 0, len(gp.regions))
	for name, region := range gp.regions {
		regionInfo := map[string]interface{}{
			"name":      name,
			"center":    region.Center,
			"radius_km": region.Radius,
		}
		regionList = append(regionList, regionInfo)
	}
	
	stats["regions"] = regionList
	
	return stats
}

// ValidateGeoData 验证地理数据的完整性和准确性
func (gp *GeoProcessor) ValidateGeoData(data map[string]interface{}) []string {
	var errors []string
	
	// 检查必要字段
	lat, hasLat := data["latitude"]
	lng, hasLng := data["longitude"]
	
	if !hasLat && !hasLng {
		// 检查其他可能的字段名
		if lat, hasLat = data["lat"]; !hasLat {
			errors = append(errors, "missing latitude field")
		}
		if lng, hasLng = data["lng"]; !hasLng {
			if lng, hasLng = data["lon"]; !hasLng {
				errors = append(errors, "missing longitude field")
			}
		}
	}
	
	// 验证数值类型和范围
	if hasLat {
		if latVal, ok := lat.(float64); ok {
			if latVal < -90 || latVal > 90 {
				errors = append(errors, "latitude out of range [-90, 90]")
			}
		} else if latStr, ok := lat.(string); ok {
			if _, err := strconv.ParseFloat(latStr, 64); err != nil {
				errors = append(errors, "invalid latitude format")
			}
		} else {
			errors = append(errors, "latitude must be number or string")
		}
	}
	
	if hasLng {
		if lngVal, ok := lng.(float64); ok {
			if lngVal < -180 || lngVal > 180 {
				errors = append(errors, "longitude out of range [-180, 180]")
			}
		} else if lngStr, ok := lng.(string); ok {
			if _, err := strconv.ParseFloat(lngStr, 64); err != nil {
				errors = append(errors, "invalid longitude format")
			}
		} else {
			errors = append(errors, "longitude must be number or string")
		}
	}
	
	return errors
}