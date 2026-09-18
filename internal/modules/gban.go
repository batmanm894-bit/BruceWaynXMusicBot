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
	"strconv"
	"strings"

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

	helpTexts["/gbanlist"] = `<i>Shows the total count and user IDs of everyone currently globally banned.</i>

<u>Usage:</u>
<b>/gbanlist</b>`

	helpTexts["/gmutelist"] = `<i>Shows the total count and user IDs of everyone currently globally muted.</i>

<u>Usage:</u>
<b>/gmutelist</b>`
}

// glistCard renders a branded count + user list (name, username, ID) for
// /gbanlist and /gmutelist. Telegram messages cap out around 4096 chars, so
// long lists are chunked and only the first chunk carries the header/count;
// the rest are sent as plain follow-up messages.
func glistCard(m *tg.NewMessage, title string, ids []int64) error {
	if len(ids) == 0 {
		_, err := m.Reply(fmt.Sprintf(
			"%s\n\n▸ <b>%s</b>\n\n<pre>Total   0</pre>",
			modHeader, title,
		))
		return err
	}

	lines := make([]string, len(ids))
	for i, id := range ids {
		idStr := strconv.FormatInt(id, 10)
		entry := utils.MentionHTML(nil) + " — <code>" + idStr + "</code>"

		if user, err := m.Client.GetUser(id); err == nil && user != nil {
			name := utils.MentionHTML(user)
			if user.Username != "" {
				entry = fmt.Sprintf("%s (@%s) — <code>%s</code>", name, user.Username, idStr)
			} else {
				entry = fmt.Sprintf("%s — <code>%s</code>", name, idStr)
			}
		}

		lines[i] = fmt.Sprintf("%d. %s", i+1, entry)
	}

	const chunkSize = 40
	first := true
	for start := 0; start < len(lines); start += chunkSize {
		end := min(start+chunkSize, len(lines))
		body := strings.Join(lines[start:end], "\n")

		var text string
		if first {
			text = fmt.Sprintf(
				"%s\n\n▸ <b>%s</b>\n\n<pre>Total   %d</pre>\n\n%s",
				modHeader, title, len(ids), body,
			)
			first = false
		} else {
			text = body
		}

		if _, err := m.Reply(text); err != nil {
			return err
		}
	}
	return nil
}

func gbanlistHandler(m *tg.NewMessage) error {
	ids, err := database.GbannedUsers()
	if err != nil {
		m.Reply(modError("Gbanlist", "failed to fetch: "+err.Error()))
		return tg.ErrEndGroup
	}
	glistCard(m, "🚫 Globally Banned Users", ids)
	return tg.ErrEndGroup
}

func gmutelistHandler(m *tg.NewMessage) error {
	ids, err := database.GmutedUsers()
	if err != nil {
		m.Reply(modError("Gmutelist", "failed to fetch: "+err.Error()))
		return tg.ErrEndGroup
	}
	glistCard(m, "🔇 Globally Muted Users", ids)
	return tg.ErrEndGroup
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
