package switchhttp

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	sw "github.com/adamchabin/cheap-switch-mqtt-broker/internal/switchpkg"
)

type SwitchClient struct {
	BaseURL  string
	Username string
	Password string
	mu       sync.Mutex
}

func NewSwitchClient(baseURL, username, password string) *SwitchClient {
	return &SwitchClient{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
	}
}

// SetPoE ustawia port PoE włączony/wyłączony
func (s *SwitchClient) SetPoE(portID int, enable bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	formParams := url.Values{}
	formParams.Set("username", s.Username)
	formParams.Set("password", s.Password)
	formParams.Set("language", "EN")
	formParams.Set("Response", getMD5Hash(s.Username+s.Password))
	formParams.Set("portid", fmt.Sprintf("%d", portID))
	if enable {
		formParams.Set("state", "1")
	} else {
		formParams.Set("state", "0")
	}
	formParams.Set("cmd", "poe")
	formParams.Set("submit", "Apply")

	req, err := http.NewRequest("POST", s.BaseURL+"/pse_port.cgi", strings.NewReader(formParams.Encode()))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	cookieValue := getMD5Hash(s.Username + s.Password)
	req.AddCookie(&http.Cookie{Name: "admin", Value: cookieValue})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", fmt.Sprintf("http://%s/menu.cgi", strings.TrimPrefix(s.BaseURL, "http://")))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SetPoE failed: %s", resp.Status)
	}

	return nil
}

// GetPoEPorts zwraca pełne informacje o wszystkich portach
func (s *SwitchClient) GetPoEPorts(numPorts int) ([]*sw.Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", s.BaseURL+"/pse_port.cgi", nil)
	if err != nil {
		return nil, err
	}

	cookieValue := getMD5Hash(s.Username + s.Password)
	req.AddCookie(&http.Cookie{Name: "admin", Value: cookieValue})
	req.Header.Set("Referer", fmt.Sprintf("http://%s/menu.cgi", strings.TrimPrefix(s.BaseURL, "http://")))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status: %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	ports := make([]*sw.Port, numPorts)

	doc.Find("table tr").Each(func(i int, tr *goquery.Selection) {
		if i == 0 { // nagłówek
			return
		}

		tds := tr.Find("td")
		if tds.Length() < 7 {
			return
		}

		portText := strings.TrimSpace(tds.Eq(0).Text())
		portID, err := strconv.Atoi(strings.TrimPrefix(portText, "Port "))
		if err != nil || portID < 1 || portID > numPorts {
			return
		}

		enabled := strings.TrimSpace(tds.Eq(1).Text()) == "Enable"
		powerOn := strings.TrimSpace(tds.Eq(2).Text()) == "On"
		class := strings.TrimSpace(tds.Eq(3).Text())

		stats := &sw.PoEStats{}
		if v := cleanText(tds.Eq(4).Text()); v != "-" && v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				stats.Power_mW = int(math.Round(f * 1000))
			}
		}
		if v := cleanText(tds.Eq(5).Text()); v != "-" && v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				stats.Voltage_mV = int(math.Round(f * 1000))
			}
		}
		if v := cleanText(tds.Eq(6).Text()); v != "-" && v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				stats.Current_mA = i
			}
		}

		port := &sw.Port{
			ID:      portID,
			Enabled: enabled,
			PowerOn: powerOn,
			Class:   class,
			Stats:   stats,
		}

		ports[portID-1] = port
	})

	return ports, nil
}

// GetPoEStates zwraca tylko slice bool, kompatybilne z PoEHTTPClient
func (s *SwitchClient) GetPoEStates(numPorts int) ([]bool, error) {
	ports, err := s.GetPoEPorts(numPorts)
	if err != nil {
		return nil, err
	}

	states := make([]bool, numPorts)
	for i, port := range ports {
		if port != nil {
			states[i] = port.PowerOn
		}
	}
	return states, nil
}

// getMD5Hash zwraca MD5 w formacie hex
func getMD5Hash(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}

func cleanText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\u00a0", "") // usuń niełamliwe spacje
	return s
}
