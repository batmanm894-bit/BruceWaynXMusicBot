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
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	"main/internal/core"
	state "main/internal/core/models"
	"main/internal/utils"
)

var telegramDLRegex = regexp.MustCompile(
	`https:\/\/t\.me\/([a-zA-Z0-9_]{5,})\/(\d+)`,
)

const PlatformFallenApi state.PlatformName = "FallenApi"

type apiResponse struct {
	CdnUrl string `json:"cdnurl"`
}

type FallenApiPlatform struct {
	name state.PlatformName
}

func init() {
	Register(78, &FallenApiPlatform{
		name: PlatformFallenApi,
	})
}

func (f *FallenApiPlatform) Name() state.PlatformName {
	return f.name
}

func (f *FallenApiPlatform) CanGetTracks(query string) bool {
	return false
}

func (f *FallenApiPlatform) GetTracks(
	_ string,
	_ bool,
) ([]*state.Track, error) {
	return nil, errors.New("fallenapi is a download-only platform")
}

func (f *FallenApiPlatform) CanDownload(
	source state.PlatformName,
) bool {
	if config.FallenAPIURL == "" || len(config.FallenAPIKeys) == 0 {
		return false
	}
	return source == PlatformYouTube
}

func (f *FallenApiPlatform) Download(
	ctx context.Context,
	track *state.Track,
	statusMsg *telegram.NewMessage,
) (string, error) {
	// fallen api didn't support video downloads so disable it
	track.Video = false

	if f := findFile(track); f != "" {
		gologging.Debug("FallenApi: Download -> Cached File -> " + f)
		return f, nil
	}

	var pm *telegram.ProgressManager
	if statusMsg != nil {
		pm = utils.GetProgress(statusMsg)
	}

	dlURL, err := f.getDownloadURL(ctx, track.URL)
	if err != nil {
		return "", err
	}

	path := getPath(track, ".mp3")

	// Telegram-hosted files must be downloaded via the Telegram client;
	// there's no direct HTTP stream URL for them.
	if telegramDLRegex.MatchString(dlURL) {
		markDownloading(downloadKey(track))
		downloadedPath, downloadErr := f.downloadFromTelegram(ctx, dlURL, path, pm)
		unmarkDownloading(downloadKey(track))
		if downloadErr != nil {
			return "", downloadErr
		}
		if !fileExists(downloadedPath) {
			return "", errors.New("empty file returned by API")
		}
		return downloadedPath, nil
	}

	// Background disk-caching disabled: on an ephemeral filesystem (e.g.
	// Render's free tier), the downloads/ folder is wiped on every
	// restart, so the cache rarely survives long enough to be reused -
	// meanwhile this download still costs real CPU/bandwidth on an
	// already CPU-constrained host. If persistent disk is available,
	// this can be re-enabled: go f.cacheInBackground(dlURL, path)

	return dlURL, nil
}

// backgroundCacheDelay lets live playback have the CDN/bandwidth to itself
// for a while before the background cache download starts, instead of both
// competing for bandwidth at the same time right as playback begins.
const backgroundCacheDelay = 20 * time.Second

// cacheInBackground downloads a track to disk after playback has already
// started streaming from the CDN URL, so subsequent plays hit the local
// cache instead of re-fetching from the API. It waits backgroundCacheDelay
// first so it doesn't compete with the live stream for bandwidth.
func (f *FallenApiPlatform) cacheInBackground(dlURL, path string) {
	time.Sleep(backgroundCacheDelay)

	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := f.downloadFromURL(bgCtx, dlURL, path); err != nil {
		gologging.Debug("FallenApi: background cache failed -> " + err.Error())
		return
	}
	gologging.Debug("FallenApi: background cache complete -> " + path)
}

func (f *FallenApiPlatform) getDownloadURL(
	ctx context.Context,
	mediaURL string,
) (string, error) {
	var lastErr error
	triedAny := false

	for _, key := range shuffledKeys(config.FallenAPIKeys) {
		if fallenKeyCoolingDown(key) {
			continue
		}
		triedAny = true

		apiReqURL := fmt.Sprintf(
			"%s/api/track?api_key=%s&url=%s",
			config.FallenAPIURL,
			key,
			url.QueryEscape(mediaURL),
		)

		var apiResp apiResponse

		resp, err := rc.R().
			SetContext(ctx).
			SetResult(&apiResp).
			Get(apiReqURL)
		if err != nil {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return "", sanitizeAPIError(err, key)
			}
			lastErr = fmt.Errorf(
				"failed to download %s, api request failed: %w", mediaURL,
				sanitizeAPIError(err, key),
			)
			continue
		}

		if resp.StatusCode() >= 400 {
			// A quota/rate-limit style failure (429) is exactly the case
			// where trying the next key makes sense - other statuses
			// (bad request, not found, etc.) would fail the same way on
			// every key too, but there's no harm in still moving on.
			lastErr = sanitizeAPIError(fmt.Errorf(
				"failed to download %s, api request failed with status: %d body: %s",
				mediaURL,
				resp.StatusCode(),
				resp.String(),
			), key)
			gologging.Debug("FallenApi: key failed, trying next -> " + lastErr.Error())

			// 401 = key invalid/unrecognized, 403 with "expired"/"invalid"
			// style body (e.g. {"code":403,"message":"Plan expired"}) = the
			// key's plan is over. Neither will ever start working again on
			// its own, so the key is switched off until restart/redeploy
			// instead of wasting a request on it every single race.
			// 429 means the key is fine but temporarily out of quota -
			// worth retrying later, so it only gets a timed cooldown.
			switch {
			case resp.StatusCode() == 401:
				markFallenKeyDead(key, true, "API key invalid/not found")
			case resp.StatusCode() == 403 && isDeadKeyBody(resp.String()):
				markFallenKeyDead(key, true, "plan expired/key disabled")
			case resp.StatusCode() == 403:
				markFallenKeyDead(key, false, "forbidden (403)")
			case resp.StatusCode() == 429:
				markFallenKeyDead(key, false, "rate limit / daily quota exhausted")
			}
			continue
		}

		if apiResp.CdnUrl == "" {
			lastErr = sanitizeAPIError(fmt.Errorf(
				"failed to download %s, empty API response body: %s",
				mediaURL,
				resp.String(),
			), key)
			continue
		}

		return apiResp.CdnUrl, nil
	}

	if lastErr == nil {
		if !triedAny && len(config.FallenAPIKeys) > 0 {
			lastErr = errors.New(
				"fallenapi: all keys are currently cooling down (invalid/rate-limited)",
			)
		} else {
			lastErr = errors.New("fallenapi: no keys configured")
		}
	}
	gologging.Error(lastErr.Error())
	return "", lastErr
}

// fallenDeadKeys tracks, per API key, a unix timestamp until which that key
// is skipped entirely (no HTTP request spent on it). Mirrors ShrutiAPI's
// cooldown but per-key rather than global, since a multi-key setup can have
// some keys dead and others fine at the same time.
var fallenDeadKeys sync.Map

// fallenPermanentDeadline stands in for "until the process restarts" -
// config keys never change without a redeploy, and a key the API itself
// says doesn't exist isn't going to start existing on its own.
const fallenPermanentDeadline = math.MaxInt64

func fallenKeyCoolingDown(key string) bool {
	v, ok := fallenDeadKeys.Load(key)
	if !ok {
		return false
	}
	until, _ := v.(int64)
	return time.Now().Unix() < until
}

// markFallenKeyDead skips key for future requests: permanently (until
// restart) for an unrecoverable failure like "API Key not found" (401),
// or for 10 minutes for a temporary one like a rate limit (429). On the
// transition into cooldown (not on repeated hits while it's already
// skipped), it alerts the log chat and the owner's DM, naming which key
// (masked) and why - and for the 429 case, since the cooldown is 10
// minutes, a key that's still exhausted naturally re-triggers this same
// transition and re-alerts roughly every 10 minutes until it recovers.
func markFallenKeyDead(key string, permanent bool, reason string) {
	wasAlreadyDead := fallenKeyCoolingDown(key)

	until := int64(fallenPermanentDeadline)
	if !permanent {
		until = time.Now().Add(10 * time.Minute).Unix()
	}
	fallenDeadKeys.Store(key, until)

	if wasAlreadyDead {
		return
	}
	if permanent {
		sendAdminAlert(fmt.Sprintf(
			"⚠️ FallenApi: key ending in %s is dead (%s) and is switched "+
				"off until the bot is restarted with a corrected/removed key. "+
				"%d other key(s) still active.",
			maskKey(key), reason, fallenLiveKeyCount(),
		))
		return
	}
	sendAdminAlert(fmt.Sprintf(
		"⚠️ FallenApi: key ending in %s is temporarily switched off for "+
			"10 minutes (%s). %d other key(s) still active.",
		maskKey(key), reason, fallenLiveKeyCount(),
	))
}

// fallenLiveKeyCount returns how many configured FallenApi keys are not
// currently switched off.
func fallenLiveKeyCount() int {
	n := 0
	for _, k := range config.FallenAPIKeys {
		if !fallenKeyCoolingDown(k) {
			n++
		}
	}
	return n
}

// isDeadKeyBody reports whether an API error body says the key itself is
// unusable (expired plan, invalid/disabled key) as opposed to a one-off
// failure for a particular video.
func isDeadKeyBody(body string) bool {
	b := strings.ToLower(body)
	return strings.Contains(b, "expired") ||
		strings.Contains(b, "invalid") ||
		strings.Contains(b, "not found") ||
		strings.Contains(b, "disabled") ||
		strings.Contains(b, "suspended") ||
		strings.Contains(b, "revoked")
}

func (f *FallenApiPlatform) downloadFromURL(
	ctx context.Context,
	dlURL, path string,
) error {
	resp, err := rc.R().
		SetContext(ctx).
		Get(dlURL)
	if err != nil {
		os.Remove(path)
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("http download failed: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode())
	}

	if err := os.WriteFile(path, resp.Bytes(), 0o600); err != nil {
		os.Remove(path)
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (f *FallenApiPlatform) downloadFromTelegram(
	ctx context.Context,
	dlURL, path string,
	pm *telegram.ProgressManager,
) (string, error) {
	matches := telegramDLRegex.FindStringSubmatch(dlURL)
	if len(matches) < 3 {
		return "", fmt.Errorf("invalid telegram download url: %s", dlURL)
	}

	username := matches[1]
	messageID, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", fmt.Errorf("invalid message ID: %v", err)
	}

	msg, err := core.Bot.GetMessageByID(username, int32(messageID))
	if err != nil {
		return "", fmt.Errorf("failed to fetch Telegram message: %w", err)
	}

	dOpts := &telegram.DownloadOptions{
		FileName: path,
		Ctx:      ctx,
	}
	if pm != nil {
		dOpts.ProgressManager = pm
	}
	_, err = msg.Download(dOpts)
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}


// fallenAllKeysDown reports whether FallenApi can't possibly serve a
// request right now: no keys configured, or every key is in its
// dead/rate-limited cooldown. Lets the race start slower-but-working
// sources (yt-dlp) immediately instead of waiting behind it.
func fallenAllKeysDown() bool {
	if len(config.FallenAPIKeys) == 0 {
		return true
	}
	for _, key := range config.FallenAPIKeys {
		if !fallenKeyCoolingDown(key) {
			return false
		}
	}
	return true
}
