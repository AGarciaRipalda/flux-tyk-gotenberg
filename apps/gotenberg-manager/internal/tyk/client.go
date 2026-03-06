package tyk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	BaseURL  string
	AdminKey string
	http     *http.Client
}

func NewClient(baseURL, adminKey string) *Client {
	return &Client{
		BaseURL:  baseURL,
		AdminKey: adminKey,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type CreateKeyRequest struct {
	Allowance    int                    `json:"allowance"`
	Rate         int                    `json:"rate"`
	Per          int                    `json:"per"`
	Expires      int                    `json:"expires"`
	QuotaMax     int                    `json:"quota_max"`
	OrgID        string                 `json:"org_id"`
	AccessRights map[string]AccessRight `json:"access_rights"`
}

type AccessRight struct {
	APIID    string   `json:"api_id"`
	APIName  string   `json:"api_name"`
	Versions []string `json:"versions"`
}

type CreateKeyResponse struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Action  string `json:"action"`
	KeyHash string `json:"key_hash"`
}

// CreateKey creates a new API key in Tyk for a client
func (c *Client) CreateKey(rate, per, quotaMax int) (*CreateKeyResponse, error) {
	reqBody := CreateKeyRequest{
		Allowance: rate,
		Rate:      rate,
		Per:       per,
		Expires:   -1,
		QuotaMax:  int64ToInt(quotaMax),
		OrgID:     "default",
		AccessRights: map[string]AccessRight{
			"gotenberg-v1": {
				APIID:    "gotenberg-v1",
				APIName:  "Gotenberg PDF API",
				Versions: []string{"Default"},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/tyk/keys/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("x-tyk-authorization", c.AdminKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Tyk API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tyk returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CreateKeyResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// DeleteKey removes an API key from Tyk
func (c *Client) DeleteKey(keyID string) error {
	req, err := http.NewRequest("DELETE", c.BaseURL+"/tyk/keys/"+keyID, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("x-tyk-authorization", c.AdminKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call Tyk API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Tyk returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func int64ToInt(v int) int {
	if v <= 0 {
		return -1
	}
	return v
}
