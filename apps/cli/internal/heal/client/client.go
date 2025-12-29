package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	// defaultRequestTimeout is the per-request timeout for API calls.
	// This applies to each retry attempt individually.
	defaultRequestTimeout = 30 * time.Second
)

// Client wraps the Anthropic SDK for healing operations.
type Client struct {
	api anthropic.Client
}

// API returns the underlying Anthropic client.
func (c *Client) API() anthropic.Client {
	return c.api
}

// New creates a new Anthropic client with the provided API key.
// The key should already be resolved via persistence.ResolveAPIKey().
func New(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("no API key provided")
	}

	return &Client{
		api: anthropic.NewClient(
			option.WithAPIKey(apiKey),
			option.WithRequestTimeout(defaultRequestTimeout),
		),
	}, nil
}

// Test verifies the API connection by sending a simple request.
// Uses Haiku for cost efficiency (~$0.0002/call).
func (c *Client) Test(ctx context.Context) (string, error) {
	msg, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude3_5HaikuLatest,
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Say 'Hello from Claude!' in exactly 5 words.")),
		},
	})
	if err != nil {
		return "", formatAPIError(err)
	}

	// Use index-based iteration to avoid copying 600-byte blocks per iteration.
	// gocritic rangeValCopy flags this if we use `for _, block := range`.
	for i := range msg.Content {
		if text, ok := msg.Content[i].AsAny().(anthropic.TextBlock); ok {
			return text.Text, nil
		}
	}
	return "", fmt.Errorf("no text response from Claude")
}

// formatAPIError provides user-friendly error messages for Anthropic API errors.
func formatAPIError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return fmt.Errorf("invalid API key: check your ANTHROPIC_API_KEY or ~/.detent/config.jsonc")
		case 403:
			return fmt.Errorf("API key lacks permission: %w", err)
		case 429:
			return fmt.Errorf("rate limited: too many requests, try again later")
		case 500, 502, 503:
			return fmt.Errorf("anthropic API unavailable (status %d): try again later", apiErr.StatusCode)
		case 529:
			return fmt.Errorf("anthropic API overloaded: try again later")
		default:
			return fmt.Errorf("API error (status %d): %w", apiErr.StatusCode, err)
		}
	}
	return fmt.Errorf("API request failed: %w", err)
}
