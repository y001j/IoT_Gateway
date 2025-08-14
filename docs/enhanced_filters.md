# Enhanced Filter Functionality

## Overview

The IoT Gateway now includes four new advanced filter types to improve data quality and reduce false alerts in industrial IoT environments. These filters address common real-world scenarios encountered in production deployments.

## New Filter Types

### 1. Quality Filter (`quality`)

**Purpose**: Filters data based on device quality codes, a standard feature in industrial protocols like Modbus.

**Use Cases**:
- Remove data from sensors with communication errors
- Filter readings during device calibration
- Exclude data from devices in maintenance mode

**Configuration**:
```json
{
  "type": "filter",
  "config": {
    "type": "quality",
    "parameters": {
      "allowed_quality": [0, 64, 128]
    },
    "action": "drop"
  }
}
```

**Parameters**:
- `allowed_quality`: Array of acceptable quality codes (default: [0])
  - 0: Normal/Good quality
  - 64: Uncertain quality
  - 128: Bad quality
  - 192: Configuration error

**Example Scenarios**:
- PLC sensor with quality=1 (sensor fault) → Data filtered out
- Temperature sensor with quality=0 (normal) → Data passes through
- Pressure sensor with quality=3 (communication error) → Data filtered out

### 2. Change Rate Filter (`change_rate`)

**Purpose**: Filters data that changes too rapidly, preventing false alerts from sensor malfunctions or signal interference.

**Use Cases**:
- Detect sensor connection issues causing value jumps
- Filter electrical interference in analog signals
- Prevent false alerts from device restart artifacts

**Configuration**:
```json
{
  "type": "filter", 
  "config": {
    "type": "change_rate",
    "parameters": {
      "max_change_rate": 5.0,
      "time_window": "10s"
    },
    "action": "drop"
  }
}
```

**Parameters**:
- `max_change_rate`: Maximum allowed change per second
- `time_window`: Time window for rate calculation (default: "10s")

**Example Scenarios**:
- Temperature changes from 25°C to 85°C in 1 second → Filtered (too fast)
- Pressure increases by 2.5 bar/minute → Normal, passes through
- Vibration spikes due to electrical noise → Filtered out

### 3. Statistical Anomaly Filter (`statistical_anomaly`)

**Purpose**: Detects anomalies using adaptive statistical thresholds based on historical data patterns.

**Use Cases**:
- Identify gradual sensor drift or calibration issues
- Detect unusual environmental conditions
- Adaptive threshold that adjusts to seasonal changes

**Configuration**:
```json
{
  "type": "filter",
  "config": {
    "type": "statistical_anomaly", 
    "parameters": {
      "window_size": 20,
      "std_threshold": 2.5,
      "min_samples": 8
    },
    "action": "drop"
  }
}
```

**Parameters**:
- `window_size`: Number of historical samples to maintain (default: 20)
- `std_threshold`: Number of standard deviations for anomaly detection (default: 2.0)  
- `min_samples`: Minimum samples before anomaly detection starts (default: 5)

**Example Scenarios**:
- Outdoor temperature sensor: Winter average 5°C, reading 35°C → Anomaly detected
- Indoor humidity sensor: Normal range 45-55%, reading 85% → Anomaly detected
- Flow sensor: Gradual drift over weeks → Eventually detected as anomaly

### 4. Consecutive Anomaly Filter (`consecutive`)

**Purpose**: Only triggers filtering after consecutive anomalous readings, reducing false positives from temporary issues.

**Use Cases**:
- Prevent single-point failures from triggering alerts
- Require consistent anomalies before taking action  
- Reduce noise from intermittent communication issues

**Configuration**:
```json
{
  "type": "filter",
  "config": {
    "type": "consecutive",
    "parameters": {
      "consecutive_count": 3,
      "inner_filter": {
        "type": "threshold",
        "parameters": {
          "threshold": 50.0,
          "operator": "gt"
        }
      }
    },
    "action": "drop"
  }
}
```

**Parameters**:
- `consecutive_count`: Required number of consecutive anomalies (default: 3)
- `inner_filter`: The filter condition to check for anomalies

**Example Scenarios**:
- One network timeout causing bad reading → Ignored
- Three consecutive high temperature readings → Filtered/Alerted
- Intermittent sensor errors → Only persistent errors trigger action

## Implementation Details

### Memory Management
- All filters use TTL-based cache cleanup (5-minute intervals)
- Different cache prefixes prevent conflicts between filter types
- Thread-safe implementation with read-write mutex protection

### Performance Characteristics
- Quality Filter: O(1) lookup performance
- Change Rate Filter: O(1) with small memory footprint per device
- Statistical Anomaly: O(n) where n is window size, but bounded
- Consecutive Filter: O(1) state tracking per device

### Cache Key Patterns
- Quality: No caching needed (stateless)
- Change Rate: `chg:{device_id}:{key}`
- Statistical: `stat:{device_id}:{key}`  
- Consecutive: `cons:{device_id}:{key}`

## Best Practices

### 1. Filter Order
When using multiple filters, apply them in this recommended order:
1. Quality filter (remove bad quality data first)
2. Range/null filters (basic validation)
3. Change rate filter (remove obvious anomalies) 
4. Statistical anomaly (advanced detection)
5. Consecutive filter (final validation)

### 2. Parameter Tuning

**Quality Filter**:
- Start with [0] for critical systems
- Add [64] for systems that can tolerate uncertain data
- Never include error codes like [128, 192] unless required

**Change Rate Filter**:
- Set based on physical system capabilities
- Temperature sensors: 1-5°C/s maximum
- Pressure sensors: 0.1-1.0 bar/s maximum
- Flow sensors: Based on valve/pump response times

**Statistical Anomaly**:
- `window_size`: 10-50 samples depending on data frequency
- `std_threshold`: 2.0-3.0 for most applications
- `min_samples`: 50% of window_size

**Consecutive Filter**:
- 2-3 for high-frequency data
- 3-5 for low-frequency data  
- Higher values for very noisy environments

### 3. Monitoring and Debugging

Enable debug logging to track filter decisions:
```json
{
  "log_level": "debug"
}
```

Monitor filter effectiveness through rule execution metrics in the web UI.

## Migration from Existing Filters

Existing filter configurations remain fully compatible. New enhanced filters can be added alongside existing ones or used as replacements:

**Before** (Basic threshold):
```json
{
  "type": "threshold",
  "parameters": {
    "threshold": 50.0,
    "operator": "gt"
  }
}
```

**After** (Consecutive threshold with reduced false positives):
```json
{
  "type": "consecutive", 
  "parameters": {
    "consecutive_count": 3,
    "inner_filter": {
      "type": "threshold",
      "parameters": {
        "threshold": 50.0,
        "operator": "gt"
      }
    }
  }
}
```

## Testing and Validation

Test the new filters with your specific data patterns:

1. **Quality Filter**: Test with devices having different quality codes
2. **Change Rate**: Inject rapid value changes to verify filtering  
3. **Statistical**: Run with historical data to tune parameters
4. **Consecutive**: Test with intermittent anomalies

Use the provided examples in `examples/rules/enhanced_filter_examples.json` as starting points for your specific use cases.