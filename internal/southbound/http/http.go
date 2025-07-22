package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func init() {
	// 注册适配器工厂
	southbound.Register("http", func() southbound.Adapter {
		return &HTTPAdapter{}
	})
}

// HTTPAdapter 是一个HTTP适配器，用于从HTTP API获取数据
type HTTPAdapter struct {
	*southbound.BaseAdapter
	client    *http.Client
	endpoints []Endpoint
	deviceID  string
	interval  time.Duration
	stopCh    chan struct{}
	mutex     sync.Mutex
	running   bool
}

// Endpoint 定义了要请求的HTTP端点
type Endpoint struct {
	URL        string            `json:"url"`         // HTTP端点URL
	Method     string            `json:"method"`      // HTTP方法: GET, POST, PUT等
	Headers    map[string]string `json:"headers"`     // HTTP请求头
	Body       string            `json:"body"`        // 请求体（用于POST/PUT）
	DataPoints []DataPoint       `json:"data_points"` // 要从响应中提取的数据点
	Timeout    int               `json:"timeout_ms"`  // 请求超时(ms)
}

// DataPoint 定义了从HTTP响应中提取的数据点
type DataPoint struct {
	Key      string            `json:"key"`            // 数据点标识符
	Path     string            `json:"path"`           // JSON路径，用于从响应中提取值
	Type     string            `json:"type"`           // 数据类型: int, float, bool, string
	DeviceID string            `json:"device_id"`      // 设备ID，如果为空则使用适配器默认值
	Tags     map[string]string `json:"tags,omitempty"` // 附加标签
}

// HTTPConfig 是HTTP适配器的配置
type HTTPConfig struct {
	Name      string     `json:"name"`
	DeviceID  string     `json:"device_id"`   // 默认设备ID
	Interval  int        `json:"interval_ms"` // 采样间隔(ms)
	Endpoints []Endpoint `json:"endpoints"`   // 要请求的HTTP端点列表
	Timeout   int        `json:"timeout_ms"`  // 默认超时时间(ms)
}

// Name 返回适配器名称
func (a *HTTPAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *HTTPAdapter) Init(cfg json.RawMessage) error {
	var config HTTPConfig
	if err := json.Unmarshal(cfg, &config); err != nil {
		return err
	}

	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "http")
	a.deviceID = config.DeviceID
	a.interval = time.Duration(config.Interval) * time.Millisecond
	if a.interval < 100*time.Millisecond {
		a.interval = 100 * time.Millisecond // 最小间隔100ms
	}
	a.endpoints = config.Endpoints
	a.stopCh = make(chan struct{})

	// 创建HTTP客户端
	timeout := time.Duration(config.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 5000 * time.Millisecond // 默认超时5秒
	}
	a.client = &http.Client{
		Timeout: timeout,
	}

	log.Info().
		Str("name", a.Name()).
		Str("device_id", a.deviceID).
		Int("endpoints", len(a.endpoints)).
		Dur("interval", a.interval).
		Msg("HTTP适配器初始化完成")

	return nil
}

// Start 启动适配器
func (a *HTTPAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return nil
	}
	a.running = true

	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 请求所有端点
				for _, endpoint := range a.endpoints {
					// 创建请求
					req, err := a.createRequest(endpoint)
					if err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("创建HTTP请求失败")
						continue
					}

					// 设置超时
					timeout := time.Duration(endpoint.Timeout) * time.Millisecond
					if timeout > 0 {
						reqCtx, cancel := context.WithTimeout(ctx, timeout)
						req = req.WithContext(reqCtx)
						defer cancel()
					}

					// 记录请求开始时间
					requestStart := time.Now()
					
					// 发送请求
					resp, err := a.client.Do(req)
					if err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("HTTP请求失败")
						continue
					}

					// 确保响应体被关闭
					defer resp.Body.Close()

					// 检查响应状态码
					if resp.StatusCode < 200 || resp.StatusCode >= 300 {
						log.Error().
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Int("status", resp.StatusCode).
							Msg("HTTP请求返回非成功状态码")
						continue
					}

					// 读取响应体
					body, err := io.ReadAll(resp.Body)
					if err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("读取HTTP响应失败")
						continue
					}

					// 解析JSON响应
					var data map[string]interface{}
					if err := json.Unmarshal(body, &data); err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("url", endpoint.URL).
							Msg("解析HTTP响应JSON失败")
						continue
					}

					// 处理每个数据点
					for _, dp := range endpoint.DataPoints {
						// 提取值
						value, err := a.extractValue(data, dp.Path)
						if err != nil {
							log.Error().
								Err(err).
								Str("name", a.Name()).
								Str("url", endpoint.URL).
								Str("path", dp.Path).
								Msg("从HTTP响应中提取值失败")
							continue
						}

						// 确定设备ID
						deviceID := dp.DeviceID
						if deviceID == "" {
							deviceID = a.deviceID
						}
						if deviceID == "" {
							deviceID = "http"
						}

						// 根据类型转换值
						var dataType model.DataType
						switch dp.Type {
						case "int":
							switch v := value.(type) {
							case float64:
								value = int(v)
							case string:
								var intVal int
								if _, err := fmt.Sscanf(v, "%d", &intVal); err != nil {
									log.Error().
										Str("name", a.Name()).
										Str("url", endpoint.URL).
										Str("value", v).
										Msg("无法将字符串转换为整数")
									continue
								}
								value = intVal
							}
							dataType = model.TypeInt
						case "float":
							switch v := value.(type) {
							case int:
								value = float64(v)
							case string:
								var floatVal float64
								if _, err := fmt.Sscanf(v, "%f", &floatVal); err != nil {
									log.Error().
										Str("name", a.Name()).
										Str("url", endpoint.URL).
										Str("value", v).
										Msg("无法将字符串转换为浮点数")
									continue
								}
								value = floatVal
							}
							dataType = model.TypeFloat
						case "bool":
							switch v := value.(type) {
							case string:
								switch v {
								case "true", "1", "on", "yes":
									value = true
								case "false", "0", "off", "no":
									value = false
								default:
									log.Error().
										Str("name", a.Name()).
										Str("url", endpoint.URL).
										Str("value", v).
										Msg("无法将字符串转换为布尔值")
									continue
								}
							case float64:
								value = v != 0
							case int:
								value = v != 0
							}
							dataType = model.TypeBool
						case "string":
							if _, ok := value.(string); !ok {
								value = fmt.Sprintf("%v", value)
							}
							dataType = model.TypeString
						default:
							// 默认为字符串类型
							if _, ok := value.(string); !ok {
								value = fmt.Sprintf("%v", value)
							}
							dataType = model.TypeString
						}

						// 创建数据点
						point := model.NewPoint(dp.Key, deviceID, value, dataType)

						// 添加标签
						point.AddTag("source", "http")
						point.AddTag("url", endpoint.URL)

						// 添加自定义标签
						for k, v := range dp.Tags {
							point.AddTag(k, v)
						}

						// 使用BaseAdapter的SafeSendDataPoint方法，自动处理统计
						a.SafeSendDataPoint(ch, point, requestStart)
						
						log.Debug().
							Str("name", a.Name()).
							Str("key", dp.Key).
							Str("url", endpoint.URL).
							Interface("value", value).
							Str("type", string(dataType)).
							Msg("发送HTTP数据点")
					}
				}
			case <-a.stopCh:
				log.Info().Str("name", a.Name()).Msg("HTTP适配器停止")
				return
			case <-ctx.Done():
				log.Info().Str("name", a.Name()).Msg("HTTP适配器上下文取消")
				return
			}
		}
	}()

	log.Info().Str("name", a.Name()).Msg("HTTP适配器启动")
	return nil
}

// createRequest 创建HTTP请求
func (a *HTTPAdapter) createRequest(endpoint Endpoint) (*http.Request, error) {
	method := strings.ToUpper(endpoint.Method)
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if endpoint.Body != "" && (method == "POST" || method == "PUT" || method == "PATCH") {
		body = strings.NewReader(endpoint.Body)
	}

	req, err := http.NewRequest(method, endpoint.URL, body)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	for k, v := range endpoint.Headers {
		req.Header.Set(k, v)
	}

	// 如果没有设置Content-Type，且有请求体，默认为JSON
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// extractValue 从JSON中提取值
func (a *HTTPAdapter) extractValue(data map[string]interface{}, path string) (interface{}, error) {
	// 处理嵌套路径，如 "data.temperature"
	parts := strings.Split(path, ".")
	current := data

	// 遍历路径的每一部分，直到最后一个
	for i, part := range parts {
		if i == len(parts)-1 {
			// 最后一个部分，返回值
			if value, ok := current[part]; ok {
				return value, nil
			}
			return nil, fmt.Errorf("路径 %s 不存在", path)
		}

		// 不是最后一个部分，继续向下遍历
		next, ok := current[part]
		if !ok {
			return nil, fmt.Errorf("路径 %s 的部分 %s 不存在", path, part)
		}

		// 确保下一级是一个对象
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("路径 %s 的部分 %s 不是一个对象", path, part)
		}

		current = nextMap
	}

	return nil, fmt.Errorf("无效的路径: %s", path)
}

// Stop 停止适配器
func (a *HTTPAdapter) Stop() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running {
		return nil
	}

	close(a.stopCh)
	a.running = false
	return nil
}

// NewAdapter 创建一个新的HTTP适配器实例
func NewAdapter() southbound.Adapter {
	return &HTTPAdapter{}
}
