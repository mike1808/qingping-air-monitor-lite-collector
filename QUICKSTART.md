# Qingping CGDN1 Collector - Quick Start

## What This Does

Collects sensor data from your Qingping CGDN1 Air Monitor Lite and exports it as Prometheus metrics for Grafana.

## Features

✅ MQTT connection to your device  
✅ Prometheus metrics on port 9273  
✅ Clean logging (no verbose output)  
✅ Optional MQTT authentication  
✅ Automatic reconnection  
✅ Docker containerized  

## Quick Setup (5 minutes)

### 1. Prerequisites
- Qingping CGDN1 configured for MQTT (via developer.qingping.co)
- Mosquitto MQTT broker running
- Docker and Docker Compose

### 2. Deploy

```bash
cd /mnt/user/appdata
# Download/copy the qingping-collector folder here

cd qingping-collector

# Edit docker-compose.yml:
nano docker-compose.yml
# Update: DEVICE_MAC, DEVICE_NAME
# Optional: MQTT_USERNAME, MQTT_PASSWORD

# Start
docker-compose up -d

# Check logs
docker logs -f qingping-collector
```

### 3. Verify

```bash
# Should see sensor readings
docker logs qingping-collector

# Check metrics
curl http://localhost:9273/metrics | grep qingping
```

## Configuration

**Required:**
- `DEVICE_MAC` - Your device MAC (e.g., `CCB5D132775A`)

**Optional:**
- `DEVICE_NAME` - Label for metrics (default: `living_room`)
- `MQTT_USERNAME` - MQTT username (if using auth)
- `MQTT_PASSWORD` - MQTT password (if using auth)
- `UPDATE_INTERVAL` - Seconds between readings (default: `60`)
- `DURATION` - How long device reports (default: `21600` = 6 hours)
- `METRICS_PORT` - Prometheus port (default: `9273`)

## Grafana Integration

### Option 1: Use Provided Dashboard

1. In Grafana: Dashboards → Import
2. Upload `grafana-dashboard.json`
3. Select your Prometheus datasource
4. Done!

### Option 2: Manual Setup

Add to Prometheus (`prometheus.yml`):
```yaml
scrape_configs:
  - job_name: 'qingping'
    static_configs:
      - targets: ['your-unraid-ip:9273']
```

Create dashboard with queries:
```promql
qingping_temperature_celsius
qingping_humidity_percent
qingping_co2_ppm
qingping_pm25_ugm3
qingping_pm10_ugm3
qingping_battery_percent
```

## Metrics Reference

| Metric | Unit | Description |
|--------|------|-------------|
| `qingping_temperature_celsius` | °C | Temperature |
| `qingping_humidity_percent` | % | Relative humidity |
| `qingping_co2_ppm` | ppm | CO2 concentration |
| `qingping_pm25_ugm3` | μg/m³ | PM2.5 particulate |
| `qingping_pm10_ugm3` | μg/m³ | PM10 particulate |
| `qingping_tvoc_ppb` | ppb | Total VOC |
| `qingping_battery_percent` | % | Battery level |
| `qingping_last_update_timestamp` | unix | Last data update |

All metrics include `device` label for multi-device support.

## Troubleshooting

**No data?**
```bash
# Check device is sending to MQTT
docker exec -it mosquitto mosquitto_sub -t 'qingping/#' -v

# Check collector logs
docker logs qingping-collector

# Verify device MAC (no colons!)
# ✅ CCB5D132775A
# ❌ CC:B5:D1:32:77:5A
```

**Authentication errors?**
```bash
# If MQTT doesn't require auth, leave empty:
- MQTT_USERNAME=
- MQTT_PASSWORD=
```

**Metrics not showing?**
```bash
# Test endpoint
curl http://localhost:9273/metrics

# Check port is exposed
docker ps | grep qingping-collector
```

## Project Structure

```
qingping-collector/
├── main.go                   # Go application
├── go.mod                    # Dependencies
├── go.sum                    # Checksums
├── Dockerfile                # Container build
├── docker-compose.yml        # Deployment config
├── grafana-dashboard.json    # Pre-built dashboard
├── README.md                 # Full documentation
├── PROTOCOL.md               # MQTT protocol details
├── QUICK_REFERENCE.md        # Protocol reference
└── FINAL_UPDATE.md           # Latest changes
```

## Updates & Rebuilding

```bash
cd /mnt/user/appdata/qingping-collector

# Stop
docker-compose down

# Update files (main.go, docker-compose.yml, etc)

# Rebuild
docker-compose build

# Start
docker-compose up -d
```

## Support

For issues, check:
1. `README.md` - Full documentation
2. `PROTOCOL.md` - MQTT protocol details
3. `TYPE_DISCOVERY.md` - Message type analysis
4. `TROUBLESHOOTING.md` - Common issues

## Example Output

**Console Logs:**
```
2024/11/23 16:14:35 Connected to MQTT broker
2024/11/23 16:14:35 Subscribed to: qingping/CCB5D132775A/up
2024/11/23 16:14:35 Starting Prometheus metrics server on :9273
2024/11/23 16:14:35 Sent Type 12 config (interval: 60s, duration: 21600s)
2024/11/23 16:14:49 [air-sensor] Temp: 23.1°C, Humidity: 67.3%, CO2: 1024 ppm, PM2.5: 8.0 μg/m³, PM10: 8.0 μg/m³, TVOC: 0 ppb, Battery: 100%
```

**Prometheus Metrics:**
```
qingping_temperature_celsius{device="air-sensor"} 23.1
qingping_humidity_percent{device="air-sensor"} 67.3
qingping_co2_ppm{device="air-sensor"} 1024
qingping_pm25_ugm3{device="air-sensor"} 8
qingping_pm10_ugm3{device="air-sensor"} 8
qingping_battery_percent{device="air-sensor"} 100
```

## Architecture

```
CGDN1 Device → MQTT Broker → Qingping Collector → Prometheus → Grafana
                                     ↓
                              (HTTP :9273)
                                Metrics
```

That's it! You should now have air quality data flowing into Grafana.
