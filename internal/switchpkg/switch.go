package switchpkg

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

type PoEHTTPClient interface {
	SetPoE(portID int, enable bool) error
}

type Switch struct {
	Ports map[int]*Port
	mu    sync.Mutex
	HTTP  PoEHTTPClient
}

type Port struct {
	ID                 int
	Enabled            bool
	PowerOn            bool
	Class              string
	Stats              *PoEStats
	LastPublishedState string
}

type PoEStats struct {
	Power_mW   int
	Voltage_mV int
	Current_mA int
}

func NewSwitch(numPorts int, httpClient PoEHTTPClient) *Switch {
	s := &Switch{
		Ports: make(map[int]*Port),
		HTTP:  httpClient,
	}
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
		logger.WithField("port", portID).Warn("⚠️ Unknown port")
		return false
	}

	var enable bool
	switch payload {
	case "ON":
		enable = true
	case "OFF":
		enable = false
	default:
		logger.WithFields(map[string]interface{}{"port": portID, "payload": payload}).Warn("⚠️ Unknown PoE command")
		return false
	}

	if p.Enabled == enable {
		return false
	}

	if s.HTTP != nil {
		if err := s.HTTP.SetPoE(portID-1, enable); err != nil {
			logger.WithFields(map[string]interface{}{"port": portID, "error": err}).Error("HTTP error while setting PoE")
			return false
		}
	}

	p.Enabled = enable
	if enable {
		logger.WithField("port", portID).Info("🔌 PoE enabled")
	} else {
		logger.WithField("port", portID).Info("❌ PoE disabled")
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

func ParseTopic(topic string) (portID int, ok bool) {
	var n int
	_, err := fmt.Sscanf(topic, "dom/switch1/port%d/poe/set", &n)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (s *Switch) SetPort(portID int, port *Port) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Ports == nil {
		s.Ports = make(map[int]*Port)
	}
	s.Ports[portID] = port
}
