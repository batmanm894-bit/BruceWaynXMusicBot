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

	"main/internal/core"
	"main/internal/database"
	"main/internal/utils"
)

const maxWarnLimit = 3

func init() {
	helpTexts["/warn"] = `<i>Warn a user in this chat.</i>

<u>Usage:</u>
<b>/warn</b> (reply) — Warn the replied user`

	helpTexts["/warnings"] = `<i>Show a user's warning count in this chat.</i>`
	helpTexts["/resetwarn"] = `<i>Reset a user's warnings in this chat.</i>`
	helpTexts["/pin"] = `<i>Pin the replied message.</i>`
	helpTexts["/unpin"] = `<i>Unpin the replied (or last pinned) message.</i>`
	helpTexts["/purge"] = `<i>Delete all messages between the replied message and this one.</i>`
}

func warnHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Reply to the user's message or give an ID/username to warn them.")
		return tg.ErrEndGroup
	}

	count, err := database.AddWarn(chatID, userID)
	if err != nil {
		m.Reply("Failed to save warn: " + err.Error())
		return tg.ErrEndGroup
	}

	if count >= maxWarnLimit {
		database.ResetWarn(chatID, userID)
		m.Reply(fmt.Sprintf("⚠️ User has reached %d warnings — limit exceeded. (Auto-kick/ban is not enabled yet, take manual action.)", maxWarnLimit))
		return tg.ErrEndGroup
	}

	m.Reply(fmt.Sprintf("⚠️ Warning issued. (%d/%d)", count, maxWarnLimit))
	return tg.ErrEndGroup
}

func warningsHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Reply to the user's message or give an ID/username to check their warnings.")
		return tg.ErrEndGroup
	}

	count, err := database.GetWarnCount(chatID, userID)
	if err != nil {
		m.Reply("Error: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply(fmt.Sprintf("This user has %d/%d warnings.", count, maxWarnLimit))
	return tg.ErrEndGroup
}

func resetwarnHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Reply to the user's message or give an ID/username to reset their warnings.")
		return tg.ErrEndGroup
	}

	if err := database.ResetWarn(chatID, userID); err != nil {
		m.Reply("Error: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ Warnings have been reset.")
	return tg.ErrEndGroup
}

func pinHandler(m *tg.NewMessage) error {
	reply, err := m.GetReplyMessage()
	if err != nil || reply == nil {
		m.Reply("Reply to the message you want to pin.")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.PinMessage(m.ChannelID(), reply.ID, &tg.PinOptions{Silent: false}); err != nil {
		m.Reply("Failed to pin: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("📌 Message has been pinned.")
	return tg.ErrEndGroup
}

func unpinHandler(m *tg.NewMessage) error {
	reply, err := m.GetReplyMessage()
	var msgID int32
	if err == nil && reply != nil {
		msgID = reply.ID
	}

	if _, err := m.Client.UnpinMessage(m.ChannelID(), msgID); err != nil {
		m.Reply("Failed to unpin: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("📌 Message has been unpinned.")
	return tg.ErrEndGroup
}

func purgeHandler(m *tg.NewMessage) error {
	reply, err := m.GetReplyMessage()
	if err != nil || reply == nil {
		m.Reply("Reply to the message you want to start purging from, then send /purge.")
		return tg.ErrEndGroup
	}

	chatID := m.ChannelID()
	var ids []int32
	for id := reply.ID; id <= m.ID; id++ {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		m.Reply("Nothing found to delete.")
		return tg.ErrEndGroup
	}

	if _, err := core.Bot.DeleteMessages(chatID, ids); err != nil {
		m.Reply("Failed to purge: " + err.Error())
		return tg.ErrEndGroup
	}

	return tg.ErrEndGroup
}
