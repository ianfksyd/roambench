package telegram

// Update represents a single Telegram update.
type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID       int            `json:"message_id"`
	From            *User           `json:"from,omitempty"`
	Chat            Chat           `json:"chat"`
	Date            int            `json:"date"`
	Text            string         `json:"text,omitempty"`
	ReplyToMessage  *Message       `json:"reply_to_message,omitempty"`
}

// User represents a Telegram user.
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username,omitempty"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// sendMessagePayload is the request body for sendMessage.
type sendMessagePayload struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// sendMessageResponse is the response from sendMessage.
type sendMessageResponse struct {
	OK          bool    `json:"ok"`
	Description string  `json:"description,omitempty"`
	Result      Message `json:"result"`
}

// getUpdatesResponse is the response from getUpdates.
type getUpdatesResponse struct {
	OK     bool     `json:"ok"`
	Result []Update `json:"result"`
}

// webhookRequest is the body of a Telegram webhook update.
// Telegram wraps an Update in JSON identical to getUpdates Result entries.
