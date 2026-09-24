package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type client struct {
	http    *http.Client
	baseURL string
	token   string
}

func newClient(baseURL, token string) *client {
	return &client{http: &http.Client{Timeout: 20 * time.Second}, baseURL: strings.TrimRight(baseURL, "/"), token: token}
}

func (c *client) get(url string, headers map[string]string, dst any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}
