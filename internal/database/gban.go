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

package database

func GbannedUsers() ([]int64, error) {
	state, err := getBotState()
	if err != nil {
		return nil, err
	}
	return append([]int64(nil), state.Gbanned...), nil
}

func IsGbanned(userID int64) (bool, error) {
	state, err := getBotState()
	if err != nil {
		return false, err
	}
	return contains(state.Gbanned, userID), nil
}

func AddGban(userID int64) error {
	return modifyBotState(func(s *BotState) bool {
		var added bool
		s.Gbanned, added = addUnique(s.Gbanned, userID)
		return added
	})
}

func RemoveGban(userID int64) error {
	return modifyBotState(func(s *BotState) bool {
		var removed bool
		s.Gbanned, removed = removeElement(s.Gbanned, userID)
		return removed
	})
}

func GmutedUsers() ([]int64, error) {
	state, err := getBotState()
	if err != nil {
		return nil, err
	}
	return append([]int64(nil), state.Gmuted...), nil
}

func IsGmuted(userID int64) (bool, error) {
	state, err := getBotState()
	if err != nil {
		return false, err
	}
	return contains(state.Gmuted, userID), nil
}

func AddGmute(userID int64) error {
	return modifyBotState(func(s *BotState) bool {
		var added bool
		s.Gmuted, added = addUnique(s.Gmuted, userID)
		return added
	})
}

func RemoveGmute(userID int64) error {
	return modifyBotState(func(s *BotState) bool {
		var removed bool
		s.Gmuted, removed = removeElement(s.Gmuted, userID)
		return removed
	})
}
