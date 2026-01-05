package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	config "github.com/adamchabin/cheap-switch-mqtt-broker/internal/config"
	"github.com/adamchabin/cheap-switch-mqtt-broker/internal/mqtt"
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

	// MQTT
	broker := mqtt.NewBroker(config.BrokerURL, config.BrokerUsername, config.BrokerPassword, config.BrokerClientID, logger)
	if err := broker.Connect(); err != nil {
		logger.Fatal(err)
	}

	// Switch HTTP
	switchHTTPClient := switchhttp.NewSwitchClient(
		config.SwitchURL,
		config.SwitchUsername,
		config.SwitchPassword,
	)

	swClient := sw.NewSwitch(config.SwitchPortNumber, switchHTTPClient)

	// get ports status from switch
	ports, err := switchHTTPClient.GetPoEPorts(config.SwitchPortNumber)
	if err != nil {
		logger.WithError(err).Fatal("Nie udało się pobrać portów PoE")
	}

	for _, port := range ports {
		if port != nil {
			swClient.SetPort(port.ID, port)

			if port.Enabled == true {
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
				logger.Infof(
					"Port %d: Enabled=%t",
					port.ID,
					port.Enabled,
				)
			}
		}
	}

	// Subskrypcja topiców MQTT
	if err := broker.Subscribe(config.BrokerTopic, func(topic string, payload []byte) {
		portID, ok := sw.ParseTopic(topic)
		if !ok {
			logger.WithField("topic", topic).Debug("📌 Ignorowany topic (nie SET)")
			return
		}

		if swClient.HandlePoECommand(portID, string(payload), logger) {
			stateTopic := fmt.Sprintf("%s/port%d/poe/state", config.BrokerTopic, portID)
			broker.Publish(stateTopic, []byte(swClient.GetPortState(portID)))
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
