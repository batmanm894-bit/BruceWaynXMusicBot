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

	"main/internal/database"
	"main/internal/utils"
)

func init() {
	helpTexts["/gban"] = `<i>Globally ban a user from using the bot in any chat.</i>

<u>Usage:</u>
<b>/gban</b> (reply) — Gban the replied user
<b>/gban [user_id]</b> — Gban by ID/username`

	helpTexts["/gmute"] = `<i>Globally mute a user from using playback commands in any chat.</i>

<u>Usage:</u>
<b>/gmute</b> (reply) — Gmute the replied user
<b>/gmute [user_id]</b> — Gmute by ID/username`
}

func gbanHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if already, _ := database.IsGbanned(userID); already {
		m.Reply("This user is already globally banned.")
		return tg.ErrEndGroup
	}

	if err := database.AddGban(userID); err != nil {
		m.Reply("Failed to save gban: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ User has been globally banned — they can no longer use the bot in any chat.")
	return tg.ErrEndGroup
}

func ungbanHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	removed, err := database.IsGbanned(userID)
	if err != nil {
		m.Reply("Error: " + err.Error())
		return tg.ErrEndGroup
	}
	if !removed {
		m.Reply("This user is not globally banned.")
		return tg.ErrEndGroup
	}

	if err := database.RemoveGban(userID); err != nil {
		m.Reply("Failed to remove gban: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ Global ban has been removed.")
	return tg.ErrEndGroup
}

func gmuteHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	if already, _ := database.IsGmuted(userID); already {
		m.Reply("This user is already globally muted.")
		return tg.ErrEndGroup
	}

	if err := database.AddGmute(userID); err != nil {
		m.Reply("Failed to save gmute: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ User has been globally muted — they can no longer use playback commands in any chat.")
	return tg.ErrEndGroup
}

func ungmuteHandler(m *tg.NewMessage) error {
	userID, err := utils.ExtractUser(m)
	if err != nil {
		m.Reply("Please specify a valid user (reply to their message or give an ID/username).")
		return tg.ErrEndGroup
	}

	removed, err := database.IsGmuted(userID)
	if err != nil {
		m.Reply("Error: " + err.Error())
		return tg.ErrEndGroup
	}
	if !removed {
		m.Reply("This user is not globally muted.")
		return tg.ErrEndGroup
	}

	if err := database.RemoveGmute(userID); err != nil {
		m.Reply("Failed to remove gmute: " + err.Error())
		return tg.ErrEndGroup
	}

	m.Reply("✅ Global mute has been removed.")
	return tg.ErrEndGroup
}

