package config

import "time"

// Plugin configuration types for sidecar and external plugins

// PluginMetadata represents plugin metadata configuration
type PluginMetadata struct {
	Name        string            `json:"name" yaml:"name" validate:"required"`
	Version     string            `json:"version" yaml:"version" validate:"required"`
	Type        string            `json:"type" yaml:"type" validate:"required,oneof=adapter sink"`
	Mode        string            `json:"mode" yaml:"mode" validate:"required,oneof=builtin external isp-sidecar"`
	Entry       string            `json:"entry,omitempty" yaml:"entry,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Author      string            `json:"author,omitempty" yaml:"author,omitempty"`
	License     string            `json:"license,omitempty" yaml:"license,omitempty"`
	Tags        map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	
	// ISP Sidecar specific
	ISPPort    int               `json:"isp_port,omitempty" yaml:"isp_port,omitempty" validate:"port"`
	ISPTimeout time.Duration     `json:"isp_timeout,omitempty" yaml:"isp_timeout,omitempty"`
	
	// External plugin specific
	Command     string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
}

// SidecarPluginConfig represents configuration for ISP sidecar plugins
type SidecarPluginConfig struct {
	BaseConfig   `json:",inline" yaml:",inline"`
	ISPPort      int           `json:"isp_port" yaml:"isp_port" validate:"required,port"`
	ISPTimeout   time.Duration `json:"isp_timeout,omitempty" yaml:"isp_timeout,omitempty"`
	Entry        string        `json:"entry" yaml:"entry" validate:"required"`
	AutoRestart  bool          `json:"auto_restart,omitempty" yaml:"auto_restart,omitempty"`
	MaxRetries   int           `json:"max_retries,omitempty" yaml:"max_retries,omitempty" validate:"min=0,max=10"`
	
	// Plugin-specific configuration (passed to sidecar)
	PluginConfig map[string]interface{} `json:"plugin_config,omitempty" yaml:"plugin_config,omitempty"`
}

// ExternalPluginConfig represents configuration for external plugins
type ExternalPluginConfig struct {
	BaseConfig  `json:",inline" yaml:",inline"`
	Command     string            `json:"command" yaml:"command" validate:"required"`
	Args        []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	AutoRestart bool              `json:"auto_restart,omitempty" yaml:"auto_restart,omitempty"`
	MaxRetries  int               `json:"max_retries,omitempty" yaml:"max_retries,omitempty" validate:"min=0,max=10"`
}

// GetDefaultPluginMetadata returns default plugin metadata
func GetDefaultPluginMetadata() PluginMetadata {
	return PluginMetadata{
		Version:    "1.0.0",
		Mode:       "builtin",
		ISPTimeout: 30 * time.Second,
		Tags:       make(map[string]string),
	}
}

// GetDefaultSidecarPluginConfig returns default sidecar plugin configuration
func GetDefaultSidecarPluginConfig() SidecarPluginConfig {
	return SidecarPluginConfig{
		BaseConfig: BaseConfig{
			Enabled: true,
			Tags:    make(map[string]string),
		},
		ISPTimeout:   30 * time.Second,
		AutoRestart:  true,
		MaxRetries:   3,
		PluginConfig: make(map[string]interface{}),
	}
}

// GetDefaultExternalPluginConfig returns default external plugin configuration
func GetDefaultExternalPluginConfig() ExternalPluginConfig {
	return ExternalPluginConfig{
		BaseConfig: BaseConfig{
			Enabled: true,
			Tags:    make(map[string]string),
		},
		Environment: make(map[string]string),
		Timeout:     60 * time.Second,
		AutoRestart: true,
		MaxRetries:  3,
	}
}