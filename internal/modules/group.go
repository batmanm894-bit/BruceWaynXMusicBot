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
		m.Reply("Kisi ko warn karne ke liye uske message par reply karo ya ID/username do.")
		return tg.ErrEndGroup
	}

	count, err := database.AddWarn(chatID, userID)
	if err != nil {
		m.Reply("Warn save karne me error: " + err.Error())
		return tg.ErrEndGroup
	}

	if count >= maxWarnLimit {
		database.ResetWarn(chatID, userID)
		m.Reply(fmt.Sprintf("⚠️ User ko %d warnings mil chuke — limit cross ho gayi. (Auto-kick/ban abhi enabled nahi hai, manually action lo.)", maxWarnLimit))
		return tg.ErrEndGroup
	}

	m.Reply(fmt.Sprintf("⚠️ Warning diya gaya. (%d/%d)", count, maxWarnLimit))
	return tg.ErrEndGroup
}

func warningsHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Kis user ke warnings dekhne hai, uske message par reply karo ya ID/username do.")
		return tg.ErrEndGroup
	}

	count, err := database.GetWarnCount(chatID, userID)
	if err != nil {
		m.Reply("Error: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply(fmt.Sprintf("Is user ke pass %d/%d warnings hai.", count, maxWarnLimit))
	return tg.ErrEndGroup
}

func resetwarnHandler(m *tg.NewMessage) error {
	chatID := m.ChannelID()
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Kis user ke warnings reset karne hai, uske message par reply karo ya ID/username do.")
		return tg.ErrEndGroup
	}

	if err := database.ResetWarn(chatID, userID); err != nil {
		m.Reply("Error: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ Warnings reset kar diye gaye.")
	return tg.ErrEndGroup
}

func pinHandler(m *tg.NewMessage) error {
	reply, err := m.GetReplyMessage()
	if err != nil || reply == nil {
		m.Reply("Jis message ko pin karna hai, uspar reply karo.")
		return tg.ErrEndGroup
	}

	if _, err := m.Client.PinMessage(m.ChannelID(), reply.ID, &tg.PinOptions{Silent: false}); err != nil {
		m.Reply("Pin karne me error: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("📌 Message pin kar diya gaya.")
	return tg.ErrEndGroup
}

func unpinHandler(m *tg.NewMessage) error {
	reply, err := m.GetReplyMessage()
	var msgID int32
	if err == nil && reply != nil {
		msgID = reply.ID
	}

	if _, err := m.Client.UnpinMessage(m.ChannelID(), msgID); err != nil {
		m.Reply("Unpin karne me error: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("📌 Message unpin kar diya gaya.")
	return tg.ErrEndGroup
}

func purgeHandler(m *tg.NewMessage) error {
	reply, err := m.GetReplyMessage()
	if err != nil || reply == nil {
		m.Reply("Jaha se purge start karna hai, us message par reply karke /purge bhejo.")
		return tg.ErrEndGroup
	}

	chatID := m.ChannelID()
	var ids []int32
	for id := reply.ID; id <= m.ID; id++ {
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		m.Reply("Kuch delete karne layak nahi mila.")
		return tg.ErrEndGroup
	}

	if _, err := core.Bot.DeleteMessages(chatID, ids); err != nil {
		m.Reply("Purge karne me error: " + err.Error())
		return tg.ErrEndGroup
	}

	return tg.ErrEndGroup
}
