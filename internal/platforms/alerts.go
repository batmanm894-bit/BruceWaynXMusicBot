package platforms

import (
	"github.com/Laky-64/gologging"

	"main/internal/config"
	"main/internal/core"
)

// sendAdminAlert notifies both the bot's log chat/channel and the owner's
// DM about something that needs a human's attention (expired cookies, a
// dead API key, ...) - both, since either one on its own might go
// unnoticed for a while. Best-effort: a delivery failure here is only
// logged, never surfaced to whoever's using the bot.
func sendAdminAlert(text string) {
	seen := make(map[int64]bool, 2)
	for _, chatID := range []int64{config.LoggerID, config.OwnerID} {
		if chatID == 0 || seen[chatID] {
			continue
		}
		seen[chatID] = true
		go func(id int64) {
			if _, err := core.Bot.SendMessage(id, text); err != nil {
				gologging.WarnF("Failed to send admin alert to %d: %v", id, err)
			}
		}(chatID)
	}
}

// maskKey returns just enough of an API key to identify it in a message
// without exposing the whole thing (e.g. in a log chat/channel other
// people might be able to read).
func maskKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "..." + key[len(key)-4:]
}
