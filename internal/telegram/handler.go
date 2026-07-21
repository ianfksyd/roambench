package telegram

import (
	"fmt"
	"log"
	"strings"

	"github.com/ianf339/roambench/internal/terminal"
)

// ReplyHandler processes Telegram replies and injects them into terminal sessions.
type ReplyHandler struct {
	client    *Client
	mapping   *messageMapping
	terminals TerminalSender
	username  string
}

// TerminalSender is the subset of terminal.Manager used by ReplyHandler.
type TerminalSender interface {
	SendKeys(username, sessionID, text string, withEnter bool) error
}

// NewReplyHandler creates a handler for incoming Telegram commands.
func NewReplyHandler(client *Client, mapping *messageMapping, terminals TerminalSender, username string) *ReplyHandler {
	return &ReplyHandler{
		client:    client,
		mapping:   mapping,
		terminals: terminals,
		username:  username,
	}
}

// HandleUpdate processes a single Telegram update.
func (h *ReplyHandler) HandleUpdate(update Update) {
	if h == nil || h.client == nil || update.Message == nil {
		return
	}
	msg := update.Message
	if msg.Chat.ID != h.client.ChatID() {
		return
	}
	if strings.TrimSpace(msg.Text) == "" {
		return
	}
	if msg.ReplyToMessage == nil {
		_ = h.sendReply(msg.Chat.ID, msg.MessageID, "Reply to a RoamBench notification to send a command to that terminal, or use: /to <session-id> <command>")
		return
	}
	entry, ok := h.mapping.Lookup(msg.ReplyToMessage.MessageID)
	if !ok {
		_ = h.sendReply(msg.Chat.ID, msg.MessageID, "Could not find the terminal for this message. It may have expired; wait for a new notification and reply to that.")
		return
	}
	command := strings.TrimSpace(msg.Text)
	if err := h.terminals.SendKeys(h.username, entry.SessionID, command, true); err != nil {
		log.Printf("telegram command injection error: %v", err)
		_ = h.sendReply(msg.Chat.ID, msg.MessageID, fmt.Sprintf("Failed to inject command into %s: %v", entry.SessionName, err))
		return
	}
	summary := command
	if len(summary) > 120 {
		summary = summary[:120] + "…"
	}
	_ = h.sendReply(msg.Chat.ID, msg.MessageID, fmt.Sprintf("Injected into %s: %s", entry.SessionName, summary))
}

func (h *ReplyHandler) sendReply(chatID int64, replyTo int, text string) error {
	if h.client == nil {
		return nil
	}
	payload := map[string]interface{}{
		"chat_id":             chatID,
		"text":                text,
		"reply_to_message_id": replyTo,
	}
	return h.client.postJSON("sendMessage", payload)
}

// Compile-time check that terminal.Manager implements TerminalSender.
var _ TerminalSender = (*terminal.Manager)(nil)
