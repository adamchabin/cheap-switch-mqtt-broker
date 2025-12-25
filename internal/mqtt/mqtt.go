package mqtt

import (
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sirupsen/logrus"
)

type Broker struct {
	client mqtt.Client
	logger *logrus.Logger
	opts   *mqtt.ClientOptions
}

func NewBroker(brokerURL, username, password, clientID string, logger *logrus.Logger) *Broker {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)
	return &Broker{
		opts:   opts,
		logger: logger,
	}
}

func (b *Broker) Connect() error {
	b.client = mqtt.NewClient(b.opts)
	token := b.client.Connect()
	token.Wait()
	if token.Error() != nil {
		return token.Error()
	}
	b.logger.Info("✅ Connected to MQTT broker")
	return nil
}

func (b *Broker) Disconnect() {
	if b.client != nil && b.client.IsConnected() {
		b.client.Disconnect(250)
	}
}

func (b *Broker) Subscribe(topic string, handler func(topic string, payload []byte)) error {
	token := b.client.Subscribe(topic, 0, func(c mqtt.Client, msg mqtt.Message) {
		handler(msg.Topic(), msg.Payload())
	})
	token.Wait()
	if token.Error() != nil {
		return token.Error()
	}
	b.logger.Infof("📡 Subscribed to topic: %s", topic)
	return nil
}

func (b *Broker) Publish(topic string, payload []byte) {
	token := b.client.Publish(topic, 0, false, payload)
	token.WaitTimeout(2 * time.Second)
}
