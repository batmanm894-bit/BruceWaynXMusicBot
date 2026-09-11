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

	"main/internal/utils"
)

func init() {
	helpTexts["/ban"] = `<i>Ban a user from this chat.</i>

<u>Usage:</u>
<b>/ban</b> (reply) — Ban the replied user
<b>/ban [user_id]</b> — Ban by ID/username`

	helpTexts["/unban"] = `<i>Unban a previously banned user in this chat.</i>`

	helpTexts["/kick"] = `<i>Kick a user from this chat (they can rejoin via invite link).</i>`

	helpTexts["/mute"] = `<i>Mute a user in this chat (they can no longer send messages).</i>

<u>Usage:</u>
<b>/mute</b> (reply) — Mute the replied user
<b>/mute [user_id]</b> — Mute by ID/username`

	helpTexts["/unmute"] = `<i>Unmute a previously muted user in this chat.</i>

<u>Usage:</u>
<b>/unmute</b> (reply) — Unmute the replied user
<b>/unmute [user_id]</b> — Unmute by ID/username`

	helpTexts["/promote"] = `<i>Promote a user to admin in this chat.</i>`
	helpTexts["/demote"] = `<i>Demote an admin back to a normal member.</i>`
}

// --- Premium-styled reply template (matches the now-playing card style) ---

const modHeader = "🦇 —— <b>Bruce Music</b> —— 🦇"

// modCard builds a bordered, branded reply for a moderation action performed
// on a single user in the current chat.
func modCard(m *tg.NewMessage, action string, targetID int64) string {
	target := fmt.Sprintf(`<a href="tg://user?id=%d">User</a>`, targetID)
	admin := utils.MentionHTML(m.Sender)

	chat := "this chat"
	if m.Channel != nil && m.Channel.Title != "" {
		chat = utils.EscapeHTML(m.Channel.Title)
	}

	return fmt.Sprintf(
		"%s\n\n▸ <b>%s</b>\n\n<pre>┌ User      %s\n├ Chat      %s\n└ By        %s</pre>",
		modHeader, action, target, chat, admin,
	)
}

// modError builds a bordered, branded error reply.
func modError(action, reason string) string {
	return fmt.Sprintf(
		"%s\n\n▸ <b>%s Failed</b>\n\n<pre>└ Reason    %s</pre>",
		modHeader, action, reason,
	)
}

const noValidUserMsg = "Please specify a valid user (reply to their message or give an ID/username)."

func banHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditBannedBuilder(chatID, userID).Ban(0); err != nil {
		m.Reply(modError("Ban", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "🚫 User Banned", userID))
	return tg.ErrEndGroup
}

func unbanHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditBannedBuilder(chatID, userID).Unban(); err != nil {
		m.Reply(modError("Unban", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "✅ User Unbanned", userID))
	return tg.ErrEndGroup
}

func kickHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.KickParticipant(chatID, userID); err != nil {
		m.Reply(modError("Kick", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "👢 User Kicked", userID))
	return tg.ErrEndGroup
}

// muteHandler restricts a user from sending messages in the current chat.
// This is separate from /cmute (which mutes the voice-chat audio stream).
func muteHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditBannedBuilder(chatID, userID).Mute(); err != nil {
		m.Reply(modError("Mute", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "🔇 User Muted", userID))
	return tg.ErrEndGroup
}

// unmuteHandler lifts a previous /mute restriction for a user in this chat.
// This is separate from /cunmute (which unmutes the voice-chat audio stream).
func unmuteHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditBannedBuilder(chatID, userID).Unmute(); err != nil {
		m.Reply(modError("Unmute", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "🔊 User Unmuted", userID))
	return tg.ErrEndGroup
}

func promoteHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditAdminBuilder(chatID, userID).Promote(); err != nil {
		m.Reply(modError("Promote", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "⬆️ User Promoted", userID))
	return tg.ErrEndGroup
}

func demoteHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply(noValidUserMsg)
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditAdminBuilder(chatID, userID).Demote(); err != nil {
		m.Reply(modError("Demote", err.Error()))
		return tg.ErrEndGroup
	}

	m.Reply(modCard(m, "⬇️ User Demoted", userID))
	return tg.ErrEndGroup
}
