package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	config "github.com/adamchabin/cheap-switch-mqtt-broker/internal/config"
	mqtt "github.com/adamchabin/cheap-switch-mqtt-broker/internal/mqtt"
	switchhttp "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchhttp"
	sw "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchpkg"
	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetLevel(logrus.InfoLevel)
	if config.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}

	// --- MQTT ---
	broker := mqtt.NewBroker(config.BrokerURL, config.BrokerUsername, config.BrokerPassword, config.BrokerClientID, logger)
	if err := broker.Connect(); err != nil {
		logger.Fatal(err)
	}
	defer broker.Disconnect()

	// --- Switch HTTP ---
	switchHTTPClient := switchhttp.NewSwitchClient(
		config.SwitchURL,
		config.SwitchUsername,
		config.SwitchPassword,
	)

	swClient := sw.NewSwitch(config.SwitchPortNumber, switchHTTPClient)

	// --- Początkowy stan portów ---
	ports, err := switchHTTPClient.GetPoEPorts(config.SwitchPortNumber)
	if err != nil {
		logger.WithError(err).Fatal("Nie udało się pobrać portów PoE")
	}

	// Mapa stanu portów (ID -> Port)
	portState := make(map[int]*sw.Port)

	for _, port := range ports {
		if port != nil {
			swClient.SetPort(port.ID, port)
			portState[port.ID] = port
			// logAndPublishPort(port, swClient, broker, config.BrokerTopic, logger)
		}
	}

	// --- Subskrypcja topiców MQTT ---
	subscribeTopic := config.BrokerTopic + "/#"
	if err := broker.Subscribe(subscribeTopic, func(topic string, payload []byte) {
		// --- DEBUG log przychodzącej wiadomości ---
		logger.WithFields(logrus.Fields{
			"topic":   topic,
			"payload": string(payload),
		}).Debug("📥 Otrzymano wiadomość na kolejce MQTT")

		portID, ok := sw.ParseTopic(topic)
		if !ok {
			logger.WithField("topic", topic).Debug("📌 Ignorowany topic (nie SET)")
			return
		}

		// --- INFO: komenda typu portX/poe/set ---
		logger.WithFields(logrus.Fields{
			"port":    portID,
			"payload": string(payload),
		}).Info("⚡ Otrzymano komendę PoE")

		if swClient.HandlePoECommand(portID, string(payload), logger) {
			stateTopic := fmt.Sprintf("%s/port%d/poe/state", config.BrokerTopic, portID)
			broker.Publish(stateTopic, []byte(swClient.GetPortState(portID)))
		}
	}); err != nil {
		logger.Fatal(err)
	}

	// --- Goroutine: cykliczne sprawdzanie portów ---
	go func() {
		checkInterval := 5 * time.Second
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		// pierwszy Publish po subskrypcji
		for _, port := range ports {
			if port != nil {
				publishPortValues(port, swClient, broker, config.BrokerTopic, logger)
			}
		}

		for range ticker.C {
			logger.Debugf("Getting PoE ports status via HTTP...")

			var err error
			maxRetries := 10

			for i := 0; i < maxRetries; i++ {
				ports, err = switchHTTPClient.GetPoEPorts(config.SwitchPortNumber)
				if err == nil {
					break // udało się pobrać porty
				}

				logger.Warnf(
					"Błąd pobierania portów PoE, próba %d/%d",
					i+1,
					maxRetries,
				)
				// reconnect
				switchHTTPClient = switchhttp.NewSwitchClient(
					config.SwitchURL,
					config.SwitchUsername,
					config.SwitchPassword,
				)

				time.Sleep(1 * time.Second)
			}

			if err != nil {
				logger.WithError(err).Error("Nie udało się pobrać portów PoE po kilku próbach")
				continue
			}

			for _, port := range ports {
				if port == nil {
					continue
				}

				oldPort := portState[port.ID]
				portState[port.ID] = port
				swClient.SetPort(port.ID, port)

				if oldPort == nil {
					// pierwszy odczyt, publikujemy wszystko
					publishPortValues(port, swClient, broker, config.BrokerTopic, logger)
					continue
				}

				// --- Sprawdzenie, które pola się zmieniły ---
				if oldPort.Enabled != port.Enabled || oldPort.PowerOn != port.PowerOn {
					logger.Infof(
						"Port %d: Enabled=%t, PoE=%t, Class=%s, Power=%dmW, Voltage=%dmV, Current=%dmA",
						port.ID,
						port.Enabled,
						port.PowerOn,
						port.Class,
						port.Stats.Power_mW,
						port.Stats.Voltage_mV,
						port.Stats.Current_mA,
					)
				}

				if oldPort.Enabled != port.Enabled || oldPort.PowerOn != port.PowerOn {
					broker.Publish(fmt.Sprintf("%s/port%d/poe/state", config.BrokerTopic, port.ID), []byte(swClient.GetPortState(port.ID)))
				}
				if oldPort.Stats != nil && port.Stats != nil {
					if oldPort.Stats.Current_mA != port.Stats.Current_mA {
						broker.Publish(fmt.Sprintf("%s/port%d/poe/current", config.BrokerTopic, port.ID), []byte(fmt.Sprintf("%d", port.Stats.Current_mA)))
					}
					if oldPort.Stats.Voltage_mV != port.Stats.Voltage_mV {
						broker.Publish(fmt.Sprintf("%s/port%d/poe/voltage", config.BrokerTopic, port.ID), []byte(fmt.Sprintf("%d", port.Stats.Voltage_mV)))
					}
					if oldPort.Stats.Power_mW != port.Stats.Power_mW {
						broker.Publish(fmt.Sprintf("%s/port%d/poe/power", config.BrokerTopic, port.ID), []byte(fmt.Sprintf("%d", port.Stats.Power_mW)))
					}
					if oldPort.Class != port.Class {
						broker.Publish(fmt.Sprintf("%s/port%d/poe/class", config.BrokerTopic, port.ID), []byte(port.Class))
					}
				} else if oldPort.Stats != port.Stats {
					publishPortValues(port, swClient, broker, config.BrokerTopic, logger)
				}
			}
		}
	}()
	// --- Koniec goroutine ---

	// --- Graceful shutdown ---
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	logger.Info("🛑 Shutting down")
}

func publishPortValues(port *sw.Port, swClient *sw.Switch, broker *mqtt.Broker, topicBase string, logger *logrus.Logger) {
	if port.PowerOn && port.Stats != nil {
		logger.Info("Initial port status:")
		logger.Infof(
			"Port %d: Enabled=%t, PoE=%t, Class=%s, Power=%dmW, Voltage=%dmV, Current=%dmA",
			port.ID,
			port.Enabled,
			port.PowerOn,
			port.Class,
			port.Stats.Power_mW,
			port.Stats.Voltage_mV,
			port.Stats.Current_mA,
		)
	} else {
		logger.Infof("Port %d: Enabled=%t", port.ID, port.Enabled)
	}

	broker.Publish(fmt.Sprintf("%s/port%d/poe/state", topicBase, port.ID), []byte(swClient.GetPortState(port.ID)))
	if port.Stats != nil {
		broker.Publish(fmt.Sprintf("%s/port%d/poe/current", topicBase, port.ID), []byte(fmt.Sprintf("%d", port.Stats.Current_mA)))
		broker.Publish(fmt.Sprintf("%s/port%d/poe/voltage", topicBase, port.ID), []byte(fmt.Sprintf("%d", port.Stats.Voltage_mV)))
		broker.Publish(fmt.Sprintf("%s/port%d/poe/power", topicBase, port.ID), []byte(fmt.Sprintf("%d", port.Stats.Power_mW)))
		broker.Publish(fmt.Sprintf("%s/port%d/poe/class", topicBase, port.ID), []byte(port.Class))
	}
}
