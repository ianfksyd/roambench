package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const telegramAPIBase = "https://api.telegram.org/bot"

// Client wraps the Telegram Bot API for RoamBench notifications.
type Client struct {
	botToken   string
	chatID     int64
	httpClient *http.Client
}

// NewClient creates a Telegram client.
func NewClient(botToken, chatID string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(botToken) == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil, fmt.Errorf("telegram chat id is required")
	}
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("telegram chat id must be numeric: %w", err)
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		botToken:   botToken,
		chatID:     id,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// ChatID returns the default configured chat ID.
func (c *Client) ChatID() int64 {
	return c.chatID
}

// SendMessage sends a text message to the configured chat and returns the Telegram message ID.
func (c *Client) SendMessage(text string) (int, error) {
	return c.SendMessageToChat(c.chatID, text)
}

// SendMessageToChat sends a text message to a specific chat ID.
func (c *Client) SendMessageToChat(chatID int64, text string) (int, error) {
	payload := sendMessagePayload{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "",
	}
	var resp sendMessageResponse
	if err := c.post("sendMessage", payload, &resp); err != nil {
		return 0, err
	}
	if !resp.OK {
		return 0, fmt.Errorf("telegram sendMessage failed: %s", resp.Description)
	}
	return resp.Result.MessageID, nil
}

// GetUpdates fetches pending updates with an offset.
func (c *Client) GetUpdates(offset int) ([]Update, error) {
	params := url.Values{}
	params.Set("offset", strconv.Itoa(offset))
	params.Set("limit", "100")
	var resp getUpdatesResponse
	if err := c.get("getUpdates?"+params.Encode(), &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram getUpdates failed")
	}
	return resp.Result, nil
}

// SetWebhook registers the webhook URL and optional secret with Telegram.
func (c *Client) SetWebhook(webhookURL, secret string) error {
	payload := map[string]string{
		"url": webhookURL,
	}
	if strings.TrimSpace(secret) != "" {
		payload["secret_token"] = secret
	}
	return c.postJSON("setWebhook", payload)
}

// DeleteWebhook removes any registered webhook.
func (c *Client) DeleteWebhook() error {
	return c.postJSON("deleteWebhook", map[string]string{})
}

func (c *Client) apiURL(method string) string {
	return telegramAPIBase + c.botToken + "/" + method
}

func (c *Client) get(method string, dest interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.apiURL(method), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp.Body, dest)
}

func (c *Client) post(method string, payload, dest interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.apiURL(method), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeJSON(resp.Body, dest)
}

func (c *Client) postJSON(method string, payload interface{}) error {
	return c.post(method, payload, &struct {
		OK          bool   `json:"ok"`
		Description string `json:"description,omitempty"`
	}{})
}

func decodeJSON(r io.Reader, dest interface{}) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode telegram response: %w", err)
	}
	return nil
}
