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
	Address       string                 `json:"address"`      // Redis服务器地址
	Password      string                 `json:"password"`     // Redis密码
	Database      int                    `json:"database"`     // Redis数据库
	DefaultExpiry int                    `json:"default_expiry_sec"` // 默认过期时间（秒）
	Points        map[string]PointConfig `json:"points"`       // 数据点配置
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
	
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("连接Redis失败: %w", err)
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

		// 创建上下文
		ctx := context.Background()

		// 处理每个数据点
		for _, point := range batch {
			// 查找数据点配置
			config, found := s.pointsConfig[point.Key]
			if !found {
				// 如果没有特定配置，使用默认值
				config = PointConfig{
					KeyFormat: "{device_id}:{key}",
					Format:    "json",
				}
			}

			// 确定Redis键
			redisKey := s.formatKey(config, point)

			// 确定过期时间
			var expiry time.Duration
			if config.Expiry > 0 {
				expiry = time.Duration(config.Expiry) * time.Second
			} else {
				expiry = s.defaultExpiry
			}

			// 根据配置的格式存储数据
			format := config.Format
			if format == "" {
				format = "json"
			}

			var err error
			switch format {
			case "json":
				err = s.storeAsJSON(ctx, redisKey, point, expiry)
			case "string":
				err = s.storeAsString(ctx, redisKey, point, expiry)
			case "hash":
				err = s.storeAsHash(ctx, redisKey, point, config.HashFields, expiry)
			default:
				err = fmt.Errorf("不支持的存储格式: %s", format)
			}

			if err != nil {
				return fmt.Errorf("存储数据点到Redis失败: %w", err)
			}

			log.Debug().
				Str("name", s.Name()).
				Str("key", point.Key).
				Str("device_id", point.DeviceID).
				Interface("value", point.Value).
				Str("type", string(point.Type)).
				Str("redis_key", redisKey).
				Str("format", format).
				Msg("发布数据点到Redis")
		}

		return nil
	}, publishStart)
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
func (s *RedisSink) storeAsJSON(ctx context.Context, key string, point model.Point, expiry time.Duration) error {
	// 创建安全的Tags副本 - 使用GetTagsCopy()
	safeTags := point.GetTagsCopy()

	// 创建包含所有信息的JSON对象
	data := map[string]interface{}{
		"key":       point.Key,
		"device_id": point.DeviceID,
		"value":     s.convertValue(point),
		"type":      string(point.Type),
		"timestamp": time.Now().UnixNano() / int64(time.Millisecond),
		"tags":      safeTags,
	}

	// 序列化为JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// 存储到Redis
	if expiry > 0 {
		return s.client.Set(ctx, key, jsonData, expiry).Err()
	}
	return s.client.Set(ctx, key, jsonData, 0).Err()
}

// storeAsString 将数据点作为字符串存储
func (s *RedisSink) storeAsString(ctx context.Context, key string, point model.Point, expiry time.Duration) error {
	// 将值转换为字符串
	value := fmt.Sprintf("%v", s.convertValue(point))

	// 存储到Redis
	if expiry > 0 {
		return s.client.Set(ctx, key, value, expiry).Err()
	}
	return s.client.Set(ctx, key, value, 0).Err()
}

// storeAsHash 将数据点作为哈希表存储
func (s *RedisSink) storeAsHash(ctx context.Context, key string, point model.Point, fields []string, expiry time.Duration) error {
	// 创建哈希表字段
	hash := map[string]interface{}{
		"value":     s.convertValue(point),
		"type":      string(point.Type),
		"timestamp": time.Now().UnixNano() / int64(time.Millisecond),
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

	// 存储到Redis
	if err := s.client.HSet(ctx, key, hash).Err(); err != nil {
		return err
	}

	// 设置过期时间（如果指定）
	if expiry > 0 {
		return s.client.Expire(ctx, key, expiry).Err()
	}

	return nil
}

// convertValue 根据数据类型转换值
func (s *RedisSink) convertValue(point model.Point) interface{} {
	switch point.Type {
	case model.TypeInt:
		// 确保整数类型正确
		switch v := point.Value.(type) {
		case int:
			return v
		case float64:
			return int(v)
		default:
			return 0
		}
	case model.TypeFloat:
		// 确保浮点类型正确
		switch v := point.Value.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		default:
			return 0.0
		}
	case model.TypeBool:
		// 确保布尔类型正确
		if v, ok := point.Value.(bool); ok {
			return v
		}
		return false
	case model.TypeString:
		// 确保字符串类型正确
		if v, ok := point.Value.(string); ok {
			return v
		}
		return fmt.Sprintf("%v", point.Value)
	default:
		// 默认作为字符串处理
		return fmt.Sprintf("%v", point.Value)
	}
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