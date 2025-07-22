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
	registers  []Register
	stopCh     chan struct{}
	mutex      sync.Mutex
	running    bool
	// 添加重连相关字段
	maxRetries    int
	retryInterval time.Duration
	connected     bool
}

// Register 定义了要读取的Modbus寄存器
type Register struct {
	Key       string            `json:"key"`            // 数据点标识符
	Address   uint16            `json:"address"`        // 寄存器地址
	Quantity  uint16            `json:"quantity"`       // 读取数量
	Type      string            `json:"type"`           // 数据类型: int16, int32, float32, bool, string
	Function  uint8             `json:"function"`       // 功能码: 1=读线圈, 2=读离散输入, 3=读保持寄存器, 4=读输入寄存器
	Scale     float64           `json:"scale"`          // 缩放因子，默认为1
	ByteOrder string            `json:"byte_order"`     // 字节序: big_endian, little_endian
	BitOffset int               `json:"bit_offset"`     // 位偏移，用于从寄存器中提取布尔值
	DeviceID  byte              `json:"device_id"`      // Modbus设备ID/从站地址
	Tags      map[string]string `json:"tags,omitempty"` // 附加标签
}

// ModbusConfig 是Modbus适配器的配置
type ModbusConfig struct {
	Name      string     `json:"name"`
	Mode      string     `json:"mode"`        // "tcp" 或 "rtu"
	Address   string     `json:"address"`     // TCP模式: "host:port", RTU模式: 串口名称
	Timeout   int        `json:"timeout_ms"`  // 超时时间(ms)
	Interval  int        `json:"interval_ms"` // 采样间隔(ms)
	Registers []Register `json:"registers"`
	// 添加重连配置
	MaxRetries    int `json:"max_retries,omitempty"`    // 最大重试次数，默认5
	RetryInterval int `json:"retry_interval,omitempty"` // 重试间隔(ms)，默认5000
	// RTU特有配置
	BaudRate int    `json:"baud_rate,omitempty"`
	DataBits int    `json:"data_bits,omitempty"`
	Parity   string `json:"parity,omitempty"`
	StopBits int    `json:"stop_bits,omitempty"`
	// TCP特有配置
	UnitID byte `json:"unit_id,omitempty"`
}

// Name 返回适配器名称
func (a *ModbusAdapter) Name() string {
	return a.BaseAdapter.Name()
}

// Init 初始化适配器
func (a *ModbusAdapter) Init(cfg json.RawMessage) error {
	var config ModbusConfig
	if err := json.Unmarshal(cfg, &config); err != nil {
		return fmt.Errorf("解析Modbus配置失败: %w", err)
	}

	// 验证必需字段
	if config.Name == "" {
		return fmt.Errorf("适配器名称不能为空")
	}
	if config.Mode == "" {
		config.Mode = "tcp" // 默认TCP模式
	}
	if config.Address == "" {
		return fmt.Errorf("地址不能为空")
	}

	// 初始化BaseAdapter
	a.BaseAdapter = southbound.NewBaseAdapter(config.Name, "modbus")
	a.mode = config.Mode
	a.interval = time.Duration(config.Interval) * time.Millisecond
	if a.interval < 100*time.Millisecond {
		a.interval = 1000 * time.Millisecond // 默认1秒间隔
	}

	// 设置重连参数
	a.maxRetries = config.MaxRetries
	if a.maxRetries <= 0 {
		a.maxRetries = 5 // 默认最大重试5次
	}
	a.retryInterval = time.Duration(config.RetryInterval) * time.Millisecond
	if a.retryInterval <= 0 {
		a.retryInterval = 5000 * time.Millisecond // 默认5秒重试间隔
	}

	// 验证和处理寄存器配置
	for i := range config.Registers {
		reg := &config.Registers[i]

		// 设置默认缩放因子
		if reg.Scale == 0 {
			reg.Scale = 1.0
		}

		// 验证数据类型和数量的匹配
		if err := a.validateRegister(reg); err != nil {
			return fmt.Errorf("寄存器 %s 配置错误: %w", reg.Key, err)
		}

		// 设置默认字节序
		if reg.ByteOrder == "" {
			reg.ByteOrder = "big_endian"
		}
	}

	a.registers = config.Registers
	a.stopCh = make(chan struct{})

	// 创建Modbus客户端
	timeout := time.Duration(config.Timeout) * time.Millisecond
	if timeout == 0 {
		timeout = 1000 * time.Millisecond // 默认超时1秒
	}

	switch a.mode {
	case "tcp":
		a.handler = modbus.NewTCPClientHandler(config.Address)
		a.handler.Timeout = timeout
		if config.UnitID > 0 {
			a.handler.SlaveId = config.UnitID
		}
		a.client = modbus.NewClient(a.handler)
	case "rtu":
		a.rtuHandler = modbus.NewRTUClientHandler(config.Address)
		a.rtuHandler.Timeout = timeout
		if config.BaudRate > 0 {
			a.rtuHandler.BaudRate = config.BaudRate
		}
		if config.DataBits > 0 {
			a.rtuHandler.DataBits = config.DataBits
		}
		if config.Parity != "" {
			a.rtuHandler.Parity = config.Parity
		}
		if config.StopBits > 0 {
			a.rtuHandler.StopBits = config.StopBits
		}
		a.client = modbus.NewClient(a.rtuHandler)
	default:
		return fmt.Errorf("不支持的Modbus模式: %s", a.mode)
	}

	log.Info().
		Str("name", a.Name()).
		Str("mode", a.mode).
		Str("address", config.Address).
		Int("registers", len(a.registers)).
		Dur("interval", a.interval).
		Int("max_retries", a.maxRetries).
		Msg("Modbus适配器初始化完成")

	return nil
}

// validateRegister 验证寄存器配置
func (a *ModbusAdapter) validateRegister(reg *Register) error {
	if reg.Key == "" {
		return fmt.Errorf("寄存器key不能为空")
	}

	if reg.Function < 1 || reg.Function > 4 {
		return fmt.Errorf("不支持的功能码: %d", reg.Function)
	}

	// 验证数据类型和数量的匹配
	switch reg.Type {
	case "bool":
		if reg.Function == 1 || reg.Function == 2 {
			// 线圈和离散输入，数量应该为1
			if reg.Quantity != 1 {
				reg.Quantity = 1
			}
		} else {
			// 寄存器类型，数量应该为1
			if reg.Quantity != 1 {
				reg.Quantity = 1
			}
		}
	case "int16":
		if reg.Quantity != 1 {
			reg.Quantity = 1
		}
	case "int32", "float32":
		if reg.Quantity != 2 {
			log.Warn().
				Str("key", reg.Key).
				Str("type", reg.Type).
				Uint16("quantity", reg.Quantity).
				Msg("32位数据类型需要2个寄存器，自动调整数量为2")
			reg.Quantity = 2
		}
	case "string":
		if reg.Quantity == 0 {
			reg.Quantity = 1
		}
	default:
		// 默认为int16
		reg.Type = "int16"
		if reg.Quantity == 0 {
			reg.Quantity = 1
		}
	}

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
					if err := a.readRegister(reg, ch, pollStart); err != nil {
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

// readRegister 读取单个寄存器
func (a *ModbusAdapter) readRegister(reg Register, ch chan<- model.Point, pollStart time.Time) error {
	// 设置从站地址
	switch a.mode {
	case "tcp":
		a.handler.SlaveId = reg.DeviceID
	case "rtu":
		a.rtuHandler.SlaveId = reg.DeviceID
	}

	// 根据功能码读取不同类型的寄存器
	var result []byte
	var err error

	switch reg.Function {
	case 1: // 读线圈
		result, err = a.client.ReadCoils(reg.Address, reg.Quantity)
	case 2: // 读离散输入
		result, err = a.client.ReadDiscreteInputs(reg.Address, reg.Quantity)
	case 3: // 读保持寄存器
		result, err = a.client.ReadHoldingRegisters(reg.Address, reg.Quantity)
	case 4: // 读输入寄存器
		result, err = a.client.ReadInputRegisters(reg.Address, reg.Quantity)
	default:
		return fmt.Errorf("不支持的Modbus功能码: %d", reg.Function)
	}

	if err != nil {
		return fmt.Errorf("读取寄存器失败: %w", err)
	}

	// 解析数据
	value, dataType, err := a.parseData(result, reg)
	if err != nil {
		return fmt.Errorf("解析数据失败: %w", err)
	}

	if value != nil {
		// 创建数据点
		point := model.NewPoint(reg.Key, fmt.Sprintf("modbus-%d", reg.DeviceID), value, dataType)

		// 添加标签
		point.AddTag("source", "modbus")
		point.AddTag("mode", a.mode)
		point.AddTag("address", fmt.Sprintf("%d", reg.Address))
		point.AddTag("function", fmt.Sprintf("%d", reg.Function))

		// 添加自定义标签
		for k, v := range reg.Tags {
			point.AddTag(k, v)
		}

		// 发送数据点
		a.SafeSendDataPoint(ch, point, pollStart)
	}

	return nil
}

// parseData 解析Modbus数据
func (a *ModbusAdapter) parseData(result []byte, reg Register) (interface{}, model.DataType, error) {
	if len(result) == 0 {
		return nil, "", fmt.Errorf("读取数据为空")
	}

	switch reg.Type {
	case "bool":
		return a.parseBool(result, reg)
	case "int16":
		return a.parseInt16(result, reg)
	case "int32":
		return a.parseInt32(result, reg)
	case "float32":
		return a.parseFloat32(result, reg)
	case "string":
		return a.parseString(result, reg)
	default:
		// 默认按int16处理
		return a.parseInt16(result, reg)
	}
}

// parseBool 解析布尔值
func (a *ModbusAdapter) parseBool(result []byte, reg Register) (interface{}, model.DataType, error) {
	if reg.Function == 1 || reg.Function == 2 {
		// 线圈和离散输入，每个位代表一个布尔值
		if len(result) > 0 {
			return (result[0] & 0x01) == 1, model.TypeBool, nil
		}
	} else {
		// 寄存器类型，根据位偏移读取
		if len(result) >= 2 {
			var value uint16
			if reg.ByteOrder == "little_endian" {
				value = binary.LittleEndian.Uint16(result[:2])
			} else {
				value = binary.BigEndian.Uint16(result[:2])
			}

			if reg.BitOffset >= 0 && reg.BitOffset < 16 {
				return ((value >> reg.BitOffset) & 0x01) == 1, model.TypeBool, nil
			} else {
				return value != 0, model.TypeBool, nil
			}
		}
	}

	return nil, "", fmt.Errorf("无法解析布尔值")
}

// parseInt16 解析16位整数
func (a *ModbusAdapter) parseInt16(result []byte, reg Register) (interface{}, model.DataType, error) {
	if len(result) < 2 {
		return nil, "", fmt.Errorf("数据长度不足，需要2字节")
	}

	var value int16
	if reg.ByteOrder == "little_endian" {
		value = int16(binary.LittleEndian.Uint16(result[:2]))
	} else {
		value = int16(binary.BigEndian.Uint16(result[:2]))
	}

	scaledValue := int(float64(value) * reg.Scale)
	return scaledValue, model.TypeInt, nil
}

// parseInt32 解析32位整数
func (a *ModbusAdapter) parseInt32(result []byte, reg Register) (interface{}, model.DataType, error) {
	if len(result) < 4 {
		return nil, "", fmt.Errorf("数据长度不足，需要4字节")
	}

	var value int32
	if reg.ByteOrder == "little_endian" {
		value = int32(binary.LittleEndian.Uint32(result[:4]))
	} else {
		value = int32(binary.BigEndian.Uint32(result[:4]))
	}

	scaledValue := int(float64(value) * reg.Scale)
	return scaledValue, model.TypeInt, nil
}

// parseFloat32 解析32位浮点数
func (a *ModbusAdapter) parseFloat32(result []byte, reg Register) (interface{}, model.DataType, error) {
	if len(result) < 4 {
		return nil, "", fmt.Errorf("数据长度不足，需要4字节")
	}

	var bits uint32
	if reg.ByteOrder == "little_endian" {
		bits = binary.LittleEndian.Uint32(result[:4])
	} else {
		bits = binary.BigEndian.Uint32(result[:4])
	}

	value := math.Float32frombits(bits)
	scaledValue := float64(value) * reg.Scale

	return scaledValue, model.TypeFloat, nil
}

// parseString 解析字符串
func (a *ModbusAdapter) parseString(result []byte, reg Register) (interface{}, model.DataType, error) {
	// 移除末尾的空字符
	for len(result) > 0 && result[len(result)-1] == 0 {
		result = result[:len(result)-1]
	}

	return string(result), model.TypeString, nil
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
