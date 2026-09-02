package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const systemPrompt = `You generate Omurga project manifests.
Return only one valid JSON object matching the requested schema. Do not use Markdown fences.
Never return shell commands, secrets, API keys, passwords, tokens, or private keys.
Use manifest version 1. Use only fields supported by the schema. If the request lacks
information required by the schema, choose a safe explicit default and let the caller review it.
The generated project must be deployable on Debian or Ubuntu with Docker and Caddy.
Use HTTPS only for public domains and include a valid gateway email when HTTPS is enabled.
Do not invent secret values. Use passwordSecret and secret mounts for database credentials.`

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type Client struct {
	Config     Config
	HTTPClient *http.Client
}

func NewClient(config Config) (*Client, error) {
	parsed, err := url.Parse(config.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("LLM endpoint must be an absolute HTTPS URL")
	}
	if strings.TrimSpace(config.Model) == "" || strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("LLM model and API key are required")
	}
	return &Client{Config: config, HTTPClient: &http.Client{Timeout: 120 * time.Second}}, nil
}

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}
	body, err := json.Marshal(chatRequest{
		Model: c.Config.Model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt + "\n\nReturn the complete Omurga manifest as JSON."},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("could not encode LLM request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("could not create LLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return "", fmt.Errorf("could not read LLM response: %w", err)
	}
	var decoded chatResponse
	decodeErr := json.Unmarshal(responseBody, &decoded)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if decodeErr == nil && decoded.Error != nil && decoded.Error.Message != "" {
			message = decoded.Error.Message
		}
		return "", fmt.Errorf("LLM request returned HTTP %d: %s", response.StatusCode, message)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("LLM returned invalid JSON response: %w", decodeErr)
	}
	if len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("LLM response did not contain a completion")
	}
	return strings.TrimSpace(decoded.Choices[0].Message.Content), nil
}
