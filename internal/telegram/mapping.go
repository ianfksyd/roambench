package telegram

import (
	"sync"
	"time"
)

// mappingEntry maps a Telegram message to a RoamBench terminal session.
type mappingEntry struct {
	SessionID   string
	SessionName string
	ChatID      int64
	SentAt      time.Time
}

// messageMapping stores Telegram message_id -> terminal session associations with TTL cleanup.
type messageMapping struct {
	mu      sync.RWMutex
	entries map[int]mappingEntry
	ttl     time.Duration
}

// NewMessageMapping creates a mapping store. ttl <= 0 defaults to 24 hours.
func NewMessageMapping(ttl time.Duration) *messageMapping {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &messageMapping{
		entries: make(map[int]mappingEntry),
		ttl:     ttl,
	}
}

// Store records a mapping from Telegram message ID to RoamBench session.
func (m *messageMapping) Store(telegramMsgID int, chatID int64, sessionID, sessionName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[telegramMsgID] = mappingEntry{
		SessionID:   sessionID,
		SessionName: sessionName,
		ChatID:      chatID,
		SentAt:      time.Now(),
	}
}

// Lookup returns the mapped entry and whether it exists and is still valid.
func (m *messageMapping) Lookup(telegramMsgID int) (mappingEntry, bool) {
	m.mu.RLock()
	entry, ok := m.entries[telegramMsgID]
	m.mu.RUnlock()
	if !ok {
		return mappingEntry{}, false
	}
	if time.Since(entry.SentAt) > m.ttl {
		m.mu.Lock()
		delete(m.entries, telegramMsgID)
		m.mu.Unlock()
		return mappingEntry{}, false
	}
	return entry, true
}

// Cleanup removes expired entries. It is safe to call concurrently.
func (m *messageMapping) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, entry := range m.entries {
		if now.Sub(entry.SentAt) > m.ttl {
			delete(m.entries, id)
		}
	}
}
