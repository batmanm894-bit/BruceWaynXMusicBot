package core

import (
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
)

// NotifyLogger posts an HTML notice to the group log (LOGGER_ID).
// The same key is sent at most once per 10 minutes so a repeating error
// cannot flood the log group. Never blocks the caller.
const loggerNotifyCooldown = 10 * time.Minute

var loggerLast sync.Map // key -> time.Time

func NotifyLogger(key, htmlText string) {
	if config.LoggerID == 0 || Bot == nil {
		return
	}
	if v, ok := loggerLast.Load(key); ok {
		if t, _ := v.(time.Time); time.Since(t) < loggerNotifyCooldown {
			return
		}
	}
	loggerLast.Store(key, time.Now())

	go func() {
		defer func() { _ = recover() }()
		if _, err := Bot.SendMessage(config.LoggerID, htmlText, &telegram.SendOptions{ParseMode: "HTML", LinkPreview: false}); err != nil {
			gologging.WarnF("logger notify failed: %v", err)
		}
	}()
}
