package switchpkg

import (
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// PoEHTTPClient jest interfejsem potrzebnym Switchowi do ustawiania PoE i pobierania stanu portów.
// Dzięki temu switchpkg nie importuje switchhttp i nie tworzy cyklu.
type PoEHTTPClient interface {
	SetPoE(portID int, enable bool) error
}

// Switch reprezentuje przełącznik z portami PoE.
type Switch struct {
	Ports map[int]*Port
	mu    sync.Mutex
	HTTP  PoEHTTPClient
}

// Port opisuje pojedynczy port przełącznika.
type Port struct {
	ID      int
	Enabled bool
	PowerOn bool
	Class   string
	Stats   *PoEStats
	// dodajemy to pole do śledzenia ostatniego stanu MQTT
	LastPublishedState string
}

// PoEStats przechowuje wartości mocy, napięcia i prądu portu w jednostkach mW, mV, mA.
type PoEStats struct {
	Power_mW   int
	Voltage_mV int
	Current_mA int
}

// NewSwitch tworzy nowy obiekt Switch z podaną liczbą portów i klientem HTTP.
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

// HandlePoECommand włącza lub wyłącza PoE na danym porcie.
// Zwraca true jeśli operacja się powiodła.
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

	// --- Nowość: jeśli stan się nie zmienił, nic nie robimy ---
	if p.Enabled == enable {
		return false
	}

	// Wywołanie HTTP do switcha poprzez interfejs
	if s.HTTP != nil {
		if err := s.HTTP.SetPoE(portID-1, enable); err != nil {
			logger.WithFields(map[string]interface{}{"port": portID, "error": err}).Error("Błąd HTTP przy ustawianiu PoE")
			return false
		}
	}

	// Aktualizacja stanu w pamięci
	p.Enabled = enable
	if enable {
		logger.WithField("port", portID).Info("🔌 PoE włączone")
	} else {
		logger.WithField("port", portID).Info("❌ PoE wyłączone")
	}

	return true
}

// GetPortState zwraca "ON", "OFF" lub "UNKNOWN" w zależności od stanu portu.
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

func (s *Switch) SetPort(portID int, port *Port) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Ports == nil {
		s.Ports = make(map[int]*Port)
	}
	s.Ports[portID] = port
}
