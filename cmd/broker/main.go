package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/adamchabin/cheap-switch-mqtt-broker/internal/mqtt"
	switchpkg "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switch"
	switchhttp "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchhttp"
	"github.com/sirupsen/logrus"
)

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)

	// ENV
	brokerURL := getEnv("MQTT_BROKER", "tcp://localhost:1883")
	username := getEnv("MQTT_USERNAME", "")
	password := getEnv("MQTT_PASSWORD", "")
	clientID := getEnv("MQTT_CLIENT_ID", "cheap-switch-mqtt-bridge")
	topic := getEnv("MQTT_TOPIC", "dom/switch1/#")

	// MQTT
	broker := mqtt.NewBroker(brokerURL, username, password, clientID, logger)
	if err := broker.Connect(); err != nil {
		logger.Fatal(err)
	}

	// Switch HTTP
	switchHTTPClient := switchhttp.NewSwitchClient("http://192.168.1.94", "admin", "admin")

	// Switch z 4 portami
	switches := switchpkg.NewSwitch(8, switchHTTPClient)

	// --- Przy starcie pobieramy stan PoE ---
	states, err := switchHTTPClient.GetPoEStates(8) // liczba portów
	if err != nil {
		logger.WithError(err).Warn("⚠️ Nie udało się pobrać stanu PoE z switcha")
	} else {
		for i, enabled := range states {
			portID := i + 1
			switches.Ports[portID].Enabled = enabled
			stateTopic := fmt.Sprintf("dom/switch1/port%d/poe/state", portID)
			var state string
			if enabled {
				state = "ON"
			} else {
				state = "OFF"
			}
			broker.Publish(stateTopic, []byte(state))
			logger.Infof("Initial state port %d: %s", portID, state)
		}
	}

	// Subskrypcja topiców MQTT
	if err := broker.Subscribe(topic, func(topic string, payload []byte) {
		portID, ok := switchpkg.ParseTopic(topic)
		if !ok {
			logger.WithField("topic", topic).Debug("📌 Ignorowany topic (nie SET)")
			return
		}

		if switches.HandlePoECommand(portID, string(payload), logger) {
			stateTopic := fmt.Sprintf("dom/switch1/port%d/poe/state", portID)
			broker.Publish(stateTopic, []byte(switches.GetPortState(portID)))
		}
	}); err != nil {
		logger.Fatal(err)
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	logger.Info("🛑 Shutting down")
	broker.Disconnect()
}
