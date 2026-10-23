package platforms

import (
	"html"

	"main/internal/config"
	"main/internal/core"
)

// sendAdminAlert notifies ONLY the owner's DM (not the log group), at most
// once per hour in total - repeats are merged into one digest message
// (see core.NotifyOwner). Best-effort, never surfaced to bot users.
func sendAdminAlert(text string) {
	if config.OwnerID == 0 {
		return
	}
	key := text
	if len(key) > 200 {
		key = key[:200]
	}
	core.NotifyOwner(key, "<b>Alert</b>\n"+html.EscapeString(text))
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
