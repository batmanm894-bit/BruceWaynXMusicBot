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
	"context"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/core"
	state "main/internal/core/models"
	"main/internal/locales"
	"main/internal/platforms"
	"main/internal/utils"
	"main/ntgcalls"
)

func streamEndHandler(
	chatID int64,
	streamType ntgcalls.StreamType,
	_ ntgcalls.StreamDevice,
) {
	if streamType == ntgcalls.VideoStream {
		gologging.Debug("[onStreamEndHandler] Video stream ended, returning")
		return
	}

	gologging.DebugF("[onStreamEndHandler] Stream ended in chat %d", chatID)

	// This handler is invoked directly from the ntgcalls callback (registered
	// via a.Ntg.OnStreamEnd), which is NOT wrapped by SafeMessageHandler like
	// the regular command handlers are. Any unrecovered panic here (e.g. a
	// nil *state.Track slipping through a race on the room's queue/loop
	// state) would crash the whole process and cause the bot to restart.
	defer func() {
		if rec := recover(); rec != nil {
			gologging.ErrorF("[onStreamEndHandler] recovered panic for chat %d: %v", chatID, rec)
			core.DeleteRoom(chatID)
		}
	}()

	ass, err := core.Assistants.ForChat(chatID)
	if err != nil {
		gologging.ErrorF("Failed to get Assistant for %d: %v", chatID, err)
		return
	}
	r, ok := core.GetRoom(chatID, ass, false)
	if !ok {
		return
	}
	scheduleOldPlayingMessage(r)

	if ok, v := r.GetData("is_transitioning"); ok {
		if ok, v := v.(bool); ok && v {
			return
		}
	}

	r.SetData("is_transitioning", true)
	defer r.DeleteData("is_transitioning")

	cid := r.ChatID
	r.Parse()

	var t *state.Track
	var wasLooping bool
	if len(r.Queue()) == 0 && r.Loop() == 0 {
		if !hasVoiceChatListeners(ass, cid) {
			// Nobody's left in the voice chat but the assistant itself -
			// autoplay would otherwise keep chaining forever, burning
			// search/download API quota (YouTube, FallenApi, ShrutiAPI,
			// YtDlp) into an empty room. Stop here instead of calling
			// autoplayNextTrack.
			gologging.DebugF(
				"[onStreamEndHandler] No listeners left in %d, skipping autoplay",
				cid,
			)
			core.DeleteRoom(chatID)
			core.Bot.SendMessage(cid, F(cid, "stream_queue_finished"))
			return
		}

		if next := autoplayNextTrack(cid, r); next != nil {
			r.AddTracksToQueue([]*state.Track{next})
			wasLooping = false
			t = r.NextTrack()
		} else {
			core.DeleteRoom(chatID)
			core.Bot.SendMessage(cid, F(cid, "stream_queue_finished"))
			return
		}
	} else {
		wasLooping = r.Loop() > 0
		t = r.NextTrack()
		deleteQueueMsg(cid, t)
	}

	if t == nil {
		gologging.DebugF("[onStreamEndHandler] No track resolved for chat %d, ending stream", cid)
		core.DeleteRoom(chatID)
		core.Bot.SendMessage(cid, F(cid, "stream_queue_finished"))
		return
	}

	statusText := F(cid, "stream_downloading_next")
	if wasLooping && t != nil && r.FilePath() != "" {
		statusText = F(cid, "cb_replaying")
	}

	statusMsg, err := core.Bot.SendMessage(
		cid,
		statusText,
	)
	if err != nil {
		gologging.ErrorF("[call.go] Failed to send msg: %v", err)
	}

	var filePath string
	if wasLooping && t != nil && r.FilePath() != "" {
		filePath = r.FilePath()
	} else {
		filePath, err = platforms.Download(context.Background(), t, statusMsg)
	}

	if err != nil {
		gologging.ErrorF(
			"[onStreamEndHandler] Download failed for %s: %v",
			t.URL,
			err,
		)
		utils.EOR(statusMsg, F(cid, "stream_download_fail", locales.Arg{
			"error": err.Error(),
		}))
		core.DeleteRoom(chatID)

		return
	}

	if err := r.Play(t, filePath, true); err != nil {
		gologging.ErrorF(
			"[onStreamEndHandler] Play failed for %s: %v",
			t.URL,
			err,
		)
		utils.EOR(statusMsg, F(cid, "stream_play_fail"))
		core.DeleteRoom(chatID)

		return
	}
	prefetchNextInQueue(r)

	title := utils.ShortTitle(t.Title, 25)
	safeTitle := utils.EscapeHTML(title)

	msgText := F(cid, nowPlayingKey(), locales.Arg{
		"url":      t.URL,
		"title":    safeTitle,
		"duration": utils.FormatDuration(t.Duration),
		"by":       t.Requester,
		"source":   string(t.Source),
		"bot":      core.Bot.Me().FirstName,
		"bot_link": "https://t.me/" + core.Bot.Me().Username + "?start=start",
	})

	opt := &telegram.SendOptions{
		ParseMode:   "HTML",
		ReplyMarkup: core.GetPlayMarkup(cid, r, false),
	}

	if t.Artwork != "" && shouldShowThumb(chatID) {
		opt.Media = utils.CleanURL(t.Artwork)
	}

	statusMsg, _ = utils.EOR(statusMsg, msgText, opt)
	r.SetStatusMsg(statusMsg)
}

// prefetchNextInQueue starts warming up the download for the next queued
// track in the background as soon as the current track begins playing, so
// it's ready (or already cached) by the time it's needed instead of only
// starting once the current track ends. No-op if the queue is empty.
func prefetchNextInQueue(r *core.RoomState) {
	if q := r.Queue(); len(q) > 0 {
		platforms.Prefetch(q[0])
	}
}

// hasVoiceChatListeners reports whether anyone besides the assistant
// itself is currently present in chatID's voice chat. Used to stop
// autoplay from chaining forever (and burning search/download API quota)
// once everyone has left and only the assistant remains connected.
//
// On any error fetching participants (e.g. a transient API hiccup), this
// defaults to true (assume listeners present) rather than silently
// killing playback - autoplay should only stop when we're actually sure
// the room is empty.
func hasVoiceChatListeners(ass *core.Assistant, chatID int64) bool {
	participants, err := ass.Ntg.GetParticipants(chatID)
	if err != nil {
		gologging.DebugF(
			"[autoplay] Failed to fetch participants for %d, assuming listeners present: %v",
			chatID, err,
		)
		return true
	}

	// The assistant itself is always one of the participants once it has
	// joined the call, so "just the assistant" means length 1 (or 0, if
	// the participant list hasn't caught up yet - also treated as empty).
	return len(participants) > 1
}
