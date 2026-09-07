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

	helpTexts["/promote"] = `<i>Promote a user to admin in this chat.</i>`
	helpTexts["/demote"] = `<i>Demote an admin back to a normal member.</i>`
}

func banHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditBannedBuilder(chatID, userID).Ban(0); err != nil {
		m.Reply("Failed to ban: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("🚫 User has been banned from this chat.")
	return tg.ErrEndGroup
}

func unbanHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditBannedBuilder(chatID, userID).Unban(); err != nil {
		m.Reply("Failed to unban: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ User has been unbanned.")
	return tg.ErrEndGroup
}

func kickHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.KickParticipant(chatID, userID); err != nil {
		m.Reply("Failed to kick: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("👢 User has been kicked from this chat (they can rejoin via invite link).")
	return tg.ErrEndGroup
}

func promoteHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditAdminBuilder(chatID, userID).Promote(); err != nil {
		m.Reply("Failed to promote: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("⬆️ User has been promoted to admin.")
	return tg.ErrEndGroup
}

func demoteHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.EditAdminBuilder(chatID, userID).Demote(); err != nil {
		m.Reply("Failed to demote: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("⬇️ User has been demoted.")
	return tg.ErrEndGroup
}
