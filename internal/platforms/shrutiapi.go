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

package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"math"
	"os"
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	state "main/internal/core/models"
	"main/internal/utils"
)

const PlatformShrutiAPI state.PlatformName = "ShrutiAPI"

// shrutiDeadKeys tracks, per API key, a unix timestamp until which that key
// is skipped (no HTTP request spent on it). A 429 (quota/rate limit) only
// switches that one key off for 10 minutes; 401/403 with an
// expired/invalid style body switches it off until restart. The other
// keys keep working normally.
var shrutiDeadKeys sync.Map

const shrutiPermanentDeadline = math.MaxInt64

func shrutiKeyCoolingDown(key string) bool {
	v, ok := shrutiDeadKeys.Load(key)
	if !ok {
		return false
	}
	until, _ := v.(int64)
	return time.Now().Unix() < until
}

func shrutiLiveKeyCount() int {
	n := 0
	for _, k := range config.ShrutiAPIKeys {
		if !shrutiKeyCoolingDown(k) {
			n++
		}
	}
	return n
}

func markShrutiKeyDead(key string, permanent bool, reason string) {
	wasAlreadyDead := shrutiKeyCoolingDown(key)

	until := int64(shrutiPermanentDeadline)
	if !permanent {
		until = time.Now().Add(10 * time.Minute).Unix()
	}
	shrutiDeadKeys.Store(key, until)

	if wasAlreadyDead {
		return
	}
	if permanent {
		sendAdminAlert(fmt.Sprintf(
			"⚠️ ShrutiAPI: key ending in %s is dead (%s) and is switched "+
				"off until the bot is restarted with a corrected/removed key. "+
				"%d other key(s) still active.",
			maskKey(key), reason, shrutiLiveKeyCount(),
		))
		return
	}
	sendAdminAlert(fmt.Sprintf(
		"⚠️ ShrutiAPI: key ending in %s is temporarily switched off for "+
			"10 minutes (%s). %d other key(s) still active.",
		maskKey(key), reason, shrutiLiveKeyCount(),
	))
}

// ShrutiAPICoolingDown reports whether ShrutiAPI can't serve a request
// right now: no keys configured, or EVERY key is switched off (expired or
// rate-limited). While at least one key is alive this is false. When true
// it fails instantly (no network call), so raceDelayFor uses it to skip
// the stagger delay for the candidates behind it.
func ShrutiAPICoolingDown() bool {
	if len(config.ShrutiAPIKeys) == 0 {
		return true
	}
	return shrutiLiveKeyCount() == 0
}

// shrutiAPIErrorResponse covers the JSON shape ShrutiAPI sends back on
// failure (e.g. invalid key, rate limit). On success it does NOT return
// JSON at all - it streams the raw audio/video bytes directly as the
// response body - so this is only used to extract a readable message when
// something goes wrong.
type shrutiAPIErrorResponse struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (r shrutiAPIErrorResponse) text() string {
	if r.Message != "" {
		return r.Message
	}
	return r.Error
}

type ShrutiAPIPlatform struct {
	name state.PlatformName
}

func init() {
	// Tried right after FallenApi (80) and before YT-DLP (60).
	// Tried first among download candidates - in practice the fastest
	// and most reliable of the free/self-hosted sources, so putting it
	// first (no stagger delay) means the other, heavier candidates often
	// never even need to start - keeping load on free-tier hosting low.
	Register(82, &ShrutiAPIPlatform{
		name: PlatformShrutiAPI,
	})
}

func (s *ShrutiAPIPlatform) Name() state.PlatformName {
	return s.name
}

func (s *ShrutiAPIPlatform) CanGetTracks(query string) bool {
	return false
}

func (s *ShrutiAPIPlatform) GetTracks(
	_ string,
	_ bool,
) ([]*state.Track, error) {
	return nil, errors.New("shrutiapi is a download-only platform")
}

func (s *ShrutiAPIPlatform) CanDownload(source state.PlatformName) bool {
	if len(config.ShrutiAPIURLs) == 0 || len(config.ShrutiAPIKeys) == 0 {
		return false
	}
	return source == PlatformYouTube
}

func (s *ShrutiAPIPlatform) Download(
	ctx context.Context,
	track *state.Track,
	statusMsg *telegram.NewMessage,
) (string, error) {
	if f := findFile(track); f != "" {
		gologging.Debug("ShrutiAPI: Download -> Cached File -> " + f)
		return f, nil
	}
	return s.downloadToDisk(ctx, track, statusMsg)
}

// downloadToDisk fetches the track from ShrutiAPI and writes it straight
// to disk. ShrutiAPI serves the media file directly in the response body
// (confirmed from logs: a raw fragmented-mp4/m4a container), not a JSON
// object pointing to a separate URL - so there's no "instant stream" path
// like FallenApi's CDN links; every request is a full download.
func (s *ShrutiAPIPlatform) downloadToDisk(
	ctx context.Context,
	track *state.Track,
	statusMsg *telegram.NewMessage,
) (string, error) {
	var pm *telegram.ProgressManager
	if statusMsg != nil {
		pm = utils.GetProgress(statusMsg)
	}

	mediaType := "audio"
	ext := ".m4a"
	if track.Video {
		mediaType = "video"
		ext = ".mp4"
	}
	path := getPath(track, ext)

	markDownloading(downloadKey(track))
	defer unmarkDownloading(downloadKey(track))

	if err := s.fetchAndSave(ctx, track.ID, mediaType, path, pm); err != nil {
		return "", err
	}

	if !fileExists(path) {
		return "", errors.New("empty file returned by API")
	}

	return path, nil
}

// fetchAndSave picks a random key each call (so usage spreads evenly
// across all configured keys instead of always starting with the first
// one), and tries each configured base URL in order for that key (the
// announcement says the three endpoints are interchangeable). If a key is
// exhausted (daily limit, etc.) on every URL, it falls through to the next
// key. On success, the response body (raw media bytes) is written
// directly to path.
func (s *ShrutiAPIPlatform) fetchAndSave(
	ctx context.Context,
	videoID, mediaType, path string,
	pm *telegram.ProgressManager,
) error {
	var lastErr error

	if ShrutiAPICoolingDown() {
		return fmt.Errorf("shrutiapi cooling down: all keys expired/rate-limited")
	}

	for _, key := range shuffledKeys(config.ShrutiAPIKeys) {
		if shrutiKeyCoolingDown(key) {
			continue
		}
	baseLoop:
		for _, base := range config.ShrutiAPIURLs {
			apiReqURL := fmt.Sprintf(
				"%s/download?url=%s&type=%s&api_key=%s",
				base,
				url.QueryEscape(videoID),
				mediaType,
				key,
			)

			resp, err := rc.R().SetContext(ctx).Get(apiReqURL)
			if err != nil {
				if errors.Is(err, context.Canceled) ||
					errors.Is(err, context.DeadlineExceeded) {
					return sanitizeAPIError(err, key)
				}
				lastErr = sanitizeAPIError(
					fmt.Errorf("shrutiapi request to %s failed: %w", base, err),
					key,
				)
				continue
			}

			if resp.StatusCode() >= 400 {
				msg := resp.String()
				var errResp shrutiAPIErrorResponse
				if jsonErr := json.Unmarshal(resp.Bytes(), &errResp); jsonErr == nil && errResp.text() != "" {
					msg = errResp.text()
				}
				lastErr = sanitizeAPIError(fmt.Errorf(
					"shrutiapi request to %s failed with status: %d body: %s",
					base, resp.StatusCode(), msg,
				), key)
				gologging.Debug("ShrutiAPI: key/url failed, trying next -> " + lastErr.Error())
				switch {
				case resp.StatusCode() == 401,
					resp.StatusCode() == 403 && isDeadKeyBody(msg):
					// Key itself is dead (invalid / plan expired): stop
					// using it and move on to the next key.
					markShrutiKeyDead(key, true, "invalid key / plan expired")
					break baseLoop
				case resp.StatusCode() == 429:
					// Quota is per key, so the other base URLs would
					// answer the same: switch only this key off for a
					// while and try the next key.
					markShrutiKeyDead(key, false, "rate limit / daily quota exhausted")
					break baseLoop
				}
				continue
			}

			body := resp.Bytes()
			if len(body) == 0 {
				lastErr = sanitizeAPIError(fmt.Errorf(
					"shrutiapi at %s returned an empty response", base,
				), key)
				continue
			}

			if err := os.WriteFile(path, body, 0o600); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			if err := verifyMediaFile(ctx, path); err != nil {
				os.Remove(path)
				lastErr = sanitizeAPIError(fmt.Errorf(
					"shrutiapi at %s returned a corrupt/truncated file: %w", base, err,
				), key)
				gologging.Debug("ShrutiAPI: " + lastErr.Error())
				continue
			}

			_ = pm // reserved: wire up progress reporting here if/when needed
			return nil
		}
	}

	if lastErr == nil {
		lastErr = errors.New("shrutiapi: no keys/endpoints configured")
	}
	gologging.Error(lastErr.Error())
	return lastErr
}
