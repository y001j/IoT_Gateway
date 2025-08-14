package modbus

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/goburrow/modbus"
	"github.com/rs/zerolog/log"
	"github.com/y001j/iot-gateway/internal/config"
	"github.com/y001j/iot-gateway/internal/model"
	"github.com/y001j/iot-gateway/internal/southbound"
)

func init() {
	// 注册适配器工厂
	southbound.Register("modbus", func() southbound.Adapter {
		return &ModbusAdapter{}
	})
}

// ModbusAdapter 是一个Modbus适配器，支持TCP和RTU模式
type ModbusAdapter struct {
	*southbound.BaseAdapter
	mode       string // "tcp" 或 "rtu"
	interval   time.Duration
	client     modbus.Client
	handler    *modbus.TCPClientHandler
	rtuHandler *modbus.RTUClientHandler
	registers  []config.ModbusRegister
	stopCh     chan struct{}
	mutex      sync.Mutex
	running    bool
	// 添加重连相关字段
	maxRetries    int
	retryInterval time.Duration
	connected     bool
	parser        *config.ConfigParser[config.ModbusConfig]
}

// Name 返回适配器名称
func (a *ModbusAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *ModbusAdapter) Init(cfg json.RawMessage) error {
	// 创建配置解析器
	a.parser = config.NewParserWithDefaults(config.GetDefaultModbusConfig())
	
	// 解析配置
	modbusConfig, err := a.parser.Parse(cfg)
	if err != nil {
		return fmt.Errorf("解析Modbus配置失败: %w", err)
	}

	return a.initWithConfig(modbusConfig)
}

// initWithConfig 使用新配置格式初始化
func (a *ModbusAdapter) initWithConfig(config *config.ModbusConfig) error {
	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "modbus")
	a.mode = config.Protocol
	a.interval = config.Interval.Duration()
	a.registers = config.Registers
	a.stopCh = make(chan struct{})

	// 设置重连参数
	a.maxRetries = 5 // 默认值，可从配置扩展
	a.retryInterval = 5 * time.Second

	// 创建Modbus客户端
	switch a.mode {
	case "tcp":
		a.handler = modbus.NewTCPClientHandler(fmt.Sprintf("%s:%d", config.Host, config.Port))
		a.handler.Timeout = config.Timeout.Duration()
		a.handler.SlaveId = config.SlaveID
		a.client = modbus.NewClient(a.handler)
	case "rtu":
		// RTU mode would need additional configuration
		return fmt.Errorf("RTU mode not yet implemented with new config")
	default:
		return fmt.Errorf("不支持的Modbus模式: %s", a.mode)
	}

	log.Info().
		Str("name", a.Name()).
		Str("mode", a.mode).
		Str("host", config.Host).
		Int("port", config.Port).
		Int("registers", len(a.registers)).
		Dur("interval", a.interval).
		Msg("Modbus适配器初始化完成")

	return nil
}

// connect 连接到Modbus设备，支持重试
func (a *ModbusAdapter) connect() error {
	var err error

	for retry := 0; retry <= a.maxRetries; retry++ {
		switch a.mode {
		case "tcp":
			err = a.handler.Connect()
		case "rtu":
			err = a.rtuHandler.Connect()
		}

		if err == nil {
			a.connected = true
			log.Info().
				Str("name", a.Name()).
				Str("mode", a.mode).
				Msg("Modbus设备连接成功")
			return nil
		}

		if retry < a.maxRetries {
			log.Warn().
				Err(err).
				Str("name", a.Name()).
				Int("retry", retry+1).
				Int("max_retries", a.maxRetries).
				Dur("retry_interval", a.retryInterval).
				Msg("Modbus连接失败，准备重试")
			time.Sleep(a.retryInterval)
		}
	}

	return fmt.Errorf("连接Modbus设备失败，已重试%d次: %w", a.maxRetries, err)
}

// disconnect 断开连接
func (a *ModbusAdapter) disconnect() {
	if !a.connected {
		return
	}

	switch a.mode {
	case "tcp":
		a.handler.Close()
	case "rtu":
		a.rtuHandler.Close()
	}

	a.connected = false
	log.Info().Str("name", a.Name()).Msg("Modbus设备连接已断开")
}

// Start 启动适配器
func (a *ModbusAdapter) Start(ctx context.Context, ch chan<- model.Point) error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.running {
		return nil
	}
	a.running = true

	// 连接Modbus设备
	if err := a.connect(); err != nil {
		a.running = false
		return err
	}

	// 启动数据采集协程
	go func() {
		ticker := time.NewTicker(a.interval)
		defer ticker.Stop()
		defer func() {
			a.disconnect()
			a.mutex.Lock()
			a.running = false
			a.mutex.Unlock()
		}()

		for {
			select {
			case <-ticker.C:
				// 记录数据采集开始时间
				pollStart := time.Now()
				
				// 检查连接状态
				if !a.connected {
					log.Warn().Str("name", a.Name()).Msg("设备未连接，尝试重新连接")
					if err := a.connect(); err != nil {
						log.Error().Err(err).Str("name", a.Name()).Msg("重新连接失败")
						continue
					}
				}

				// 读取所有配置的寄存器
				for _, reg := range a.registers {
					if err := a.readNewRegister(reg, ch, pollStart); err != nil {
						log.Error().
							Err(err).
							Str("name", a.Name()).
							Str("key", reg.Key).
							Msg("读取寄存器失败")

						// 如果是连接错误，标记为未连接
						if isConnectionError(err) {
							a.connected = false
						}
					}
				}
			case <-a.stopCh:
				log.Info().Str("name", a.Name()).Msg("Modbus适配器停止")
				return
			case <-ctx.Done():
				log.Info().Str("name", a.Name()).Msg("Modbus适配器上下文取消")
				return
			}
		}
	}()

	log.Info().Str("name", a.Name()).Msg("Modbus适配器启动")
	return nil
}

// readNewRegister 读取单个寄存器（新格式）
func (a *ModbusAdapter) readNewRegister(reg config.ModbusRegister, ch chan<- model.Point, pollStart time.Time) error {
	// 根据寄存器类型读取数据
	var result []byte
	var err error

	switch reg.Type {
	case "coil":
		result, err = a.client.ReadCoils(reg.Address, 1)
	case "discrete_input":
		result, err = a.client.ReadDiscreteInputs(reg.Address, 1)
	case "input_register":
		result, err = a.client.ReadInputRegisters(reg.Address, getRegisterCount(reg.DataType))
	case "holding_register":
		result, err = a.client.ReadHoldingRegisters(reg.Address, getRegisterCount(reg.DataType))
	default:
		return fmt.Errorf("不支持的寄存器类型: %s", reg.Type)
	}

	if err != nil {
		return fmt.Errorf("读取寄存器失败: %w", err)
	}

	// 解析数据
	value, dataType, err := a.parseNewData(result, reg)
	if err != nil {
		return fmt.Errorf("解析数据失败: %w", err)
	}

	if value != nil {
		// 创建数据点
		point := model.NewPoint(reg.Key, reg.DeviceID, value, dataType)

		// 添加标签
		point.AddTag("source", "modbus")
		point.AddTag("mode", a.mode)
		point.AddTag("address", fmt.Sprintf("%d", reg.Address))
		point.AddTag("type", reg.Type)

		// 发送数据点
		a.SafeSendDataPoint(ch, point, pollStart)
	}

	return nil
}

// parseNewData 解析Modbus数据使用新配置格式
func (a *ModbusAdapter) parseNewData(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	if len(result) == 0 {
		return nil, "", fmt.Errorf("读取数据为空")
	}

	switch reg.Type {
	case "coil", "discrete_input":
		return a.parseNewBool(result)
	case "input_register", "holding_register":
		return a.parseNewRegisterData(result, reg)
	default:
		return nil, "", fmt.Errorf("不支持的寄存器类型: %s", reg.Type)
	}
}

// parseNewBool 解析布尔数据
func (a *ModbusAdapter) parseNewBool(result []byte) (interface{}, model.DataType, error) {
	if len(result) > 0 {
		return (result[0] & 0x01) == 1, model.TypeBool, nil
	}
	return nil, "", fmt.Errorf("无法解析布尔值")
}

// parseNewRegisterData 根据数据类型解析寄存器数据
func (a *ModbusAdapter) parseNewRegisterData(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	switch reg.DataType {
	case "uint16":
		return a.parseUInt16(result, reg)
	case "int16":
		return a.parseInt16New(result, reg)
	case "uint32":
		return a.parseUInt32(result, reg)
	case "int32":
		return a.parseInt32New(result, reg)
	case "float32":
		return a.parseFloat32New(result, reg)
	default:
		// Default to int16 for backward compatibility
		return a.parseInt16New(result, reg)
	}
}

// parseUInt16 解析16位无符号整数
func (a *ModbusAdapter) parseUInt16(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	if len(result) < 2 {
		return nil, "", fmt.Errorf("数据长度不足，需要2字节")
	}

	value := binary.BigEndian.Uint16(result[:2])
	scaledValue := uint64(float64(value)*reg.Scale + reg.Offset)
	return scaledValue, model.TypeInt, nil
}

// parseInt16New 解析16位有符号整数（新版本）
func (a *ModbusAdapter) parseInt16New(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	if len(result) < 2 {
		return nil, "", fmt.Errorf("数据长度不足，需要2字节")
	}

	value := int16(binary.BigEndian.Uint16(result[:2]))
	scaledValue := int64(float64(value)*reg.Scale + reg.Offset)
	return scaledValue, model.TypeInt, nil
}

// parseUInt32 解析32位无符号整数
func (a *ModbusAdapter) parseUInt32(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	if len(result) < 4 {
		return nil, "", fmt.Errorf("数据长度不足，需要4字节")
	}

	value := binary.BigEndian.Uint32(result[:4])
	scaledValue := uint64(float64(value)*reg.Scale + reg.Offset)
	return scaledValue, model.TypeInt, nil
}

// parseInt32New 解析32位有符号整数（新版本）
func (a *ModbusAdapter) parseInt32New(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	if len(result) < 4 {
		return nil, "", fmt.Errorf("数据长度不足，需要4字节")
	}

	value := int32(binary.BigEndian.Uint32(result[:4]))
	scaledValue := int64(float64(value)*reg.Scale + reg.Offset)
	return scaledValue, model.TypeInt, nil
}

// parseFloat32New 解析32位浮点数（新版本）
func (a *ModbusAdapter) parseFloat32New(result []byte, reg config.ModbusRegister) (interface{}, model.DataType, error) {
	if len(result) < 4 {
		return nil, "", fmt.Errorf("数据长度不足，需要4字节")
	}

	bits := binary.BigEndian.Uint32(result[:4])
	value := math.Float32frombits(bits)
	scaledValue := float64(value)*reg.Scale + reg.Offset

	return scaledValue, model.TypeFloat, nil
}

// getRegisterCount 返回数据类型需要的寄存器数量
func getRegisterCount(dataType string) uint16 {
	switch dataType {
	case "uint16", "int16":
		return 1
	case "uint32", "int32", "float32":
		return 2
	default:
		return 1
	}
}

// isConnectionError 判断是否为连接错误
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return contains(errStr, "connection") ||
		contains(errStr, "timeout") ||
		contains(errStr, "refused") ||
		contains(errStr, "broken pipe") ||
		contains(errStr, "reset by peer")
}

// contains 检查字符串是否包含子字符串（忽略大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				anyMatch(s, substr)))
}

func anyMatch(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// Stop 停止适配器
func (a *ModbusAdapter) Stop() error {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if !a.running {
		return nil
	}

	close(a.stopCh)
	a.running = false
	return nil
}

// NewAdapter 创建一个新的Modbus适配器实例
func NewAdapter() southbound.Adapter {
	return &ModbusAdapter{}
}