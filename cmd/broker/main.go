package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/adamchabin/cheap-switch-mqtt-broker/internal/mqtt"
	switchhttp "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchhttp"
	switchpkg "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchpkg"
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

	switchURL := getEnv("SWITCH_URL", "http://127.0.0.1")
	switchUsername := getEnv("SWITCH_USERNAME", "admin")
	switchPassword := getEnv("SWITCH_PASSWORD", "admin")
	switchPortNumber, err := strconv.Atoi(getEnv("SWITCH_PORT_NUMBER", "8"))

	// MQTT
	broker := mqtt.NewBroker(brokerURL, username, password, clientID, logger)
	if err := broker.Connect(); err != nil {
		logger.Fatal(err)
	}

	// Switch HTTP
	switchHTTPClient := switchhttp.NewSwitchClient(switchURL, switchUsername, switchPassword)
	sw := switchpkg.NewSwitch(switchPortNumber, switchHTTPClient)

	// --- Przy starcie pobieramy stan PoE ---
	states, err := switchHTTPClient.GetPoEStates(switchPortNumber)
	if err != nil {
		logger.WithError(err).Warn("⚠️ Nie udało się pobrać stanu PoE z switcha")
	} else {
		for i, powerOn := range states {
			portID := i + 1

			// 1️⃣ Sprawdzenie, czy mapa istnieje
			if sw.Ports == nil {
				sw.Ports = make(map[int]*switchpkg.Port)
			}

			// 2️⃣ Pobranie portu, jeśli nil — stwórz nowy
			port := sw.Ports[portID]
			if port == nil {
				port = &switchpkg.Port{
					ID:    portID,
					Stats: &switchpkg.PoEStats{},
				}
				sw.Ports[portID] = port
			}

			// 3️⃣ Aktualizacja stanu portu
			port.PowerOn = powerOn
			// Jeśli masz Enabled lub Class z HTML — też zaktualizuj
			if port.Stats == nil {
				port.Stats = &switchpkg.PoEStats{}
			}

			// 4️⃣ Publikacja do MQTT
			stateTopic := fmt.Sprintf("dom/switch1/port%d/poe/state", portID)
			var state string
			if port.PowerOn {
				state = "ON"
			} else {
				state = "OFF"
			}
			broker.Publish(stateTopic, []byte(state))

			// 5️⃣ Dokładny log JSON
			logger.WithFields(logrus.Fields{
				"id":         port.ID,
				"enabled":    port.Enabled,
				"powerOn":    port.PowerOn,
				"class":      port.Class,
				"power_mW":   port.Stats.Power_mW,
				"voltage_mV": port.Stats.Voltage_mV,
				"current_mA": port.Stats.Current_mA,
			}).Info("Initial port state")
		}
	}

	// Subskrypcja topiców MQTT
	if err := broker.Subscribe(topic, func(topic string, payload []byte) {
		portID, ok := switchpkg.ParseTopic(topic)
		if !ok {
			logger.WithField("topic", topic).Debug("📌 Ignorowany topic (nie SET)")
			return
		}

		if sw.HandlePoECommand(portID, string(payload), logger) {
			stateTopic := fmt.Sprintf("dom/switch1/port%d/poe/state", portID)
			broker.Publish(stateTopic, []byte(sw.GetPortState(portID)))
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
