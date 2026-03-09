package registryclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Index struct {
	Integrations []Integration `json:"integrations"`
}

type Integration struct {
	Name     string    `json:"name"`
	Versions []Version `json:"versions"`
}

type Version struct {
	Version    string `json:"version"`
	PackageURL string `json:"package_url"`
	Checksum   string `json:"checksum"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string, httpClient *http.Client) *Client {
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{
		baseURL: strings.TrimSpace(baseURL),
		http:    client,
	}
}

func (c *Client) FetchIndex(ctx context.Context) (Index, error) {
	if c.baseURL == "" {
		return Index{}, fmt.Errorf("integration registry url is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return Index{}, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Index{}, fmt.Errorf("fetch registry index: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Index{}, fmt.Errorf("fetch registry index: unexpected status code %d", resp.StatusCode)
	}
	var index Index
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return Index{}, fmt.Errorf("decode registry index: %w", err)
	}
	return index, nil
}

func (c *Client) Find(index Index, name string) (Integration, bool) {
	name = strings.TrimSpace(name)
	for _, integration := range index.Integrations {
		if strings.TrimSpace(integration.Name) == name {
			return integration, true
		}
	}
	return Integration{}, false
}
