package httpclient

import (
	"bytes"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

type Client struct {
	logger *logrus.Logger
	client *http.Client
}

func NewClient(logger *logrus.Logger) *Client {
	return &Client{
		logger: logger,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) Post(url string, payload []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.logger.WithFields(logrus.Fields{
		"url":    url,
		"status": resp.StatusCode,
	}).Info("📤 HTTP POST sent")
	return nil
}
