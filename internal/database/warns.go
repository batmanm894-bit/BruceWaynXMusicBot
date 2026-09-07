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

func GetWarnCount(chatID, userID int64) (int, error) {
	settings, err := GetChatSettings(chatID)
	if err != nil {
		return 0, err
	}
	if settings.Warns == nil {
		return 0, nil
	}
	return settings.Warns[userID], nil
}

func AddWarn(chatID, userID int64) (int, error) {
	var newCount int
	err := modifyChatSettings(chatID, func(s *ChatSettings) bool {
		if s.Warns == nil {
			s.Warns = map[int64]int{}
		}
		s.Warns[userID]++
		newCount = s.Warns[userID]
		return true
	})
	return newCount, err
}

func ResetWarn(chatID, userID int64) error {
	return modifyChatSettings(chatID, func(s *ChatSettings) bool {
		if s.Warns == nil {
			return false
		}
		if _, ok := s.Warns[userID]; !ok {
			return false
		}
		delete(s.Warns, userID)
		return true
	})
}

