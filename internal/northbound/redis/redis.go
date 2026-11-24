package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/northbound"
)

func init() {
	// 注册连接器工厂
	northbound.Register("redis", func() northbound.Sink {
		return NewRedisSink()
	})
}

// NewRedisSink 创建一个新的Redis连接器
func NewRedisSink() *RedisSink {
	return &RedisSink{
		BaseSink: northbound.NewBaseSink("redis"),
	}
}

// RedisSink 是一个Redis连接器，用于将数据点存储到Redis数据库
type RedisSink struct {
	*northbound.BaseSink
	client        *redis.Client
	pointsConfig  map[string]PointConfig
	defaultExpiry time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// PointConfig 定义了数据点的Redis配置
type PointConfig struct {
	KeyPrefix  string            `json:"key_prefix"`  // Redis键前缀
	KeyFormat  string            `json:"key_format"`  // Redis键格式，支持 {device_id} 和 {key} 占位符
	Expiry     int               `json:"expiry_sec"`  // 过期时间（秒）
	Format     string            `json:"format"`      // 存储格式: json, string, hash
	HashFields []string          `json:"hash_fields"` // 用于hash格式的字段列表
	Tags       map[string]string `json:"tags"`        // 附加标签
}

// RedisConfig 是Redis连接器的特定参数配置
type RedisConfig struct {
	Address       string                 `json:"address"`            // Redis服务器地址
	Password      string                 `json:"password"`           // Redis密码
	Database      int                    `json:"database"`           // Redis数据库
	DefaultExpiry int                    `json:"default_expiry_sec"` // 默认过期时间（秒）
	Points        map[string]PointConfig `json:"points"`             // 数据点配置
}

// Init 初始化连接器
func (s *RedisSink) Init(cfg json.RawMessage) error {
	// 使用标准化配置解析
	standardConfig, err := s.ParseStandardConfig(cfg)
	if err != nil {
		return fmt.Errorf("解析Redis sink配置失败: %w", err)
	}

	// 解析Redis特定参数
	var redisConfig RedisConfig
	if err := json.Unmarshal(standardConfig.Params, &redisConfig); err != nil {
		return fmt.Errorf("解析Redis特定参数失败: %w", err)
	}

	s.pointsConfig = redisConfig.Points

	// 设置默认过期时间
	if redisConfig.DefaultExpiry > 0 {
		s.defaultExpiry = time.Duration(redisConfig.DefaultExpiry) * time.Second
	}

	// 创建Redis客户端
	s.client = redis.NewClient(&redis.Options{
		Addr:     redisConfig.Address,
		Password: redisConfig.Password,
		DB:       redisConfig.Database,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Redis 暂时不可达时不让初始化失败（客户端会自动重连）：
	// 以前这里返回错误，启动时 Redis 未就绪会导致该连接器被永久跳过
	if err := s.client.Ping(ctx).Err(); err != nil {
		log.Warn().Err(err).Str("name", s.Name()).Str("address", redisConfig.Address).Msg("Redis暂时不可达，将在写入时自动重连")
	}

	log.Info().
		Str("name", s.Name()).
		Str("address", redisConfig.Address).
		Int("database", redisConfig.Database).
		Int("points_config", len(s.pointsConfig)).
		Int("batch_size", s.GetBatchSize()).
		Int("buffer_size", s.GetBufferSize()).
		Msg("Redis连接器初始化完成")

	return nil
}

// Start 启动连接器
func (s *RedisSink) Start(ctx context.Context) error {
	s.SetRunning(true)
	s.ctx, s.cancel = context.WithCancel(ctx)

	// 监听上下文取消
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.ctx.Done()

		// 关闭Redis客户端
		if s.client != nil {
			s.client.Close()
		}

		log.Info().Str("name", s.Name()).Msg("Redis连接器上下文取消")
	}()

	log.Info().Str("name", s.Name()).Msg("Redis连接器启动")
	return nil
}

// Publish 发布数据点到Redis
func (s *RedisSink) Publish(batch []model.Point) error {
	if !s.IsRunning() {
		return fmt.Errorf("Redis连接器未启动")
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.writePipeline(ctx, batch)
	}, publishStart)
}

// WriteBatch 同步写入（实现 northbound.BatchWriter）：Pipeline 执行成功即表示 Redis 已确认
func (s *RedisSink) WriteBatch(ctx context.Context, batch []model.Point) error {
	if s.client == nil {
		return fmt.Errorf("Redis客户端未初始化")
	}
	s.AddTags(batch)
	return s.writePipeline(ctx, batch)
}

// writePipeline 整批使用 Pipeline，一次网络往返完成所有写入
// （以前每个点单独 SET，一次往返一个点，是整个网关的吞吐瓶颈）
func (s *RedisSink) writePipeline(ctx context.Context, batch []model.Point) error {
	pipe := s.client.Pipeline()
	for _, point := range batch {
		if err := s.queuePoint(ctx, pipe, point); err != nil {
			return fmt.Errorf("%w: 存储数据点到Redis失败: %v", northbound.ErrPermanent, err)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("Redis批量写入失败: %w", err)
	}
	return nil
}

// queuePoint 把单个数据点的写入命令加入 Pipeline
func (s *RedisSink) queuePoint(ctx context.Context, pipe redis.Pipeliner, point model.Point) error {
	config, found := s.pointsConfig[point.Key]
	if !found {
		config = PointConfig{
			KeyFormat: "{device_id}:{key}",
			Format:    "json",
		}
	}
	redisKey := s.formatKey(config, point)

	expiry := s.defaultExpiry
	if config.Expiry > 0 {
		expiry = time.Duration(config.Expiry) * time.Second
	}

	format := config.Format
	if format == "" {
		format = "json"
	}
	switch format {
	case "json":
		return s.storeAsJSON(ctx, pipe, redisKey, point, expiry)
	case "string":
		return s.storeAsString(ctx, pipe, redisKey, point, expiry)
	case "hash":
		return s.storeAsHash(ctx, pipe, redisKey, point, config.HashFields, expiry)
	default:
		return fmt.Errorf("不支持的存储格式: %s", format)
	}
}

// formatKey 格式化Redis键
func (s *RedisSink) formatKey(config PointConfig, point model.Point) string {
	keyFormat := config.KeyFormat
	if keyFormat == "" {
		keyFormat = "{device_id}:{key}"
	}

	// 替换占位符
	key := keyFormat
	key = strings.ReplaceAll(key, "{device_id}", point.DeviceID)
	key = strings.ReplaceAll(key, "{key}", point.Key)

	// 添加前缀
	if config.KeyPrefix != "" {
		key = config.KeyPrefix + ":" + key
	}

	return key
}

// storeAsJSON 将数据点作为JSON存储
func (s *RedisSink) storeAsJSON(ctx context.Context, c redis.Cmdable, key string, point model.Point, expiry time.Duration) error {
	// 创建安全的Tags副本 - 使用GetTagsCopy()
	safeTags := point.GetTagsCopy()

	// 创建包含所有信息的JSON对象
	data := map[string]interface{}{
		"key":       point.Key,
		"device_id": point.DeviceID,
		"value":     s.convertValue(point),
		"type":      string(point.Type),
		"timestamp": northbound.PointTime(point).UnixMilli(),
		"tags":      safeTags,
	}

	// 序列化为JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	c.Set(ctx, key, jsonData, expiry)
	return nil
}

// storeAsString 将数据点作为字符串存储
func (s *RedisSink) storeAsString(ctx context.Context, c redis.Cmdable, key string, point model.Point, expiry time.Duration) error {
	// 将值转换为字符串
	value := fmt.Sprintf("%v", s.convertValue(point))

	c.Set(ctx, key, value, expiry)
	return nil
}

// storeAsHash 将数据点作为哈希表存储
func (s *RedisSink) storeAsHash(ctx context.Context, c redis.Cmdable, key string, point model.Point, fields []string, expiry time.Duration) error {
	// 创建哈希表字段
	hash := map[string]interface{}{
		"value":     s.convertValue(point),
		"type":      string(point.Type),
		"timestamp": northbound.PointTime(point).UnixMilli(),
	}

	// 添加设备ID和键
	hash["device_id"] = point.DeviceID
	hash["key"] = point.Key

	// 添加指定的标签字段
	if len(fields) > 0 {
		for _, field := range fields {
			// Go 1.24安全：使用GetTag方法替代直接Tags[]访问
			if value, exists := point.GetTag(field); exists {
				hash[field] = value
			}
		}
	} else {
		// 如果未指定字段，添加所有标签（使用安全方式）
		pointTags := point.GetTagsCopy()
		for k, v := range pointTags {
			hash[k] = v
		}
	}

	c.HSet(ctx, key, hash)
	if expiry > 0 {
		c.Expire(ctx, key, expiry)
	}
	return nil
}

// convertValue 根据数据类型转换值
func (s *RedisSink) convertValue(point model.Point) interface{} {
	return northbound.NormalizeValueOrZero(point)
}

// Stop 停止连接器
func (s *RedisSink) Stop() error {
	s.SetRunning(false)

	if s.cancel != nil {
		s.cancel()
	}

	// 等待协程完成
	s.wg.Wait()

	// 关闭Redis客户端
	if s.client != nil {
		s.client.Close()
	}

	log.Info().Str("name", s.Name()).Msg("Redis连接器停止")
	return nil
}

// Healthy 检查连接器健康状态
func (s *RedisSink) Healthy() error {
	if !s.IsRunning() {
		return fmt.Errorf("Redis连接器未运行")
	}
	if s.client == nil {
		return fmt.Errorf("Redis客户端未初始化")
	}
	// 简单的健康检查 - 尝试ping服务器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis服务器健康检查失败: %w", err)
	}
	return nil
}
