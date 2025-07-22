package main

import (
	"fmt"
	"log"
	"github.com/spf13/viper"
	"github.com/y001j/iot-gateway/internal/web/models"
)

type WebConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Auth    models.AuthConfig `mapstructure:"auth"`
}

func main() {
	v := viper.New()
	v.SetConfigFile("config_rule_engine_test.yaml")
	v.SetConfigType("yaml")
	
	if err := v.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config: %v", err)
	}
	
	fmt.Printf("Raw web_ui section: %+v\n", v.Get("web_ui"))
	fmt.Printf("web_ui.enabled: %v\n", v.GetBool("web_ui.enabled"))
	fmt.Printf("web_ui.port: %v\n", v.GetInt("web_ui.port"))
	
	var config WebConfig
	if err := v.UnmarshalKey("web_ui", &config); err != nil {
		log.Fatalf("Failed to parse web config: %v", err)
	}
	
	fmt.Printf("Parsed config: %+v\n", config)
}