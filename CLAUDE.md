# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an IoT Gateway system built in Go that processes data from various IoT devices and routes it to multiple destinations. The system uses a plugin-based architecture with NATS as the message bus and includes a powerful rule engine for real-time data processing.

## Architecture

### Core Components

1. **Runtime (`internal/core/runtime.go`)**: Main orchestration layer that manages all services, handles NATS connections (embedded or external), and provides configuration management through Viper
2. **Plugin Manager (`internal/plugin/manager.go`)**: Manages both built-in and external plugins (adapters and sinks), handles plugin lifecycle, and routes data between adapters and sinks
3. **Rule Engine (`internal/rules/`)**: Event-driven data processing with support for complex conditions and multiple action types
4. **Southbound Adapters (`internal/southbound/`)**: Data ingestion from various protocols (Modbus, HTTP, MQTT, Mock)
5. **Northbound Sinks (`internal/northbound/`)**: Data output to various destinations (MQTT, InfluxDB, Redis, Console, WebSocket, JetStream)

### Key Patterns

- **Plugin Architecture**: Both adapters and sinks are registered in global registries and can be loaded as built-in components or external plugins
- **NATS Message Bus**: Central communication hub for all data flow (`iot.data.*`, `iot.rules.*` subjects)
- **Configuration-Driven**: YAML/JSON configuration files define plugin instances and their settings
- **Hot-Reload**: File watchers enable runtime configuration updates

## Common Development Commands

### Build and Run
```bash
# Build main gateway
go build -o bin/gateway cmd/gateway/main.go

# Run with specific config
./bin/gateway -config config.yaml

# Build web server
go build -o bin/server cmd/server/main.go
```

### Frontend Development
```bash
# Frontend is in web/frontend/
cd web/frontend
npm install
npm run dev        # Development server
npm run build      # Production build  
npm run preview    # Preview production build
npm run lint       # ESLint with warnings
npm run lint:fix   # ESLint with auto-fix
```

### Testing
```bash
# Run all tests
go test ./...

# Test specific package
go test ./internal/rules/...

# Run with verbose output
go test -v ./internal/core/...
```

### Tools and Utilities
```bash
# NATS message monitoring
nats sub "iot.data.>"
nats pub iot.data.test '{"device_id":"test","key":"temp","value":25.5}'

# Modbus testing
python3 tools/modbus_simulator.py  # Requires pymodbus

# End-to-end tests
go run cmd/test/main.go

# Rule engine testing suite
./run_rule_engine_tests.sh         # Comprehensive rule engine tests
./run_rule_engine_demo.sh          # Interactive rule engine demo
./test_gateway_rules.sh            # Gateway rule testing
./quick_test_rules.sh              # Quick rule validation

# Database initialization (SQLite)
# Database is auto-created on first run at configured path
# Default: data/gateway.db

# JetStream recovery tool
go run cmd/tools/jetstream_recovery/main.go -stream=IOT_DATA -sink=influxdb -config=sink.yaml

# Modbus connectivity test
go run cmd/tools/modbus_test/main.go -host=192.168.1.100 -port=502
```

## Configuration Structure

### Main Config (`config.yaml`)
- `gateway`: Core settings (ports, log level, NATS URL, plugins directory)
- `southbound.adapters`: List of data source adapters
- `northbound.sinks`: List of data destination sinks
- `rule_engine`: Rule processing configuration
- `web_ui`: Web interface settings

### Plugin Configuration
- Built-in plugins: Configured in main config under `southbound.adapters` or `northbound.sinks`
- External plugins: JSON files in `plugins/` directory with metadata and configuration
- Sidecar plugins: Separate processes communicating via ISP protocol

## Rule Engine

The rule engine processes data through configurable rules with enhanced conditions and actions:

### Rule Structure
```json
{
  "id": "rule_id",
  "name": "Rule Name",
  "enabled": true,
  "conditions": {
    "type": "simple|and|or|expression",
    "field": "field_name",
    "operator": "eq|ne|gt|gte|lt|lte|contains|startswith|endswith|regex",
    "value": "comparison_value",
    "expression": "value > 30 && contains(device_id, \"sensor\")"
  },
  "actions": [
    {
      "type": "alert|transform|filter|aggregate|forward",
      "config": { /* action-specific configuration */ }
    }
  ]
}
```

### Enhanced Condition System
- **Simple Conditions**: Field comparison with rich operators (eq, ne, gt, gte, lt, lte, contains, startswith, endswith, regex)
- **Compound Conditions**: AND, OR, NOT logic with nesting support
- **Expression Engine**: Mathematical expressions with recursive descent parser
- **Built-in Functions**: Math (abs, max, min, sqrt, pow), string (len, upper, lower), time (now, timeFormat)
- **Regex Caching**: High-performance regex matching with global cache

### Enhanced Action Types
- **Alert**: Multi-channel notifications (console, webhook, email, SMS, NATS publishing) with throttling
- **Transform**: Data manipulation (scale, offset, unit conversion, expression calculation, lookup tables) with NATS publishing
- **Filter**: Data filtering (deduplication, range, rate limiting, pattern matching)
- **Aggregate**: Statistical operations (avg, sum, count) over time windows with circular buffers
- **Forward**: Simplified NATS-focused data routing with dynamic subject configuration

### Key Features
- **Rule Execution Events**: Automatic publishing to `iot.rules.*` subjects for monitoring
- **Performance Optimizations**: Regex caching, string operation optimization, concurrent processing
- **Error Handling**: Layered error management with retry mechanisms and error classification
- **Monitoring**: Comprehensive metrics collection and health status tracking

## Data Flow

1. **Southbound Adapters** collect data from devices → **Data Channel**
2. **Plugin Manager** routes data to **Northbound Sinks** and **NATS Bus**
3. **Rule Engine** subscribes to NATS subjects (`iot.data.*`)
4. **Rules** process data and trigger **Actions**
5. **Actions** can modify data, send alerts, or forward to other systems

## Development Practices

### Code Quality
```bash
# Format Go code
go fmt ./...

# Run Go vet for static analysis
go vet ./...

# Build with race detector (development only)
go build -race -o bin/gateway cmd/gateway/main.go

# Generate test coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
rm -rf bin/ logs/ coverage.*
```

### Dependencies
```bash
# Download dependencies
go mod download

# Verify dependencies
go mod verify

# Tidy dependencies (remove unused)
go mod tidy

# Update dependencies
go get -u ./...

# Vendor dependencies (optional)
go mod vendor
```

### Development Workflow
```bash
# Quick build and run
go run cmd/gateway/main.go -config config.yaml

# Build all binaries
go build -o bin/gateway cmd/gateway/main.go
go build -o bin/server cmd/server/main.go
go build -o bin/jetstream-recovery cmd/tools/jetstream_recovery/main.go
go build -o bin/modbus-test cmd/tools/modbus_test/main.go

# Run with debug logging
./bin/gateway -config config.yaml -log-level debug

# Run with custom plugins directory
./bin/gateway -config config.yaml -plugins-dir /path/to/plugins

# Monitor application logs
tail -f logs/gateway.log
tail -f logs/rule_engine_test_data.log
```

### Performance Analysis
```bash
# CPU profiling (requires adding pprof endpoints)
go tool pprof http://localhost:6060/debug/pprof/profile

# Memory profiling
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine analysis
go tool pprof http://localhost:6060/debug/pprof/goroutine

# Run benchmarks (when available)
go test -bench=. -benchmem ./...
```

## Development Guidelines

### Adding New Adapters
1. Implement `southbound.Adapter` interface
2. Register in `southbound.Registry` via `init()`
3. Add configuration support in main config
4. Follow existing patterns in `internal/southbound/`

### Adding New Sinks
1. Implement `northbound.Sink` interface
2. Register in `northbound.Registry` via `init()`
3. Add configuration support in main config
4. Follow existing patterns in `internal/northbound/`

### Adding New Rule Actions
1. Implement `ActionHandler` interface in `internal/rules/actions/`
2. Register in action registry
3. Add configuration validation
4. Include comprehensive error handling

### Testing Patterns
- Use `internal/southbound/mock` for adapter testing
- Test with embedded NATS server
- Create configuration files in `configs/` for integration tests
- Use `cmd/test/` for end-to-end scenarios

## Important File Locations

- **Main Applications**: `cmd/gateway/main.go`, `cmd/server/main.go`
- **Configuration**: `config.yaml`, `configs/examples/`
- **Plugin Definitions**: `plugins/*.json`
- **Rule Definitions**: `rules/*.json`
- **Documentation**: `docs/`, `README_RULE_ENGINE.md`
- **Frontend**: `web/frontend/`
- **Test Data**: `examples/`, `configs/examples/`

## Debugging Tips

### Common Issues
- Check NATS server connectivity if data flow stops
- Verify plugin configurations match expected schema
- Check file permissions for rule and plugin directories
- Monitor NATS subjects for message flow

### Useful Commands
```bash
# Check NATS connectivity
nats server check

# Monitor data flow
nats sub "iot.data.>"

# Check plugin status via web UI
curl http://localhost:8081/api/plugins

# View rule engine status
curl http://localhost:8081/api/rules
```

## Module Dependencies

- **Message Bus**: NATS with JetStream for persistent messaging
- **Configuration**: Viper for YAML/JSON config management
- **Logging**: Zerolog for structured logging
- **Web Framework**: Gin for REST API and web interface
- **Database**: SQLite for authentication and metadata
- **Protocols**: Modbus, MQTT, HTTP clients for device communication