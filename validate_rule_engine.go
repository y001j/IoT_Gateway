package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// 配置结构体（简化版）
type Config struct {
	Gateway struct {
		Name       string `yaml:"name"`
		LogLevel   string `yaml:"log_level"`
		HTTPPort   int    `yaml:"http_port"`
		NatsURL    string `yaml:"nats_url"`
	} `yaml:"gateway"`
	
	RuleEngine struct {
		Enabled  bool   `yaml:"enabled"`
		RulesDir string `yaml:"rules_dir"`
		Subject  string `yaml:"subject"`
		Rules    []Rule `yaml:"rules"`
	} `yaml:"rule_engine"`
	
	Southbound struct {
		Adapters []Adapter `yaml:"adapters"`
	} `yaml:"southbound"`
	
	Northbound struct {
		Sinks []Sink `yaml:"sinks"`
	} `yaml:"northbound"`
	
	WebUI struct {
		Enabled       bool   `yaml:"enabled"`
		ListenAddress string `yaml:"listen_address"`
	} `yaml:"web_ui"`
}

type Rule struct {
	ID          string                 `yaml:"id" json:"id"`
	Name        string                 `yaml:"name" json:"name"`
	Description string                 `yaml:"description" json:"description"`
	Enabled     bool                   `yaml:"enabled" json:"enabled"`
	Priority    int                    `yaml:"priority" json:"priority"`
	Conditions  map[string]interface{} `yaml:"conditions" json:"conditions"`
	Actions     []map[string]interface{} `yaml:"actions" json:"actions"`
}

type Adapter struct {
	Name    string                 `yaml:"name"`
	Type    string                 `yaml:"type"`
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:"config"`
}

type Sink struct {
	Name    string                 `yaml:"name"`
	Type    string                 `yaml:"type"`
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:"config"`
}

func main() {
	fmt.Println("🔍 IoT Gateway 规则引擎配置验证工具")
	fmt.Println("=====================================")
	fmt.Println()

	// 验证配置文件
	configFile := "config_rule_engine_test.yaml"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	
	fmt.Printf("📁 验证配置文件: %s\n", configFile)
	
	config, err := loadConfig(configFile)
	if err != nil {
		log.Fatalf("❌ 配置文件加载失败: %v", err)
	}
	
	fmt.Println("✅ 配置文件加载成功")
	
	// 验证配置内容
	validateConfig(config)
	
	// 验证规则文件
	if config.RuleEngine.RulesDir != "" {
		validateRulesDirectory(config.RuleEngine.RulesDir)
	}
	
	// 验证内联规则
	if len(config.RuleEngine.Rules) > 0 {
		validateInlineRules(config.RuleEngine.Rules)
	}
	
	fmt.Println()
	fmt.Println("🎉 验证完成！")
}

func loadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}
	
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("YAML解析失败: %v", err)
	}
	
	return &config, nil
}

func validateConfig(config *Config) {
	fmt.Println()
	fmt.Println("⚙️  配置内容验证:")
	
	// 验证网关配置
	fmt.Printf("  🌐 网关名称: %s\n", config.Gateway.Name)
	fmt.Printf("  📊 日志级别: %s\n", config.Gateway.LogLevel)
	fmt.Printf("  🔌 HTTP端口: %d\n", config.Gateway.HTTPPort)
	fmt.Printf("  📡 NATS地址: %s\n", config.Gateway.NatsURL)
	
	// 验证规则引擎配置
	fmt.Printf("  🔧 规则引擎: %s\n", enabledStatus(config.RuleEngine.Enabled))
	if config.RuleEngine.Enabled {
		fmt.Printf("     📂 规则目录: %s\n", config.RuleEngine.RulesDir)
		fmt.Printf("     📮 监听主题: %s\n", config.RuleEngine.Subject)
		fmt.Printf("     📋 内联规则数: %d\n", len(config.RuleEngine.Rules))
	}
	
	// 验证适配器配置
	fmt.Printf("  📥 南向适配器数量: %d\n", len(config.Southbound.Adapters))
	enabledAdapters := 0
	for _, adapter := range config.Southbound.Adapters {
		if adapter.Enabled {
			enabledAdapters++
		}
		fmt.Printf("     • %s (%s): %s\n", adapter.Name, adapter.Type, enabledStatus(adapter.Enabled))
	}
	fmt.Printf("     启用的适配器: %d/%d\n", enabledAdapters, len(config.Southbound.Adapters))
	
	// 验证输出配置
	fmt.Printf("  📤 北向输出数量: %d\n", len(config.Northbound.Sinks))
	enabledSinks := 0
	for _, sink := range config.Northbound.Sinks {
		if sink.Enabled {
			enabledSinks++
		}
		fmt.Printf("     • %s (%s): %s\n", sink.Name, sink.Type, enabledStatus(sink.Enabled))
	}
	fmt.Printf("     启用的输出: %d/%d\n", enabledSinks, len(config.Northbound.Sinks))
	
	// 验证Web UI配置
	fmt.Printf("  🌐 Web UI: %s\n", enabledStatus(config.WebUI.Enabled))
	if config.WebUI.Enabled {
		fmt.Printf("     📍 监听地址: %s\n", config.WebUI.ListenAddress)
	}
}

func validateRulesDirectory(rulesDir string) {
	fmt.Println()
	fmt.Printf("📂 验证规则目录: %s\n", rulesDir)
	
	// 检查目录是否存在
	if _, err := os.Stat(rulesDir); os.IsNotExist(err) {
		fmt.Printf("⚠️  规则目录不存在: %s\n", rulesDir)
		return
	}
	
	// 查找规则文件
	pattern := filepath.Join(rulesDir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		fmt.Printf("❌ 扫描规则文件失败: %v\n", err)
		return
	}
	
	fmt.Printf("📄 找到规则文件: %d个\n", len(files))
	
	totalRules := 0
	enabledRules := 0
	
	for _, file := range files {
		fmt.Printf("  🔍 验证文件: %s\n", filepath.Base(file))
		
		rules, err := loadRulesFromFile(file)
		if err != nil {
			fmt.Printf("     ❌ 加载失败: %v\n", err)
			continue
		}
		
		fmt.Printf("     ✅ 包含规则: %d个\n", len(rules))
		
		for _, rule := range rules {
			totalRules++
			if rule.Enabled {
				enabledRules++
			}
			fmt.Printf("        • %s: %s (%s)\n", rule.ID, rule.Name, enabledStatus(rule.Enabled))
		}
	}
	
	fmt.Printf("📊 规则统计: 总数 %d, 启用 %d\n", totalRules, enabledRules)
}

func validateInlineRules(rules []Rule) {
	fmt.Println()
	fmt.Printf("📋 验证内联规则: %d个\n", len(rules))
	
	enabledRules := 0
	for _, rule := range rules {
		if rule.Enabled {
			enabledRules++
		}
		
		fmt.Printf("  • %s: %s (%s)\n", rule.ID, rule.Name, enabledStatus(rule.Enabled))
		
		// 验证规则结构
		if rule.Conditions == nil {
			fmt.Printf("    ⚠️  缺少条件配置\n")
		}
		
		if len(rule.Actions) == 0 {
			fmt.Printf("    ⚠️  缺少动作配置\n")
		}
		
		// 验证动作类型
		for i, action := range rule.Actions {
			if actionType, ok := action["type"].(string); ok {
				fmt.Printf("    📤 动作%d: %s\n", i+1, actionType)
			} else {
				fmt.Printf("    ❌ 动作%d: 类型未定义\n", i+1)
			}
		}
	}
	
	fmt.Printf("📊 内联规则统计: 总数 %d, 启用 %d\n", len(rules), enabledRules)
}

func loadRulesFromFile(filename string) ([]Rule, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	
	var rules []Rule
	err = json.Unmarshal(data, &rules)
	if err != nil {
		return nil, err
	}
	
	return rules, nil
}

func enabledStatus(enabled bool) string {
	if enabled {
		return "✅ 启用"
	}
	return "❌ 禁用"
}