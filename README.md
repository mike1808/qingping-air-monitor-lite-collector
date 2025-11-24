# Qingping CGDN1 Air Monitor Lite - MQTT Collector

Go application to collect sensor data from Qingping CGDN1 Air Monitor Lite via MQTT using a request/response pattern.

## How It Works

The Qingping CGDN1 uses MQTT with a command/response protocol:

1. **Your app sends a Type 12 message** to `qingping/{MAC}/down` telling the device:
   - How often to report data (`up_itvl` in seconds)
   - How long to keep reporting (`duration` in seconds)
2. **Device automatically reports sensor data** to `qingping/{MAC}/up` at the specified interval
3. After `duration` expires, send another Type 12 message to keep it reporting

This is more efficient than requesting data every time - you configure once and the device reports automatically!

## Features

- Periodically requests sensor data from the device
- Parses JSON responses (temperature, humidity, CO2, PM2.5, PM10, TVOC, battery)
- Configurable polling intervals
- **Prometheus metrics exporter** for Grafana integration
- Logs all sensor readings
- Docker containerized

## Prerequisites

- Docker and Docker Compose
- MQTT broker (Mosquitto) running
- Qingping CGDN1 device configured for self-hosted MQTT via https://developer.qingping.co/

## Setup

### 1. Configure Your Device for MQTT

**Important:** You must configure your CGDN1 to use your local MQTT broker first:

1. Download the **Qingping IoT** app
2. Pair your device with the app
3. Go to https://developer.qingping.co/ and register
4. Create a new configuration with your MQTT broker details:
   - **Client ID**: Use MAC address format (e.g., `qingping/582D34123456`)
   - **Up Topic**: `qingping/{MAC}/up`
   - **Down Topic**: `qingping/{MAC}/down`
5. Link your device to this configuration

Detailed instructions: https://github.com/rand0mdud3/qingping_air_monitor_lite_CGDN1/blob/main/enableMQTT.md

### 2. Find Your Device MAC Address

Once configured, monitor MQTT traffic to see your device's MAC:

```bash
docker exec -it mosquitto mosquitto_sub -h localhost -u mike -P your_password -t 'qingping/#' -v
```

### 3. Configure Environment Variables

Edit `docker-compose.yml` and update:

```yaml
- MQTT_PASSWORD=your_actual_password
- DEVICE_MAC=YOUR_DEVICE_MAC      # e.g., 582D34123456 (no colons!)
- DEVICE_NAME=living_room
- UPDATE_INTERVAL=60              # Device reports every 60 seconds
- DURATION=21600                  # Keep reporting for 6 hours
```

**Configuration Options:**
- `UPDATE_INTERVAL`: How often the device reports data (seconds). Min: 15, recommended: 60
- `DURATION`: How long the device continues reporting before needing a new command (seconds). Default: 21600 (6 hours)

The app automatically sends a new Type 12 command just before the duration expires to maintain continuous reporting.

### 4. Build and Run

```bash
# Navigate to the project directory
cd /mnt/user/appdata/qingping-collector

# Build and start
docker-compose up -d

# Check logs
docker logs -f qingping-collector
```

## Expected Output

When working correctly, you'll see:

```
2024/11/23 10:30:45 Connected to MQTT broker
2024/11/23 10:30:45 Subscribed to: qingping/CCB5D132775A/up
2024/11/23 10:30:45 Qingping CGDN1 collector started
2024/11/23 10:30:45 Requesting data every 60 seconds for duration of 21600 seconds (6 hours)
2024/11/23 10:30:45 Starting Prometheus metrics server on :9273
2024/11/23 10:30:45 Sent Type 12 config to qingping/CCB5D132775A/down (interval: 60s, duration: 21600s)
2024/11/23 10:31:00 [air-sensor] Temp: 22.5°C, Humidity: 45.2%, CO2: 650 ppm, PM2.5: 12.3 μg/m³, PM10: 15.7 μg/m³, TVOC: 120 ppb, Battery: 85%
2024/11/23 10:32:00 [air-sensor] Temp: 22.6°C, Humidity: 45.1%, CO2: 648 ppm, PM2.5: 12.2 μg/m³, PM10: 15.6 μg/m³, TVOC: 118 ppb, Battery: 85%
...
2024/11/23 16:29:45 Refreshing device configuration...
2024/11/23 16:29:45 Sent Type 12 config to qingping/CCB5D132775A/down (interval: 60s, duration: 21600s)
```

## Troubleshooting

### No messages received

1. Verify device is configured via developer.qingping.co
2. Check device shows "Connected" in Settings > Private Cloud
3. Verify MAC address format (no colons): `582D34123456` not `58:2D:34:12:34:56`
4. Check if device can reach MQTT broker from guest network
5. Subscribe to all topics to debug: `docker exec -it mosquitto mosquitto_sub -h localhost -u mike -P password -t '#' -v`

### "Payload might be in TLV binary format"

Your device may be using the newer TLV (binary) protocol instead of JSON. This requires additional parsing code (not yet implemented).

### Connection refused

- Verify MQTT credentials in docker-compose.yml
- Check Mosquitto is running: `docker ps | grep mosquitto`
- Verify authentication is configured: `docker exec mosquitto cat /mosquitto/config/passwd`

## Protocol Details

**Type 12 Message Sent to `/down`:**
```json
{
  "type": "12",
  "up_itvl": "60",      // Report interval in seconds
  "duration": "21600"   // Keep reporting for this many seconds (6 hours)
}
```

After sending this message, the device will automatically publish sensor data every 60 seconds for the next 6 hours.

**Type 17 Message (for changing settings):**
```json
{
  "type": "17",
  "setting": {
    "temperature_offset": 0,
    "humidity_offset": 0
  }
}
```
This is used to change device settings like offsets, display settings, etc. (not implemented in this collector yet)

**Data Response from `/up`:**
```json
{
  "type": "17",
  "sensorData": [{
    "temperature": {"value": 22.5},
    "humidity": {"value": 45.2},
    "co2": {"value": 650},
    "pm25": {"value": 12.3},
    "pm10": {"value": 15.7},
    "tvoc": {"value": 120},
    "battery": {"value": 85}
  }]
}
```

## Next Steps

### Prometheus + Grafana Integration

The collector exposes Prometheus metrics on port 9273 (configurable via `METRICS_PORT`).

**Available Metrics:**
```
qingping_temperature_celsius{device="air-sensor"}
qingping_humidity_percent{device="air-sensor"}
qingping_co2_ppm{device="air-sensor"}
qingping_pm25_ugm3{device="air-sensor"}
qingping_pm10_ugm3{device="air-sensor"}
qingping_tvoc_ppb{device="air-sensor"}
qingping_battery_percent{device="air-sensor"}
qingping_last_update_timestamp{device="air-sensor"}
```

**Prometheus Configuration:**
```yaml
scrape_configs:
  - job_name: 'qingping'
    static_configs:
      - targets: ['qingping-collector:9273']
```

**Test Metrics:**
```bash
curl http://localhost:9273/metrics
```

### Grafana Dashboard

Import or create a dashboard using the metrics above. Example queries:
- Temperature: `qingping_temperature_celsius{device="air-sensor"}`
- CO2 Level: `qingping_co2_ppm{device="air-sensor"}`
- Air Quality: `qingping_pm25_ugm3{device="air-sensor"}`

## References

- Qingping Developer Portal: https://developer.qingping.co/
- Home Assistant Integration: https://github.com/mash2k3/qingping_cgs1
- Protocol Details: https://robertying.com/post/qingping-cgs1-home-assistant/
