package switchpkg

import (
	"fmt"
	"sync"

	switchhttp "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchhttp"
	"github.com/sirupsen/logrus"
)

type Switch struct {
	Ports map[int]*Port
	mu    sync.Mutex
	HTTP  *switchhttp.SwitchClient
}

type Port struct {
	ID      int
	Enabled bool
}

func NewSwitch(numPorts int, httpClient *switchhttp.SwitchClient) *Switch {
	s := &Switch{Ports: make(map[int]*Port), HTTP: httpClient}
	for i := 1; i <= numPorts; i++ {
		s.Ports[i] = &Port{ID: i, Enabled: false}
	}
	return s
}

func (s *Switch) HandlePoECommand(portID int, payload string, logger *logrus.Logger) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.Ports[portID]
	if !ok {
		logger.WithField("port", portID).Warn("⚠️ Nieznany port")
		return false
	}

	var enable bool
	switch payload {
	case "ON":
		enable = true
	case "OFF":
		enable = false
	default:
		logger.WithFields(map[string]interface{}{"port": portID, "payload": payload}).Warn("⚠️ Nieznana komenda PoE")
		return false
	}

	// Wywołanie HTTP do switcha
	if s.HTTP != nil {
		err := s.HTTP.SetPoE(portID-1, enable) // portID-1 bo switch liczy od 0
		if err != nil {
			logger.WithFields(map[string]interface{}{"port": portID, "error": err}).Error("Błąd HTTP przy ustawianiu PoE")
			return false
		}
	}

	p.Enabled = enable
	if enable {
		logger.WithField("port", portID).Info("🔌 PoE włączone")
	} else {
		logger.WithField("port", portID).Info("❌ PoE wyłączone")
	}

	return true
}

func (s *Switch) GetPortState(portID int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.Ports[portID]
	if !ok {
		return "UNKNOWN"
	}
	if p.Enabled {
		return "ON"
	}
	return "OFF"
}

// ParseTopic wyciąga numer portu z topicu typu dom/switch1/port{n}/poe/set
func ParseTopic(topic string) (portID int, ok bool) {
	var n int
	_, err := fmt.Sscanf(topic, "dom/switch1/port%d/poe/set", &n)
	if err != nil {
		return 0, false
	}
	return n, true
}
