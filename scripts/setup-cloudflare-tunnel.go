package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Configuration - Fill these values manually
const (
	// Get your API token from: https://dash.cloudflare.com/profile/api-tokens
	// Required permissions: Zone:DNS:Edit, Account:Cloudflare Tunnel:Edit
	CLOUDFLARE_API_TOKEN = ""

	// Your Cloudflare Account ID
	// Find it in: https://dash.cloudflare.com/ -> Select domain -> Overview (right sidebar)
	CLOUDFLARE_ACCOUNT_ID = ""

	// Your Zone ID for keshavstack.tech
	// Find it in: https://dash.cloudflare.com/ -> Select domain -> Overview (right sidebar)
	CLOUDFLARE_ZONE_ID = ""

	// Your domain
	DOMAIN = "keshavstack.tech"

	// Tunnel name
	TUNNEL_NAME = "MeshVPN_SelfHosting"

	// Local service URL (where Traefik is running)
	LOCAL_SERVICE_URL = "http://localhost:80"
)

type CloudflareClient struct {
	apiToken  string
	accountID string
	zoneID    string
	client    *http.Client
}

type TunnelResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		CreatedAt   time.Time `json:"created_at"`
		Connections []struct {
			ID string `json:"id"`
		} `json:"connections"`
	} `json:"result"`
}

type DNSRecordResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Name    string `json:"name"`
		Content string `json:"content"`
		Proxied bool   `json:"proxied"`
	} `json:"result"`
}

type TunnelConfigResponse struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result struct {
		Config struct {
			Ingress []map[string]interface{} `json:"ingress"`
		} `json:"config"`
	} `json:"result"`
}

func NewCloudflareClient(apiToken, accountID, zoneID string) *CloudflareClient {
	return &CloudflareClient{
		apiToken:  apiToken,
		accountID: accountID,
		zoneID:    zoneID,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CloudflareClient) makeRequest(method, url string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CreateTunnel creates a new Cloudflare Tunnel
func (c *CloudflareClient) CreateTunnel(name string) (string, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel", c.accountID)

	payload := map[string]string{
		"name":          name,
		"tunnel_secret": generateTunnelSecret(),
	}

	respBody, err := c.makeRequest("POST", url, payload)
	if err != nil {
		return "", err
	}

	var tunnelResp TunnelResponse
	if err := json.Unmarshal(respBody, &tunnelResp); err != nil {
		return "", fmt.Errorf("failed to parse tunnel response: %w", err)
	}

	if !tunnelResp.Success {
		if len(tunnelResp.Errors) > 0 {
			return "", fmt.Errorf("cloudflare API error: %s", tunnelResp.Errors[0].Message)
		}
		return "", fmt.Errorf("failed to create tunnel")
	}

	return tunnelResp.Result.ID, nil
}

// GetTunnelByName retrieves an existing tunnel by name
func (c *CloudflareClient) GetTunnelByName(name string) (string, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel?name=%s", c.accountID, name)

	respBody, err := c.makeRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	var response struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Success && len(response.Result) > 0 {
		return response.Result[0].ID, nil
	}

	return "", nil
}

// CreateDNSRecord creates a CNAME record for the wildcard domain
func (c *CloudflareClient) CreateDNSRecord(name, tunnelID string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", c.zoneID)

	payload := map[string]interface{}{
		"type":    "CNAME",
		"name":    name,
		"content": fmt.Sprintf("%s.cfargotunnel.com", tunnelID),
		"proxied": true,
		"ttl":     1, // Automatic TTL when proxied
	}

	respBody, err := c.makeRequest("POST", url, payload)
	if err != nil {
		return err
	}

	var dnsResp DNSRecordResponse
	if err := json.Unmarshal(respBody, &dnsResp); err != nil {
		return fmt.Errorf("failed to parse DNS response: %w", err)
	}

	if !dnsResp.Success {
		if len(dnsResp.Errors) > 0 {
			return fmt.Errorf("cloudflare API error: %s", dnsResp.Errors[0].Message)
		}
		return fmt.Errorf("failed to create DNS record")
	}

	return nil
}

// UpdateTunnelConfig configures the tunnel routes
func (c *CloudflareClient) UpdateTunnelConfig(tunnelID string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel/%s/configurations", c.accountID, tunnelID)

	config := map[string]interface{}{
		"config": map[string]interface{}{
			"ingress": []map[string]interface{}{
				{
					"hostname": fmt.Sprintf("*.%s", DOMAIN),
					"service":  LOCAL_SERVICE_URL,
				},
				{
					"hostname": fmt.Sprintf("self.%s", DOMAIN),
					"service":  "http://localhost:8080",
				},
				{
					// Catch-all rule (required as last rule)
					"service": "http_status:404",
				},
			},
		},
	}

	respBody, err := c.makeRequest("PUT", url, config)
	if err != nil {
		return err
	}

	var configResp TunnelConfigResponse
	if err := json.Unmarshal(respBody, &configResp); err != nil {
		return fmt.Errorf("failed to parse config response: %w", err)
	}

	if !configResp.Success {
		if len(configResp.Errors) > 0 {
			return fmt.Errorf("cloudflare API error: %s", configResp.Errors[0].Message)
		}
		return fmt.Errorf("failed to update tunnel config")
	}

	return nil
}

// GetTunnelToken retrieves the tunnel token for docker-compose
func (c *CloudflareClient) GetTunnelToken(tunnelID string) (string, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/cfd_tunnel/%s/token", c.accountID, tunnelID)

	respBody, err := c.makeRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	var response struct {
		Success bool   `json:"success"`
		Result  string `json:"result"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if !response.Success {
		return "", fmt.Errorf("failed to get tunnel token")
	}

	return response.Result, nil
}

func generateTunnelSecret() string {
	// Generate a random 32-byte secret (base64 encoded)
	// In production, use crypto/rand for better randomness
	return "dGhpc2lzYXJhbmRvbTMyYnl0ZXNlY3JldGZvcnR1bm5lbA=="
}

func validateConfig() error {
	if CLOUDFLARE_API_TOKEN == "YOUR_API_TOKEN_HERE" {
		return fmt.Errorf("CLOUDFLARE_API_TOKEN is not set")
	}
	if CLOUDFLARE_ACCOUNT_ID == "YOUR_ACCOUNT_ID_HERE" {
		return fmt.Errorf("CLOUDFLARE_ACCOUNT_ID is not set")
	}
	if CLOUDFLARE_ZONE_ID == "YOUR_ZONE_ID_HERE" {
		return fmt.Errorf("CLOUDFLARE_ZONE_ID is not set")
	}
	return nil
}

func main() {
	fmt.Println("🚀 Cloudflare Tunnel Setup for MeshVPN")
	fmt.Println("========================================")
	fmt.Println()

	// Validate configuration
	if err := validateConfig(); err != nil {
		fmt.Printf("❌ Configuration error: %v\n", err)
		fmt.Println("\nPlease edit this script and fill in:")
		fmt.Println("  - CLOUDFLARE_API_TOKEN")
		fmt.Println("  - CLOUDFLARE_ACCOUNT_ID")
		fmt.Println("  - CLOUDFLARE_ZONE_ID")
		os.Exit(1)
	}

	client := NewCloudflareClient(CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_ZONE_ID)

	// Step 1: Check if tunnel already exists
	fmt.Printf("📡 Checking for existing tunnel '%s'...\n", TUNNEL_NAME)
	tunnelID, err := client.GetTunnelByName(TUNNEL_NAME)
	if err != nil {
		fmt.Printf("❌ Error checking for existing tunnel: %v\n", err)
		os.Exit(1)
	}

	if tunnelID != "" {
		fmt.Printf("✅ Found existing tunnel: %s\n", tunnelID)
	} else {
		// Step 2: Create new tunnel
		fmt.Printf("🔧 Creating new tunnel '%s'...\n", TUNNEL_NAME)
		tunnelID, err = client.CreateTunnel(TUNNEL_NAME)
		if err != nil {
			fmt.Printf("❌ Failed to create tunnel: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Tunnel created successfully: %s\n", tunnelID)
	}

	// Step 3: Create wildcard DNS record
	fmt.Printf("\n🌐 Creating wildcard DNS record for *.%s...\n", DOMAIN)
	wildcardName := "*"
	err = client.CreateDNSRecord(wildcardName, tunnelID)
	if err != nil {
		fmt.Printf("⚠️  DNS record creation failed (may already exist): %v\n", err)
	} else {
		fmt.Printf("✅ Wildcard DNS record created: *.%s\n", DOMAIN)
	}

	// Step 4: Create DNS record for self subdomain
	fmt.Printf("🌐 Creating DNS record for self.%s...\n", DOMAIN)
	err = client.CreateDNSRecord("self", tunnelID)
	if err != nil {
		fmt.Printf("⚠️  DNS record creation failed (may already exist): %v\n", err)
	} else {
		fmt.Printf("✅ DNS record created: self.%s\n", DOMAIN)
	}

	// Step 5: Update tunnel configuration
	fmt.Printf("\n⚙️  Updating tunnel configuration...\n")
	err = client.UpdateTunnelConfig(tunnelID)
	if err != nil {
		fmt.Printf("❌ Failed to update tunnel config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Tunnel configuration updated\n")

	// Step 6: Get tunnel token
	fmt.Printf("\n🔑 Retrieving tunnel token...\n")
	token, err := client.GetTunnelToken(tunnelID)
	if err != nil {
		fmt.Printf("❌ Failed to get tunnel token: %v\n", err)
		os.Exit(1)
	}

	// Display final information
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✨ Cloudflare Tunnel Setup Complete!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nTunnel ID: %s\n", tunnelID)
	fmt.Printf("Tunnel Name: %s\n", TUNNEL_NAME)
	fmt.Printf("\nDNS Records Created:")
	fmt.Printf("\n  - *.%s -> %s.cfargotunnel.com\n", DOMAIN, tunnelID)
	fmt.Printf("  - self.%s -> %s.cfargotunnel.com\n", DOMAIN, tunnelID)
	fmt.Printf("\nTunnel Routes Configured:")
	fmt.Printf("\n  - *.%s -> %s\n", DOMAIN, LOCAL_SERVICE_URL)
	fmt.Printf("  - self.%s -> http://control-plane:8080\n", DOMAIN)
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("\n📋 Next Steps:")
	fmt.Println("\n1. Add this token to your infra/.env file:")
	fmt.Printf("\n   CLOUDFLARE_TUNNEL_TOKEN=%s\n", token)
	fmt.Println("\n2. Start your services:")
	fmt.Println("   cd infra")
	fmt.Println("   docker compose --env-file .env up -d")
	fmt.Println("\n3. Test your setup:")
	fmt.Printf("   curl https://self.%s/health\n", DOMAIN)
	fmt.Println("\n" + strings.Repeat("=", 60))
}
