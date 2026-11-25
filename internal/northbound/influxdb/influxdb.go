package influxdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	influxhttp "github.com/influxdata/influxdb-client-go/v2/api/http"
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
	client        influxdb2.Client
	writeAPI      api.WriteAPI
	pointsConfig  map[string]PointConfig
	defaultBucket string
	defaultOrg    string
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	apiMu     sync.Mutex
	extraAPIs map[string]api.WriteAPI
	closeOnce sync.Once
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
	URL           string                 `json:"url"`               // InfluxDB服务器URL
	Token         string                 `json:"token"`             // InfluxDB认证令牌
	Org           string                 `json:"org"`               // 默认组织
	Bucket        string                 `json:"bucket"`            // 默认桶
	FlushInterval int                    `json:"flush_interval_ms"` // 刷新间隔(ms)
	Points        map[string]PointConfig `json:"points"`            // 数据点配置
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
		options.SetBatchSize(uint(max(s.GetBatchSize(), 1))) // #nosec G115 -- 已保证为正数
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
		s.closeClient()

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

	publishStart := time.Now()
	return s.SafePublishBatch(batch, func(batch []model.Point) error {
		s.AddTags(batch)

		var firstErr error
		skipped := 0
		for _, point := range batch {
			p, org, bucket, err := buildInfluxPoint(point, s.pointsConfig, s.defaultOrg, s.defaultBucket)
			if err != nil {
				// 单个点类型不兼容只跳过该点，不让整批数据失败
				skipped++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			s.writeAPIFor(org, bucket).WritePoint(p)
		}
		if skipped > 0 {
			log.Warn().Err(firstErr).Str("name", s.Name()).Int("skipped", skipped).Msg("部分数据点无法转换为InfluxDB格式，已跳过")
			if skipped == len(batch) {
				return firstErr
			}
		}
		return nil
	}, publishStart)
}

// writeAPIFor 返回指定 org/bucket 的非阻塞写入 API（按需创建并缓存）。
// 以前对非默认 bucket 的每个点都使用 WriteAPIBlocking 同步写入，会严重拖慢管道。
func (s *InfluxDBSink) writeAPIFor(org, bucket string) api.WriteAPI {
	if org == s.defaultOrg && bucket == s.defaultBucket {
		return s.writeAPI
	}
	key := org + "\x00" + bucket
	s.apiMu.Lock()
	defer s.apiMu.Unlock()
	if w, ok := s.extraAPIs[key]; ok {
		return w
	}
	if s.extraAPIs == nil {
		s.extraAPIs = make(map[string]api.WriteAPI)
	}
	w := s.client.WriteAPI(org, bucket)
	errCh := w.Errors()
	go func() {
		for err := range errCh {
			s.HandleError(err, "InfluxDB写入")
		}
	}()
	s.extraAPIs[key] = w
	return w
}

// buildInfluxPoint 把网关数据点转换为 InfluxDB 数据点。
// 时间戳使用数据点的采集时间（以前错误地使用了 time.Now()，
// 导致排队或补发的数据时间错误，也无法依靠相同时间戳覆盖实现幂等）。
func buildInfluxPoint(point model.Point, pointsConfig map[string]PointConfig, defaultOrg, defaultBucket string) (*write.Point, string, string, error) {
	config, found := pointsConfig[point.Key]
	if !found {
		config = PointConfig{}
	}
	measurement := config.Measurement
	if measurement == "" {
		measurement = point.Key
	}
	bucket := config.Bucket
	if bucket == "" {
		bucket = defaultBucket
	}
	org := config.Org
	if org == "" {
		org = defaultOrg
	}

	p := write.NewPointWithMeasurement(measurement).SetTime(northbound.PointTime(point))
	p.AddTag("device_id", point.DeviceID)
	for k, v := range point.GetTagsSafe() {
		if contains(config.Fields, k) {
			p.AddField(k, v)
		} else {
			p.AddTag(k, v)
		}
	}
	for k, v := range config.Tags {
		p.AddTag(k, v)
	}

	switch point.Type {
	case model.TypeInt, model.TypeFloat, model.TypeBool:
		v, ok := northbound.NormalizeValue(point)
		if !ok {
			return nil, "", "", fmt.Errorf("无法将值 %v (%T) 转换为 %s 类型", point.Value, point.Value, point.Type)
		}
		p.AddField("value", v)
	case model.TypeString:
		v, _ := northbound.NormalizeValue(point)
		p.AddField("value", v)
	default:
		// 复合类型：展开数值型的衍生字段，同时保留 JSON 形式的原始值
		if cd, ok := point.Value.(model.CompositeData); ok {
			for k, dv := range cd.GetDerivedValues() {
				if f, ok := northbound.ToFloat64(dv); ok {
					p.AddField(k, f)
				}
			}
		}
		if raw, err := json.Marshal(point.Value); err == nil {
			p.AddField("value", string(raw))
		} else {
			p.AddField("value", fmt.Sprintf("%v", point.Value))
		}
	}

	p.AddTag("value_type", string(point.Type))
	return p, org, bucket, nil
}

// Stop 停止连接器
func (s *InfluxDBSink) Stop() error {
	s.SetRunning(false)

	if s.cancel != nil {
		s.cancel()
	}

	// 等待协程完成
	s.wg.Wait()

	s.closeClient()

	log.Info().Str("name", s.Name()).Msg("InfluxDB连接器停止")
	return nil
}

// closeClient 刷新所有待写入数据并关闭客户端（只执行一次）
func (s *InfluxDBSink) closeClient() {
	s.closeOnce.Do(func() {
		if s.writeAPI != nil {
			s.writeAPI.Flush()
		}
		s.apiMu.Lock()
		for _, w := range s.extraAPIs {
			w.Flush()
		}
		s.apiMu.Unlock()
		if s.client != nil {
			s.client.Close()
		}
	})
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

// WriteBatch 同步写入（实现 northbound.BatchWriter，供本地缓冲/断网续传使用）。
// 使用阻塞 API，返回 nil 表示 InfluxDB 已确认写入；4xx（429 除外）视为永久错误。
func (s *InfluxDBSink) WriteBatch(ctx context.Context, batch []model.Point) error {
	if s.client == nil {
		return fmt.Errorf("InfluxDB客户端未初始化")
	}
	s.AddTags(batch)
	type target struct{ org, bucket string }
	groups := map[target][]*write.Point{}
	var convErr error
	for _, point := range batch {
		p, org, bucket, err := buildInfluxPoint(point, s.pointsConfig, s.defaultOrg, s.defaultBucket)
		if err != nil {
			convErr = err
			continue // 单个点类型不兼容只跳过该点
		}
		groups[target{org, bucket}] = append(groups[target{org, bucket}], p)
	}
	if len(groups) == 0 && convErr != nil {
		return fmt.Errorf("%w: %v", northbound.ErrPermanent, convErr)
	}
	for t, pts := range groups {
		if err := s.client.WriteAPIBlocking(t.org, t.bucket).WritePoint(ctx, pts...); err != nil {
			var he *influxhttp.Error
			if errors.As(err, &he) && he.StatusCode >= 400 && he.StatusCode < 500 && he.StatusCode != 429 {
				return fmt.Errorf("%w: %v", northbound.ErrPermanent, err)
			}
			return fmt.Errorf("InfluxDB写入失败: %w", err)
		}
	}
	return nil
}
