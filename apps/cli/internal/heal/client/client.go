package client

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Client wraps the Anthropic SDK for healing operations.
type Client struct {
	api anthropic.Client
}

// New creates a new Anthropic client.
// API key is resolved in order: ANTHROPIC_API_KEY env var > config file.
func New(configAPIKey string) (*Client, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		key = configAPIKey
	}
	if key == "" {
		return nil, fmt.Errorf("no API key: set ANTHROPIC_API_KEY env var or add anthropic_api_key to ~/.detent/config.yaml")
	}

	return &Client{
		api: anthropic.NewClient(option.WithAPIKey(key)),
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
		return "", fmt.Errorf("API request failed: %w", err)
	}

	for i := range msg.Content {
		if text, ok := msg.Content[i].AsAny().(anthropic.TextBlock); ok {
			return text.Text, nil
		}
	}
	return "", fmt.Errorf("no text response from Claude")
}
