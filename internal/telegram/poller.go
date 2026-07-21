package telegram

import (
	"context"
	"log"
	"time"
)

// StartPoller begins a long-polling loop that fetches Telegram updates.
// It stops when the provided context is cancelled.
func StartPoller(ctx context.Context, client *Client, handler *ReplyHandler, interval time.Duration, mapping *messageMapping) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	offset := 0
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			mapping.Cleanup()
		case <-ticker.C:
			updates, err := client.GetUpdates(offset)
			if err != nil {
				log.Printf("telegram poll error: %v", err)
				continue
			}
			for _, u := range updates {
				handler.HandleUpdate(u)
				if u.UpdateID >= offset {
					offset = u.UpdateID + 1
				}
			}
		}
	}
}
