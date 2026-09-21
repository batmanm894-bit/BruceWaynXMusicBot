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

package utils

import (
	"fmt"
	"unicode/utf16"

	"github.com/amarnathcjd/gogram/telegram"
)

// ExtractOwnURLs returns URLs found only in the message itself, ignoring
// whatever message it replies to. Callers use this so that text typed next
// to a command (e.g. "/play song name") isn't overridden by a link that
// happens to sit in the message being replied to (like the bot's own
// "now playing" message, which carries the current track's URL).
func ExtractOwnURLs(m *telegram.NewMessage) []string {
	if m == nil || m.Message == nil {
		return nil
	}
	return collectURLs(m.Message)
}

func ExtractURLs(m *telegram.NewMessage) ([]string, error) {
	if m == nil || m.Message == nil {
		return nil, fmt.Errorf("invalid message")
	}

	urls := make([]string, 0, estimateCapacity(m))
	urls = append(urls, collectURLs(m.Message)...)

	if !m.IsReply() {
		return finalizeURLs(urls)
	}

	r, err := m.GetReplyMessage()
	if err != nil {
		if len(urls) > 0 {
			return urls, fmt.Errorf("failed to fetch reply message: %w", err)
		}
		return nil, fmt.Errorf("failed to fetch reply message: %w", err)
	}

	urls = append(urls, collectURLs(r.Message)...)
	return finalizeURLs(urls)
}

// --- Sub Functions ---

func estimateCapacity(m *telegram.NewMessage) int {
	capacity := len(m.Message.Entities)
	if m.IsReply() {
		if r, err := m.GetReplyMessage(); err == nil && r.Message != nil {
			capacity += len(r.Message.Entities)
		}
	}
	return capacity
}

func collectURLs(msg *telegram.MessageObj) []string {
	if msg == nil {
		return nil
	}

	// Telegram entity offsets/lengths are counted in UTF-16 code units,
	// not bytes. Slicing the Go string directly returns garbage (or
	// silently drops the URL) as soon as the text contains Hindi, emoji or
	// any other non-ASCII character before the link.
	units := utf16.Encode([]rune(msg.Message))
	urls := make([]string, 0, len(msg.Entities))

	for _, ent := range msg.Entities {
		switch e := ent.(type) {
		case *telegram.MessageEntityURL:
			start, end := int(e.Offset), int(e.Offset+e.Length)
			if start >= 0 && start < end && end <= len(units) {
				urls = append(urls, string(utf16.Decode(units[start:end])))
			}
		case *telegram.MessageEntityTextURL:
			if e.URL != "" {
				urls = append(urls, e.URL)
			}
		}
	}
	return urls
}

func finalizeURLs(urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no URLs found")
	}
	return urls, nil
}
