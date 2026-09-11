/*
 * ● YukkiMusic
 * ○ A high-performance engine for streaming music in Telegram voicechats.
 *
 * Copyright (C) 2026 TheTeamVivek
 *
 * This program is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software Foundation,
 * either version 3 of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
 * PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * Repository: https://github.com/TheTeamVivek/YukkiMusic
 */

package modules

import (
	"fmt"

	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/database"
	"main/internal/utils"
)

func init() {
	helpTexts["/gban"] = `<i>Globally ban a user — removed and banned from every chat this bot serves, plus blocked from using the bot anywhere.</i>

<u>Usage:</u>
<b>/gban</b> (reply) — Gban the replied user
<b>/gban [user_id]</b> — Gban by ID/username`

	helpTexts["/gmute"] = `<i>Globally mute a user — restricted from sending messages in every chat this bot serves, plus blocked from using playback commands anywhere.</i>

<u>Usage:</u>
<b>/gmute</b> (reply) — Gmute the replied user
<b>/gmute [user_id]</b> — Gmute by ID/username`
}

// gcardHeader mirrors the branded style used for local moderation actions,
// with a global-scope tag.
func gcard(action string, targetID int64, ok, fail int) string {
	target := fmt.Sprintf(`<a href="tg://user?id=%d">User</a>`, targetID)
	return fmt.Sprintf(
		"%s\n\n▸ <b>%s</b>\n\n<pre>┌ User      %s\n├ Applied   %d chats\n└ Failed    %d chats</pre>",
		modHeader, action, target, ok, fail,
	)
}

// applyToAllChats runs fn against every chat the bot currently serves and
// returns how many succeeded vs failed. Errors are swallowed per-chat
// (e.g. bot no longer in that chat, or lacks rights there) so one bad
// chat doesn't stop the rest.
func applyToAllChats(c *tg.Client, fn func(chatID int64) error) (ok, fail int) {
	chats, err := database.ServedChats()
	if err != nil {
		return 0, 0
	}
	for _, chatID := range chats {
		if err := fn(chatID); err != nil {
			fail++
			continue
		}
		ok++
	}
	return ok, fail
}

func gbanHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if already, _ := database.IsGbanned(userID); already {
		m.Reply(modError("Gban", "this user is already globally banned."))
		return tg.ErrEndGroup
	}

	if err := database.AddGban(userID); err != nil {
		m.Reply(modError("Gban", "failed to save: "+err.Error()))
		return tg.ErrEndGroup
	}

	ok, fail := applyToAllChats(m.Client, func(chatID int64) error {
		_, err := m.Client.EditBannedBuilder(chatID, userID).Ban(0)
		return err
	})

	m.Reply(gcard("🚫 User Globally Banned", userID, ok, fail))
	return tg.ErrEndGroup
}

func ungbanHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	wasGbanned, err := database.IsGbanned(userID)
	if err != nil {
		m.Reply(modError("Ungban", err.Error()))
		return tg.ErrEndGroup
	}
	if !wasGbanned {
		m.Reply(modError("Ungban", "this user is not globally banned."))
		return tg.ErrEndGroup
	}

	if err := database.RemoveGban(userID); err != nil {
		m.Reply(modError("Ungban", "failed to save: "+err.Error()))
		return tg.ErrEndGroup
	}

	ok, fail := applyToAllChats(m.Client, func(chatID int64) error {
		_, err := m.Client.EditBannedBuilder(chatID, userID).Unban()
		return err
	})

	m.Reply(gcard("✅ Global Ban Removed", userID, ok, fail))
	return tg.ErrEndGroup
}

func gmuteHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if already, _ := database.IsGmuted(userID); already {
		m.Reply(modError("Gmute", "this user is already globally muted."))
		return tg.ErrEndGroup
	}

	if err := database.AddGmute(userID); err != nil {
		m.Reply(modError("Gmute", "failed to save: "+err.Error()))
		return tg.ErrEndGroup
	}

	ok, fail := applyToAllChats(m.Client, func(chatID int64) error {
		_, err := m.Client.EditBannedBuilder(chatID, userID).Mute()
		return err
	})

	m.Reply(gcard("🔇 User Globally Muted", userID, ok, fail))
	return tg.ErrEndGroup
}

func ungmuteHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	wasGmuted, err := database.IsGmuted(userID)
	if err != nil {
		m.Reply(modError("Ungmute", err.Error()))
		return tg.ErrEndGroup
	}
	if !wasGmuted {
		m.Reply(modError("Ungmute", "this user is not globally muted."))
		return tg.ErrEndGroup
	}

	if err := database.RemoveGmute(userID); err != nil {
		m.Reply(modError("Ungmute", "failed to save: "+err.Error()))
		return tg.ErrEndGroup
	}

	ok, fail := applyToAllChats(m.Client, func(chatID int64) error {
		_, err := m.Client.EditBannedBuilder(chatID, userID).Unmute()
		return err
	})

	m.Reply(gcard("🔊 Global Mute Removed", userID, ok, fail))
	return tg.ErrEndGroup
}
