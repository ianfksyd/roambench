package telegram

import (
	"fmt"
	"log"
	"strings"

	"github.com/ianf339/roambench/internal/terminal"
)

// Dispatcher forwards terminal OSC notifications to Telegram.
type Dispatcher struct {
	client  *Client
	mapping *messageMapping
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(client *Client, mapping *messageMapping) *Dispatcher {
	return &Dispatcher{
		client:  client,
		mapping: mapping,
	}
}

// Dispatch formats and sends a terminal notification to Telegram.
func (d *Dispatcher) Dispatch(n terminal.OSCNotification, sessions []terminal.SessionInfo) {
	if d == nil || d.client == nil {
		return
	}
	name := sessionNameByID(n.SessionID, sessions)
	text := formatNotification(name, n)
	msgID, err := d.client.SendMessage(text)
	if err != nil {
		log.Printf("telegram dispatch error: %v", err)
		return
	}
	d.mapping.Store(msgID, d.client.ChatID(), n.SessionID, name)
}

func formatNotification(sessionName string, n terminal.OSCNotification) string {
	var b strings.Builder
	if sessionName != "" {
		fmt.Fprintf(&b, "🖥️ *%s*\n", escapeMarkdown(sessionName))
	}
	if strings.TrimSpace(n.Title) != "" {
		fmt.Fprintf(&b, "*%s*\n", escapeMarkdown(strings.TrimSpace(n.Title)))
	}
	body := strings.TrimSpace(n.Body)
	if body == "" && strings.TrimSpace(n.Title) == "" {
		body = "Terminal notification"
	}
	if body != "" {
		b.WriteString(escapeMarkdown(body))
	}
	b.WriteString("\n\n_Reply to this message to send a command back to the terminal._")
	return b.String()
}

func sessionNameByID(sessionID string, sessions []terminal.SessionInfo) string {
	for _, s := range sessions {
		if s.ID == sessionID {
			return s.Name
		}
	}
	return sessionID
}

func escapeMarkdown(s string) string {
	// Telegram pre-formatted (MarkdownV2) escaping is strict; we use plain text
	// with Unicode bold via characters instead of parse mode to avoid escaping complexity.
	return s
}
