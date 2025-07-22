package model

import (
	"encoding/json"
	"time"
)

// DataType 表示点位数据的类型
type DataType string

const (
	TypeInt    DataType = "int"
	TypeFloat  DataType = "float"
	TypeBool   DataType = "bool"
	TypeString DataType = "string"
	TypeBinary DataType = "binary"
)

// Point 表示从设备采集的一个数据点
type Point struct {
	// 点位唯一标识
	Key string `json:"key"`

	// 点位所属设备ID
	DeviceID string `json:"device_id"`

	// 采集时间戳
	Timestamp time.Time `json:"timestamp"`

	// 数据类型
	Type DataType `json:"type"`

	// 数据值，使用interface{}以支持多种类型
	Value interface{} `json:"value"`

	// 质量码，0表示正常，其他值表示异常
	Quality int `json:"quality"`

	// 附加标签，用于分类、过滤等
	Tags map[string]string `json:"tags,omitempty"`
}

// NewPoint 创建一个新的数据点
func NewPoint(key, deviceID string, value interface{}, dataType DataType) Point {
	return Point{
		Key:       key,
		DeviceID:  deviceID,
		Timestamp: time.Now(),
		Type:      dataType,
		Value:     value,
		Quality:   0, // 默认为正常
		Tags:      make(map[string]string),
	}
}

// AddTag 添加一个标签
func (p *Point) AddTag(key, value string) {
	if p.Tags == nil {
		p.Tags = make(map[string]string)
	}
	p.Tags[key] = value
}

// SetQuality 设置质量码
func (p *Point) SetQuality(quality int) {
	p.Quality = quality
}

// UnmarshalJSON 自定义JSON反序列化
func (p *Point) UnmarshalJSON(data []byte) error {
	type Alias Point
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	return json.Unmarshal(data, &aux)
}
