package influxdb

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

func init() {
	// 注册连接器工厂
	northbound.Register("influxdb", func() northbound.Sink {
		return NewInfluxDBSink()
	})
}

// NewInfluxDBSink 创建一个新的InfluxDB连接器
func NewInfluxDBSink() *InfluxDBSink {
	return &InfluxDBSink{
		BaseSink: northbound.NewBaseSink("influxdb"),
	}
}

// InfluxDBSink 是一个InfluxDB连接器，用于将数据点存储到InfluxDB时序数据库
type InfluxDBSink struct {
	*northbound.BaseSink
	client         influxdb2.Client
	writeAPI       api.WriteAPI
	pointsConfig   map[string]PointConfig
	defaultBucket  string
	defaultOrg     string
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// PointConfig 定义了数据点的InfluxDB配置
type PointConfig struct {
	Measurement string            `json:"measurement"` // InfluxDB测量名称
	Bucket      string            `json:"bucket"`      // InfluxDB桶（可选，覆盖默认值）
	Org         string            `json:"org"`         // InfluxDB组织（可选，覆盖默认值）
	Tags        map[string]string `json:"tags"`        // 附加标签
	Fields      []string          `json:"fields"`      // 要保存为字段的标签（默认值存储为"value"字段）
}

// InfluxDBConfig 是InfluxDB连接器的特定参数配置
type InfluxDBConfig struct {
	URL           string                 `json:"url"`          // InfluxDB服务器URL
	Token         string                 `json:"token"`        // InfluxDB认证令牌
	Org           string                 `json:"org"`          // 默认组织
	Bucket        string                 `json:"bucket"`       // 默认桶
	FlushInterval int                    `json:"flush_interval_ms"` // 刷新间隔(ms)
	Points        map[string]PointConfig `json:"points"`       // 数据点配置
}

// Init 初始化连接器
func (s *InfluxDBSink) Init(cfg json.RawMessage) error {
	// 使用标准化配置解析
	standardConfig, err := s.ParseStandardConfig(cfg)
	if err != nil {
		return fmt.Errorf("解析InfluxDB sink配置失败: %w", err)
	}

	// 解析InfluxDB特定参数
	var influxConfig InfluxDBConfig
	if err := json.Unmarshal(standardConfig.Params, &influxConfig); err != nil {
		return fmt.Errorf("解析InfluxDB特定参数失败: %w", err)
	}

	s.defaultOrg = influxConfig.Org
	s.defaultBucket = influxConfig.Bucket
	s.pointsConfig = influxConfig.Points

	// 创建InfluxDB客户端
	options := influxdb2.DefaultOptions()
	if s.GetBatchSize() > 0 {
		options.SetBatchSize(uint(s.GetBatchSize()))
	}
	if influxConfig.FlushInterval > 0 {
		options.SetFlushInterval(uint(influxConfig.FlushInterval))
	}

	s.client = influxdb2.NewClientWithOptions(influxConfig.URL, influxConfig.Token, options)
	
	// 创建写入API（非阻塞）
	s.writeAPI = s.client.WriteAPI(influxConfig.Org, influxConfig.Bucket)
	
	// 设置错误处理
	errorsCh := s.writeAPI.Errors()
	go func() {
		for err := range errorsCh {
			s.HandleError(err, "InfluxDB写入")
		}
	}()

	log.Info().
		Str("name", s.Name()).
		Str("url", influxConfig.URL).
		Str("org", influxConfig.Org).
		Str("bucket", influxConfig.Bucket).
		Int("points_config", len(s.pointsConfig)).
		Int("batch_size", s.GetBatchSize()).
		Int("buffer_size", s.GetBufferSize()).
		Msg("InfluxDB连接器初始化完成")

	return nil
}

// Start 启动连接器
func (s *InfluxDBSink) Start(ctx context.Context) error {
	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 监听上下文取消
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.ctx.Done()

		// 刷新所有待处理的写入并关闭客户端
		s.writeAPI.Flush()
		s.client.Close()

		log.Info().Str("name", s.Name()).Msg("InfluxDB连接器上下文取消")
	}()

	log.Info().Str("name", s.Name()).Msg("InfluxDB连接器启动")
	return nil
}

// Publish 发布数据点到InfluxDB
func (s *InfluxDBSink) Publish(batch []model.Point) error {
	if !s.IsRunning() {
		return fmt.Errorf("InfluxDB连接器未启动")
	}

	if len(batch) == 0 {
		return nil
	}

	// 记录发布操作开始时间
	publishStart := time.Now()

	// 使用BaseSink的SafePublishBatch方法，自动处理统计
	return s.SafePublishBatch(batch, func(batch []model.Point) error {
		// 使用基础方法添加标签
		s.AddTags(batch)

		// 创建上下文
		ctx := context.Background()

		// 处理每个数据点
		for _, point := range batch {
			// 查找数据点配置
			config, found := s.pointsConfig[point.Key]
			if !found {
				// 如果没有特定配置，使用默认值
				config = PointConfig{
					Measurement: point.Key,
					Bucket:      s.defaultBucket,
					Org:         s.defaultOrg,
				}
			}

			// 确定测量名称
			measurement := config.Measurement
			if measurement == "" {
				measurement = point.Key
			}

			// 确定桶和组织
			bucket := config.Bucket
			if bucket == "" {
				bucket = s.defaultBucket
			}

			org := config.Org
			if org == "" {
				org = s.defaultOrg
			}

			// 创建InfluxDB数据点
			p := write.NewPoint(
				measurement,
				make(map[string]string),
				make(map[string]interface{}),
				time.Now(),
			)

			// 添加设备ID作为标签
			p.AddTag("device_id", point.DeviceID)

			// 添加所有原始标签
			for k, v := range point.Tags {
				// 检查是否应该作为字段而不是标签
				if contains(config.Fields, k) {
					p.AddField(k, v)
				} else {
					p.AddTag(k, v)
				}
			}

			// 添加自定义标签
			for k, v := range config.Tags {
				p.AddTag(k, v)
			}

			// 根据数据类型添加值字段
			switch point.Type {
			case model.TypeInt:
				// 确保整数类型正确
				var intValue int
				switch v := point.Value.(type) {
				case int:
					intValue = v
				case float64:
					intValue = int(v)
				default:
					return fmt.Errorf("无法将值转换为整数: %v", point.Value)
				}
				p.AddField("value", intValue)
			case model.TypeFloat:
				// 确保浮点类型正确
				var floatValue float64
				switch v := point.Value.(type) {
				case float64:
					floatValue = v
				case int:
					floatValue = float64(v)
				default:
					return fmt.Errorf("无法将值转换为浮点数: %v", point.Value)
				}
				p.AddField("value", floatValue)
			case model.TypeBool:
				// 确保布尔类型正确
				boolValue, ok := point.Value.(bool)
				if !ok {
					return fmt.Errorf("无法将值转换为布尔值: %v", point.Value)
				}
				p.AddField("value", boolValue)
			case model.TypeString:
				// 确保字符串类型正确
				strValue, ok := point.Value.(string)
				if !ok {
					strValue = fmt.Sprintf("%v", point.Value)
				}
				p.AddField("value", strValue)
			default:
				// 默认作为字符串处理
				p.AddField("value", fmt.Sprintf("%v", point.Value))
			}

			// 添加数据类型作为标签
			p.AddTag("value_type", string(point.Type))

			// 写入数据点
			if org != s.defaultOrg || bucket != s.defaultBucket {
				// 如果桶或组织与默认值不同，使用阻塞写入API
				writeAPI := s.client.WriteAPIBlocking(org, bucket)
				if err := writeAPI.WritePoint(ctx, p); err != nil {
					return fmt.Errorf("InfluxDB阻塞写入失败: %w", err)
				}
			} else {
				// 使用非阻塞写入API
				s.writeAPI.WritePoint(p)
			}

			log.Debug().
				Str("name", s.Name()).
				Str("key", point.Key).
				Str("device_id", point.DeviceID).
				Interface("value", point.Value).
				Str("type", string(point.Type)).
				Str("measurement", measurement).
				Str("org", org).
				Str("bucket", bucket).
				Msg("发布数据点到InfluxDB")
		}

		return nil
	}, publishStart)
}

// Stop 停止连接器
func (s *InfluxDBSink) Stop() error {
	s.SetRunning(false)

	if s.cancel != nil {
		s.cancel()
	}

	// 等待协程完成
	s.wg.Wait()

	// 刷新所有待处理的写入并关闭客户端
	if s.writeAPI != nil {
		s.writeAPI.Flush()
	}
	if s.client != nil {
		s.client.Close()
	}

	log.Info().Str("name", s.Name()).Msg("InfluxDB连接器停止")
	return nil
}

// Healthy 检查连接器健康状态
func (s *InfluxDBSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("InfluxDB连接器未运行")
	}
	if s.client == nil {
		return fmt.Errorf("InfluxDB客户端未初始化")
	}
	// 简单的健康检查 - 尝试ping服务器
	_, err := s.client.Health(context.Background())
	if err != nil {
		return fmt.Errorf("InfluxDB服务器健康检查失败: %w", err)
	}
	return nil
}

// 辅助函数：检查字符串是否在切片中
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}