package switchhttp

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
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

// SetPoE ustawia PoE na danym porcie (0-based)
func (s *SwitchClient) SetPoE(portID int, enable bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &http.Client{Timeout: 5 * time.Second}

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

	// --- LOGI ---
	fmt.Printf("➡️ Sending HTTP POST to %s\n", req.URL.String())
	fmt.Printf("    Form data: %s\n", formParams.Encode())
	for _, c := range req.Cookies() {
		fmt.Printf("    Cookie: %s=%s\n", c.Name, c.Value)
	}
	fmt.Printf("    Headers:\n")
	for k, v := range req.Header {
		fmt.Printf("       %s: %s\n", k, strings.Join(v, ", "))
	}
	// --- KONIEC LOGÓW ---

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("⬅️ Response Status: %s\n", resp.Status)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SetPoE failed: %s", resp.Status)
	}

	fmt.Printf("✅ Port %d PoE set to %v\n", portID+1, enable)
	return nil
}

// GetPoEStates pobiera aktualny stan PoE wszystkich portów
func (s *SwitchClient) GetPoEStates(numPorts int) ([]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &http.Client{Timeout: 5 * time.Second}

	formParams := url.Values{}
	formParams.Set("username", s.Username)
	formParams.Set("password", s.Password)
	formParams.Set("language", "EN")
	formParams.Set("Response", getMD5Hash(s.Username+s.Password))

	req, err := http.NewRequest("GET", s.BaseURL+"/pse_port.cgi", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	cookieValue := getMD5Hash(s.Username + s.Password)
	req.AddCookie(&http.Cookie{Name: "admin", Value: cookieValue})
	req.Header.Set("Referer", fmt.Sprintf("http://%s/menu.cgi", strings.TrimPrefix(s.BaseURL, "http://")))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", resp.Status)
	}

	// W tym miejscu możesz sparsować HTML zwracany przez switch
	// i zwrócić slice bool dla każdego portu.
	// Na razie zakładamy prosty stub:
	states := make([]bool, numPorts)
	// TODO: sparsować HTML i ustawić prawidłowe wartości
	// np. states[0] = true jeśli port 1 jest włączony
	return states, nil
}

// getMD5Hash zwraca MD5 w formacie heksadecymalnym
func getMD5Hash(s string) string {
	hash := md5.Sum([]byte(s))
	return hex.EncodeToString(hash[:])
}
