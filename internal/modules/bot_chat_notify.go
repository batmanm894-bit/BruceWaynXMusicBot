package modules

import (
	"fmt"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/utils"
)

// Post to the group log (LOGGER_ID) right away when the bot is added to / removed from a group.
// Added: includes the group's member count. Removed: the bot is already out,
// so Telegram will not give the count any more.

func chatMemberCount(c *telegram.Client, chatID int64) string {
	defer func() { _ = recover() }()
	_, total, err := c.GetChatMembers(chatID, &telegram.ParticipantOptions{
		Limit:            1,
		SleepThresholdMs: 3000,
	})
	if err != nil {
		gologging.DebugF("member count failed for %d: %v", chatID, err)
		return "N/A"
	}
	return fmt.Sprintf("%d", total)
}

func ownerChatBlock(m *telegram.NewMessage, chatID int64, who string) string {
	title, uname := "Unknown", "N/A"
	if m.Channel != nil {
		if m.Channel.Title != "" {
			title = utils.EscapeHTML(m.Channel.Title)
		}
		if m.Channel.Username != "" {
			uname = "@" + m.Channel.Username
		}
	}
	by := "Unknown"
	var byID int64
	if m.Sender != nil {
		by = utils.MentionHTML(m.Sender)
		byID = m.SenderID()
	}
	return fmt.Sprintf(
		"<blockquote><b>Group:</b> %s\n<b>ID:</b> <code>%d</code>\n<b>Username:</b> %s\n\n<b>%s:</b> %s (<code>%d</code>)\n<b>Time:</b> %s</blockquote>",
		title, chatID, uname, who, by, byID, time.Now().Format("02 Jan 2006 • 15:04"),
	)
}

func notifyLoggerBotAdded(m *telegram.NewMessage, chatID int64) {
	if config.LoggerID == 0 || !isLoggerEnabled() {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		text := "✦ <b>Bot added to a group</b>\n\n" +
			ownerChatBlock(m, chatID, "Added by") +
			"\n\n👥 <b>Members:</b> " + chatMemberCount(m.Client, chatID)
		if _, err := m.Client.SendMessage(config.LoggerID, text, &telegram.SendOptions{LinkPreview: false}); err != nil {
			gologging.WarnF("logger msg (bot added) failed: %v", err)
		}
	}()
}

func notifyLoggerBotRemoved(m *telegram.NewMessage, chatID int64) {
	if config.LoggerID == 0 || !isLoggerEnabled() {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		text := "✦ <b>Bot removed from a group</b>\n\n" + ownerChatBlock(m, chatID, "Removed by")
		if _, err := m.Client.SendMessage(config.LoggerID, text, &telegram.SendOptions{LinkPreview: false}); err != nil {
			gologging.WarnF("logger msg (bot removed) failed: %v", err)
		}
	}()
}
