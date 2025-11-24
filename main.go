package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	temperature = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_temperature_celsius",
		Help: "Temperature in Celsius",
	}, []string{"device"})

	humidity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_humidity_percent",
		Help: "Humidity percentage",
	}, []string{"device"})

	co2 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_co2_ppm",
		Help: "CO2 level in parts per million",
	}, []string{"device"})

	pm25 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_pm25_ugm3",
		Help: "PM2.5 in micrograms per cubic meter",
	}, []string{"device"})

	pm10 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_pm10_ugm3",
		Help: "PM10 in micrograms per cubic meter",
	}, []string{"device"})

	tvoc = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_tvoc_ppb",
		Help: "TVOC in parts per billion",
	}, []string{"device"})

	battery = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_battery_percent",
		Help: "Battery percentage",
	}, []string{"device"})

	lastUpdate = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qingping_last_update_timestamp",
		Help: "Timestamp of last sensor update",
	}, []string{"device"})

	// Track last update time for each device to expire stale metrics
	lastUpdateTimes = make(map[string]time.Time)
	lastUpdateMutex sync.RWMutex
)

type Config struct {
	MQTTBroker     string
	MQTTPort       string
	MQTTUsername   string
	MQTTPassword   string
	DeviceMAC      string // MAC address of your CGDN1
	DeviceName     string
	UpdateInterval int    // seconds between data requests (Type 12)
	Duration       int    // how long device should keep reporting (seconds)
	MetricsPort    string // Prometheus metrics port
}

// CGDN1Data represents the Air Monitor Lite sensor data
type CGDN1Data struct {
	Temperature float64   `json:"temperature"` // °C
	Humidity    float64   `json:"humidity"`    // %
	CO2         int       `json:"co2"`         // ppm
	PM25        float64   `json:"pm25"`        // μg/m³
	PM10        float64   `json:"pm10"`        // μg/m³
	TVOC        float64   `json:"tvoc"`        // ppb
	Battery     int       `json:"battery"`     // %
	Timestamp   time.Time `json:"timestamp"`
}

// QingpingConfigMessage represents the Type 12 message for requesting data
type QingpingConfigMessage struct {
	Type     string `json:"type"`
	UpItvl   string `json:"up_itvl"`  // update interval in seconds
	Duration string `json:"duration"` // how long to report (in seconds)
}

// QingpingSettingMessage represents Type 17 message for changing settings
type QingpingSettingMessage struct {
	Type    string                 `json:"type"`
	Setting map[string]interface{} `json:"setting"`
}

// QingpingUpMessage represents the response from /up topic
type QingpingUpMessage struct {
	Type       string                   `json:"type"`
	SensorData []map[string]SensorValue `json:"sensorData"`
}

type SensorValue struct {
	Value float64 `json:"value"`
}

func main() {
	config := Config{
		MQTTBroker:     getEnv("MQTT_BROKER", "mosquitto"),
		MQTTPort:       getEnv("MQTT_PORT", "1883"),
		MQTTUsername:   getEnv("MQTT_USERNAME", ""),
		MQTTPassword:   getEnv("MQTT_PASSWORD", ""),
		DeviceMAC:      getEnv("DEVICE_MAC", ""), // e.g., "582D34123456"
		DeviceName:     getEnv("DEVICE_NAME", "living_room"),
		UpdateInterval: getEnvInt("UPDATE_INTERVAL", 60), // 60 seconds default
		Duration:       getEnvInt("DURATION", 21600),     // 6 hours default
		MetricsPort:    getEnv("METRICS_PORT", "9273"),   // Prometheus metrics port
	}

	if config.DeviceMAC == "" {
		log.Fatal("DEVICE_MAC environment variable is required")
	}

	// Start Prometheus metrics server
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Printf("Starting Prometheus metrics server on :%s", config.MetricsPort)
		if err := http.ListenAndServe(":"+config.MetricsPort, nil); err != nil {
			log.Fatalf("Failed to start metrics server: %v", err)
		}
	}()

	// Setup MQTT client
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", config.MQTTBroker, config.MQTTPort))
	opts.SetClientID("qingping_collector")
	opts.SetUsername(config.MQTTUsername)
	opts.SetPassword(config.MQTTPassword)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)

	opts.OnConnect = func(client mqtt.Client) {
		log.Println("Connected to MQTT broker")
		subscribeToCGDN1(client, config)
		// Send initial config message
		sendConfigMessage(client, config)
	}

	opts.OnConnectionLost = func(client mqtt.Client, err error) {
		log.Printf("Connection lost: %v", err)
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}

	log.Println("Qingping CGDN1 collector started")
	log.Printf("Requesting data every %d seconds for duration of %d seconds (%d hours)",
		config.UpdateInterval, config.Duration, config.Duration/3600)

	// Setup periodic config messages to keep device reporting
	ticker := time.NewTicker(time.Duration(2*config.UpdateInterval) * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			log.Println("Refreshing device configuration...")
			sendConfigMessage(client, config)
		}
	}()

	// Setup periodic cleanup of stale metrics
	// Check every updateInterval seconds for expired metrics
	cleanupTicker := time.NewTicker(time.Duration(config.UpdateInterval) * time.Second)
	defer cleanupTicker.Stop()

	go func() {
		for range cleanupTicker.C {
			cleanupStaleMetrics(config.UpdateInterval)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	client.Disconnect(250)
}

func subscribeToCGDN1(client mqtt.Client, config Config) {
	// Subscribe to the /up topic where device publishes data
	upTopic := fmt.Sprintf("qingping/%s/up", config.DeviceMAC)

	token := client.Subscribe(upTopic, 0, func(client mqtt.Client, msg mqtt.Message) {
		handleCGDN1Message(msg, config.DeviceName)
	})

	if token.Wait() && token.Error() != nil {
		log.Printf("Failed to subscribe to %s: %v", upTopic, token.Error())
	} else {
		log.Printf("Subscribed to: %s", upTopic)
	}
}

func sendConfigMessage(client mqtt.Client, config Config) {
	downTopic := fmt.Sprintf("qingping/%s/down", config.DeviceMAC)

	// Type 12 message: Request data at specified interval for specified duration
	configMsg := QingpingConfigMessage{
		Type:     "12",
		UpItvl:   fmt.Sprintf("%d", config.UpdateInterval),
		Duration: fmt.Sprintf("%d", config.Duration),
	}

	payload, err := json.Marshal(configMsg)
	if err != nil {
		log.Printf("Failed to marshal config message: %v", err)
		return
	}

	token := client.Publish(downTopic, 0, false, payload)
	if token.Wait() && token.Error() != nil {
		log.Printf("Failed to publish config to %s: %v", downTopic, token.Error())
	} else {
		log.Printf("Sent Type 12 config to %s (interval: %ds, duration: %ds)",
			downTopic, config.UpdateInterval, config.Duration)
	}
}

func cleanupStaleMetrics(updateInterval int) {
	// Expire metrics after 2x the update interval
	expirationDuration := time.Duration(updateInterval*2) * time.Second

	lastUpdateMutex.Lock()
	defer lastUpdateMutex.Unlock()

	now := time.Now()
	for deviceName, lastTime := range lastUpdateTimes {
		if now.Sub(lastTime) > expirationDuration {
			log.Printf("Device '%s' has not responded in %v, removing stale metrics", deviceName, now.Sub(lastTime))

			// Delete all metrics for this device
			temperature.DeleteLabelValues(deviceName)
			humidity.DeleteLabelValues(deviceName)
			co2.DeleteLabelValues(deviceName)
			pm25.DeleteLabelValues(deviceName)
			pm10.DeleteLabelValues(deviceName)
			tvoc.DeleteLabelValues(deviceName)
			battery.DeleteLabelValues(deviceName)

			// Remove from tracking map
			delete(lastUpdateTimes, deviceName)
		}
	}
}

func handleCGDN1Message(msg mqtt.Message, deviceName string) {
	// Try to parse as JSON
	var upMsg QingpingUpMessage
	if err := json.Unmarshal(msg.Payload(), &upMsg); err != nil {
		log.Printf("Failed to parse message as JSON: %v", err)
		return
	}

	// Skip Type 17 and Type 13 (config responses without sensor data)
	if upMsg.Type == "17" || upMsg.Type == "13" {
		return
	}

	// Check if there's sensor data in the message
	if len(upMsg.SensorData) == 0 {
		return
	}

	if len(upMsg.SensorData) == 0 {
		log.Printf("No sensor data in message")
		return
	}

	sensorData := CGDN1Data{
		Timestamp: time.Now(),
	}

	// Extract values from the first sensor data entry
	data := upMsg.SensorData[0]

	if val, ok := data["temperature"]; ok {
		sensorData.Temperature = val.Value
		temperature.WithLabelValues(deviceName).Set(val.Value)
	}
	if val, ok := data["humidity"]; ok {
		sensorData.Humidity = val.Value
		humidity.WithLabelValues(deviceName).Set(val.Value)
	}
	if val, ok := data["co2"]; ok {
		sensorData.CO2 = int(val.Value)
		co2.WithLabelValues(deviceName).Set(val.Value)
	}
	if val, ok := data["pm25"]; ok {
		sensorData.PM25 = val.Value
		pm25.WithLabelValues(deviceName).Set(val.Value)
	}
	if val, ok := data["pm10"]; ok {
		sensorData.PM10 = val.Value
		pm10.WithLabelValues(deviceName).Set(val.Value)
	}
	if val, ok := data["tvoc"]; ok {
		sensorData.TVOC = val.Value
		tvoc.WithLabelValues(deviceName).Set(val.Value)
	}
	if val, ok := data["battery"]; ok {
		sensorData.Battery = int(val.Value)
		battery.WithLabelValues(deviceName).Set(val.Value)
	}

	// Update last update timestamp
	now := time.Now()
	lastUpdate.WithLabelValues(deviceName).Set(float64(now.Unix()))

	// Track update time for metric expiration
	lastUpdateMutex.Lock()
	lastUpdateTimes[deviceName] = now
	lastUpdateMutex.Unlock()

	// Log the data
	log.Printf("[%s] Temp: %.1f°C, Humidity: %.1f%%, CO2: %d ppm, PM2.5: %.1f μg/m³, PM10: %.1f μg/m³, TVOC: %.0f ppb, Battery: %d%%",
		deviceName,
		sensorData.Temperature,
		sensorData.Humidity,
		sensorData.CO2,
		sensorData.PM25,
		sensorData.PM10,
		sensorData.TVOC,
		sensorData.Battery,
	)
}

func limitString(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}
