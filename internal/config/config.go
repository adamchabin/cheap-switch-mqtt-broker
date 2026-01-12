package config

import (
	"log"
	"os"
	"strconv"
)

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

var Debug bool

var BrokerURL string = getEnv("MQTT_BROKER", "tcp://localhost:1883")
var BrokerUsername string = getEnv("MQTT_USERNAME", "")
var BrokerPassword string = getEnv("MQTT_PASSWORD", "")
var BrokerClientID string = getEnv("MQTT_CLIENT_ID", "cheap-switch-mqtt-bridge")
var BrokerTopic string = getEnv("MQTT_TOPIC", "dom/switch1/#")

var SwitchURL string = getEnv("SWITCH_URL", "http://127.0.0.1")
var SwitchUsername string = getEnv("SWITCH_USERNAME", "admin")
var SwitchPassword string = getEnv("SWITCH_PASSWORD", "admin")
var SwitchPortNumber int

func init() {
	var err error
	SwitchPortNumber, err = strconv.Atoi(getEnv("SWITCH_PORT_NUMBER", "8"))
	if err != nil {
		SwitchPortNumber = 8
	}
	val := getEnv("DEBUG", "0")

	Debug = val == "1"
	log.Printf("DEBUG mode: %v", Debug)
}
