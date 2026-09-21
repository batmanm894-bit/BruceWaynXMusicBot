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
	"strings"

	tg "github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
	state "main/internal/core/models"
	"main/internal/database"
	"main/internal/locales"
	"main/internal/platforms"
	"main/internal/utils"
)

func init() {
	helpTexts["/autoplay"] = `<i>Automatically keep playing similar tracks when the queue ends.</i>

<u>Usage:</u>
<b>/autoplay</b> — Shows current autoplay status (enabled/disabled).
<b>/autoplay enable</b> — Enable autoplay.
<b>/autoplay disable</b> — Disable autoplay.

<b>🧠 Details:</b>
When enabled, if the queue finishes with nothing left to play, the bot
searches for a track similar to the last one played (based on its title)
and keeps the music going automatically.

<b>⚠️ Restrictions:</b>
This command can only be used by chat admins.`
}

func autoplayHandler(m *tg.NewMessage) error {
	args := strings.Fields(m.Text())
	chatID := m.ChannelID()

	current, err := database.Autoplay(chatID)
	if err != nil {
		return err
	}

	if len(args) < 2 {
		m.Reply(F(chatID, "autoplay_status", locales.Arg{
			"action": F(chatID, utils.IfElse(current, "enabled", "disabled")),
		}))
		return tg.ErrEndGroup
	}

	enabled, err := utils.ParseBool(args[1])
	if err != nil {
		m.Reply(F(chatID, "invalid_bool"))
		return tg.ErrEndGroup
	}

	if enabled == current {
		m.Reply(F(chatID, "autoplay_already", locales.Arg{
			"action": F(chatID, utils.IfElse(enabled, "enabled", "disabled")),
		}))
		return tg.ErrEndGroup
	}

	if err := database.SetAutoplay(chatID, enabled); err != nil {
		return err
	}

	m.Reply(F(chatID, "autoplay_updated", locales.Arg{
		"action": F(chatID, utils.IfElse(enabled, "enabled", "disabled")),
	}))
	return tg.ErrEndGroup
}

// autoplayHistoryKey is the RoomState.Data key holding every track ID
// AND normalized title played in this room's session (manually queued or
// autoplay-picked), so autoplay never repeats a track already heard this
// session - whether it's the exact same upload (same ID) or a different
// upload of the same song (same title, different ID), and whether it was
// the original manually-requested track or one autoplay picked itself.
const autoplayHistoryKey = "autoplay_history"

// autoplayCandidates returns tracks worth trying for autoplay's next pick.
// For YouTube-sourced tracks, it uses YouTube's own "Mix" recommendations
// (the same same-vibe curation YouTube's own player uses for "up next"),
// which actually diversifies into different-but-similar tracks. For
// anything else (Spotify, SoundCloud, Telegram...), it first finds the song
// on YouTube and uses that video's Mix; only if that fails does it fall
// back to searching by the previous track's (cleaned) title.
func autoplayCandidates(
	r *core.RoomState,
	last *state.Track,
) ([]*state.Track, error) {
	seedID := ""

	if last.Source == platforms.PlatformYouTube && last.ID != "" {
		seedID = last.ID
	} else if q := cleanAutoplayQuery(last.Title); q != "" {
		// Spotify / SoundCloud / Telegram tracks have no YouTube video ID,
		// so searching by their title used to only resurface the same song
		// (filtered out as "already played"), and autoplay played nothing
		// similar. Find the song on YouTube first and use ITS Mix as the
		// seed, so the next tracks match the language/genre/vibe of what
		// the user actually played.
		if found, err := platforms.SearchQuery(q, false); err == nil {
			for _, f := range found {
				if f != nil && f.ID != "" {
					seedID = f.ID
					// Same song as the one just played - never suggest it.
					pushAutoplayHistory(r, f.ID, f.Title)
					break
				}
			}
		}
	}

	if seedID != "" {
		tracks, err := platforms.GetSimilarTracks(seedID, 10)
		if err == nil && len(tracks) > 0 {
			return tracks, nil
		}
	}

	query := cleanAutoplayQuery(last.Title)
	if query == "" {
		return nil, nil
	}
	return platforms.SearchQuery(query, false)
}

// autoplayNextTrack finds a track similar to the last one played in the
// given room, to keep music going when the queue runs out. It returns nil
// if autoplay is disabled for the chat, there's no track to base the
// search on, or no suitable track is found.
func autoplayNextTrack(chatID int64, r *core.RoomState) *state.Track {
	enabled, err := database.Autoplay(chatID)
	if err != nil || !enabled {
		return nil
	}

	last := r.Track()
	if last == nil || (last.Title == "" && last.ID == "") {
		return nil
	}

	// Record the track autoplay is branching off from too (it may have
	// been manually /play'd and never added to history otherwise), so it
	// can never be re-suggested later in this session either.
	pushAutoplayHistory(r, last.ID, last.Title)

	tracks, err := autoplayCandidates(r, last)
	if err != nil || len(tracks) == 0 {
		return nil
	}

	history := autoplayHistory(r)

	// pick records a candidate in the session history and returns it.
	pick := func(track *state.Track) *state.Track {
		track.Requester = F(chatID, "autoplay_requester")
		pushAutoplayHistory(r, track.ID, track.Title)
		return track
	}

	// Candidates whose length is wildly different from the song just played
	// (podcasts, hour-long mixes, 30-second clips) are rarely the same vibe,
	// so they're only used if nothing better exists.
	var fallback *state.Track

	for _, track := range tracks {
		if track == nil {
			continue
		}
		if containsEntry(history, track.ID, track.Title) {
			continue
		}
		// Same limit /play enforces - without it autoplay can pick a
		// multi-hour mix/compilation as "the next similar song".
		if track.Duration > config.DurationLimit {
			continue
		}
		if last.Duration > 0 && track.Duration > 0 &&
			(track.Duration*5 < last.Duration*2 ||
				track.Duration*2 > last.Duration*5) {
			if fallback == nil {
				fallback = track
			}
			continue
		}
		return pick(track)
	}

	if fallback != nil {
		return pick(fallback)
	}

	return nil
}

// autoplayHistoryEntry pairs a video ID with its normalized title so a
// candidate can be matched on either.
type autoplayHistoryEntry struct {
	ID    string
	Title string
}

// autoplayHistory reads everything played so far in this room's session.
func autoplayHistory(r *core.RoomState) []autoplayHistoryEntry {
	ok, v := r.GetData(autoplayHistoryKey)
	if !ok {
		return nil
	}
	history, _ := v.([]autoplayHistoryEntry)
	return history
}

// pushAutoplayHistory records a played track (by ID and normalized title)
// for this session, unless it's already recorded.
func pushAutoplayHistory(r *core.RoomState, trackID, title string) {
	if trackID == "" && title == "" {
		return
	}
	history := autoplayHistory(r)
	if containsEntry(history, trackID, title) {
		return
	}
	r.SetData(autoplayHistoryKey, append(history, autoplayHistoryEntry{
		ID:    trackID,
		Title: normalizeAutoplayTitle(title),
	}))
}

// containsEntry reports whether a track (by exact ID, or by a song title
// that normalizes the same as something already heard - e.g. a different
// upload of the same song) has already been seen this session.
func containsEntry(history []autoplayHistoryEntry, id, title string) bool {
	normalized := normalizeAutoplayTitle(title)
	for _, existing := range history {
		if id != "" && existing.ID == id {
			return true
		}
		if normalized != "" && existing.Title == normalized {
			return true
		}
	}
	return false
}

// normalizeAutoplayTitle strips the same noise cleanAutoplayQuery does and
// lowercases the result, so different uploads of the same song (e.g.
// "Daku (Official Video)" vs "Daku (Lyrical Video)" vs a slightly
// re-punctuated re-upload) are recognized as the same song rather than
// "new" content.
func normalizeAutoplayTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(cleanAutoplayQuery(title)))
}

// cleanAutoplayQuery strips common noise (official video/audio tags,
// bracketed text, featuring credits) from a track title so the search
// is more likely to surface a genuinely similar track rather than the
// exact same upload.
func cleanAutoplayQuery(title string) string {
	noise := []string{
		"(Official Video)", "(Official Audio)", "(Official Music Video)",
		"[Official Video]", "[Official Audio]", "(Lyrics)", "(Lyric Video)",
		"(Audio)", "(Visualizer)", "(HD)", "(4K)",
	}
	cleaned := title
	for _, n := range noise {
		cleaned = strings.ReplaceAll(cleaned, n, "")
	}
	return strings.TrimSpace(cleaned)
}
