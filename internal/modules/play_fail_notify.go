package modules

import (
	"fmt"
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	state "main/internal/core/models"
	"main/internal/utils"
)

// Post to the group log (LOGGER_ID) when a song fails to download or play,
// with the reason, so failures are visible without digging through server logs.

var playFailLast sync.Map // chatID -> time.Time (1 alert per chat per 30s)

func notifyLoggerPlayFail(
	client *tg.Client,
	chatID int64,
	requesterHTML string,
	requesterID int64,
	track *state.Track,
	stage string,
	cause error,
) {
	if config.LoggerID == 0 || !isLoggerEnabled() || client == nil || cause == nil {
		return
	}
	if v, ok := playFailLast.Load(chatID); ok {
		if t, _ := v.(time.Time); time.Since(t) < 30*time.Second {
			return
		}
	}
	playFailLast.Store(chatID, time.Now())

	go func() {
		defer func() { _ = recover() }()

		title, source, link := "Unknown", "N/A", ""
		if track != nil {
			title = utils.EscapeHTML(utils.ShortTitle(track.Title, 60))
			source = utils.EscapeHTML(string(track.Source))
			link = track.URL
		}
		reason := cause.Error()
		if len(reason) > 400 {
			reason = reason[:400] + "..."
		}

		by := "Unknown"
		if requesterHTML != "" {
			by = fmt.Sprintf("%s (<code>%d</code>)", requesterHTML, requesterID)
		}
		trackLine := title
		if link != "" {
			trackLine = fmt.Sprintf(`<a href="%s">%s</a>`, link, title)
		}

		text := fmt.Sprintf(
			"✦ <b>Playback failed</b> (%s)\n\n"+
				"<blockquote><b>Track:</b> %s\n<b>Source:</b> %s\n<b>Group:</b> %s (<code>%d</code>)\n<b>Requested by:</b> %s</blockquote>\n\n"+
				"<b>Reason:</b>\n<pre>%s</pre>",
			stage, trackLine, source, verifyChatTitle(chatID), chatID, by, utils.EscapeHTML(reason),
		)
		if _, err := client.SendMessage(config.LoggerID, text, &tg.SendOptions{LinkPreview: false}); err != nil {
			gologging.WarnF("logger msg (play failed) failed: %v", err)
		}
	}()
}
