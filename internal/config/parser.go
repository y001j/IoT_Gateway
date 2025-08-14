package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// ConfigParser provides generic configuration parsing with validation
type ConfigParser[T any] struct {
	defaults T
}

// NewParser creates a new configuration parser
func NewParser[T any]() *ConfigParser[T] {
	return &ConfigParser[T]{}
}

// NewParserWithDefaults creates a new configuration parser with default values
func NewParserWithDefaults[T any](defaults T) *ConfigParser[T] {
	return &ConfigParser[T]{
		defaults: defaults,
	}
}

// Parse parses configuration from JSON raw message
func (p *ConfigParser[T]) Parse(raw json.RawMessage) (*T, error) {
	var config T

	// Apply defaults first
	if !reflect.ValueOf(p.defaults).IsZero() {
		config = p.defaults
	}

	// Unmarshal JSON
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Validate configuration
	if err := p.Validate(&config); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &config, nil
}

// ParseFromMap parses configuration from map
func (p *ConfigParser[T]) ParseFromMap(data map[string]interface{}) (*T, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal map to json: %w", err)
	}

	return p.Parse(jsonData)
}

// ParseFromManager parses configuration from ConfigManager
func (p *ConfigParser[T]) ParseFromManager(mgr ConfigManager, key string) (*T, error) {
	var config T
	if err := mgr.GetAs(key, &config); err != nil {
		return nil, fmt.Errorf("get config from manager: %w", err)
	}

	if err := p.Validate(&config); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &config, nil
}

// Validate validates the configuration using struct tags
func (p *ConfigParser[T]) Validate(config *T) error {
	return validateStruct(reflect.ValueOf(config).Elem(), "")
}

// validateStruct validates a struct using reflection and tags
func validateStruct(v reflect.Value, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		fieldName := fieldType.Name
		
		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		// Skip unexported fields
		if !field.CanInterface() {
			continue
		}

		// Get validation tag
		tag := fieldType.Tag.Get("validate")
		if tag == "" {
			// If no validation tag, continue to nested structs
			if field.Kind() == reflect.Struct {
				if err := validateStruct(field, fieldName); err != nil {
					return err
				}
			}
			continue
		}

		// Parse validation rules
		rules := strings.Split(tag, ",")
		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			if err := validateField(field, fieldName, rule); err != nil {
				return err
			}
		}

		// Validate nested structs
		if field.Kind() == reflect.Struct {
			if err := validateStruct(field, fieldName); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateField validates a single field based on validation rule
func validateField(field reflect.Value, fieldName, rule string) error {
	switch {
	case rule == "required":
		if isZeroValue(field) {
			return fmt.Errorf("field '%s' is required", fieldName)
		}

	case strings.HasPrefix(rule, "min="):
		minStr := strings.TrimPrefix(rule, "min=")
		min, err := strconv.Atoi(minStr)
		if err != nil {
			return fmt.Errorf("invalid min value for field '%s': %s", fieldName, minStr)
		}
		if err := validateMin(field, fieldName, min); err != nil {
			return err
		}

	case strings.HasPrefix(rule, "max="):
		maxStr := strings.TrimPrefix(rule, "max=")
		max, err := strconv.Atoi(maxStr)
		if err != nil {
			return fmt.Errorf("invalid max value for field '%s': %s", fieldName, maxStr)
		}
		if err := validateMax(field, fieldName, max); err != nil {
			return err
		}

	case strings.HasPrefix(rule, "range="):
		rangeStr := strings.TrimPrefix(rule, "range=")
		parts := strings.Split(rangeStr, "-")
		if len(parts) != 2 {
			return fmt.Errorf("invalid range format for field '%s': %s", fieldName, rangeStr)
		}
		min, err1 := strconv.Atoi(parts[0])
		max, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid range values for field '%s': %s", fieldName, rangeStr)
		}
		if err := validateRange(field, fieldName, min, max); err != nil {
			return err
		}

	case strings.HasPrefix(rule, "oneof="):
		optionsStr := strings.TrimPrefix(rule, "oneof=")
		options := strings.Split(optionsStr, " ")
		if err := validateOneOf(field, fieldName, options); err != nil {
			return err
		}

	case rule == "url":
		if err := validateURL(field, fieldName); err != nil {
			return err
		}

	case rule == "port":
		if err := validatePort(field, fieldName); err != nil {
			return err
		}
	}

	return nil
}

// isZeroValue checks if a value is zero
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return v.IsNil()
	default:
		return reflect.DeepEqual(v.Interface(), reflect.Zero(v.Type()).Interface())
	}
}

// validateMin validates minimum value
func validateMin(field reflect.Value, fieldName string, min int) error {
	switch field.Kind() {
	case reflect.String:
		if len(field.String()) < min {
			return fmt.Errorf("field '%s' must have at least %d characters", fieldName, min)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() < int64(min) {
			return fmt.Errorf("field '%s' must be at least %d", fieldName, min)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if field.Uint() < uint64(min) {
			return fmt.Errorf("field '%s' must be at least %d", fieldName, min)
		}
	case reflect.Float32, reflect.Float64:
		if field.Float() < float64(min) {
			return fmt.Errorf("field '%s' must be at least %f", fieldName, float64(min))
		}
	case reflect.Slice, reflect.Array:
		if field.Len() < min {
			return fmt.Errorf("field '%s' must have at least %d elements", fieldName, min)
		}
	}
	return nil
}

// validateMax validates maximum value
func validateMax(field reflect.Value, fieldName string, max int) error {
	switch field.Kind() {
	case reflect.String:
		if len(field.String()) > max {
			return fmt.Errorf("field '%s' must have at most %d characters", fieldName, max)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() > int64(max) {
			return fmt.Errorf("field '%s' must be at most %d", fieldName, max)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if field.Uint() > uint64(max) {
			return fmt.Errorf("field '%s' must be at most %d", fieldName, max)
		}
	case reflect.Float32, reflect.Float64:
		if field.Float() > float64(max) {
			return fmt.Errorf("field '%s' must be at most %f", fieldName, float64(max))
		}
	case reflect.Slice, reflect.Array:
		if field.Len() > max {
			return fmt.Errorf("field '%s' must have at most %d elements", fieldName, max)
		}
	}
	return nil
}

// validateRange validates value within range
func validateRange(field reflect.Value, fieldName string, min, max int) error {
	if err := validateMin(field, fieldName, min); err != nil {
		return err
	}
	return validateMax(field, fieldName, max)
}

// validateOneOf validates field value is one of specified options
func validateOneOf(field reflect.Value, fieldName string, options []string) error {
	value := fmt.Sprintf("%v", field.Interface())
	for _, option := range options {
		if value == option {
			return nil
		}
	}
	return fmt.Errorf("field '%s' must be one of: %s", fieldName, strings.Join(options, ", "))
}

// validateURL validates URL format
func validateURL(field reflect.Value, fieldName string) error {
	if field.Kind() != reflect.String {
		return fmt.Errorf("field '%s' must be a string for URL validation", fieldName)
	}
	url := field.String()
	if url == "" {
		return nil // Empty URL is allowed unless required is also specified
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "tcp://") {
		return fmt.Errorf("field '%s' must be a valid URL", fieldName)
	}
	return nil
}

// validatePort validates port number
func validatePort(field reflect.Value, fieldName string) error {
	var port int64
	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		port = field.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		port = int64(field.Uint())
	default:
		return fmt.Errorf("field '%s' must be a number for port validation", fieldName)
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("field '%s' must be a valid port number (1-65535)", fieldName)
	}
	return nil
}