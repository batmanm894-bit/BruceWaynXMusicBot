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
	"os"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	state "main/internal/core/models"
)

const PlatformSaavn state.PlatformName = "Saavn"

// saavnSearchResponse covers the JSON shape returned by the
// sumitkolhe/jiosaavn-api (a.k.a. saavn.dev) "/api/search/songs" endpoint.
// Only the fields this platform actually needs are declared.
type saavnSearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Results []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			DownloadURL []struct {
				Quality string `json:"quality"`
				URL     string `json:"url"`
			} `json:"downloadUrl"`
		} `json:"results"`
	} `json:"data"`
}

// bestDownloadURL picks the highest-quality direct link from the
// downloadUrl array. The API lists qualities from lowest to highest
// (e.g. 12kbps ... 320kbps), so the last entry is normally the best - but
// this scans explicitly for "320kbps" first in case ordering ever changes.
func (r *saavnSearchResponse) bestDownloadURL() string {
	if len(r.Data.Results) == 0 {
		return ""
	}
	links := r.Data.Results[0].DownloadURL
	if len(links) == 0 {
		return ""
	}
	for _, l := range links {
		if l.Quality == "320kbps" {
			return l.URL
		}
	}
	return links[len(links)-1].URL
}

type SaavnPlatform struct {
	name state.PlatformName
}

func init() {
	// JioSaavn doesn't know about YouTube video IDs - it has its own
	// catalog, so tracks are matched by title. Tried between ShrutiAPI (75)
	// and YT-DLP (60): it's a title-search match rather than an exact
	// video, but a self-hosted instance is generally quota-free, so it's
	// worth trying before falling back to yt-dlp's cookie/PO-token issues.
	Register(65, &SaavnPlatform{
		name: PlatformSaavn,
	})
}

func (s *SaavnPlatform) Name() state.PlatformName {
	return s.name
}

func (s *SaavnPlatform) CanGetTracks(query string) bool {
	return false
}

func (s *SaavnPlatform) GetTracks(
	_ string,
	_ bool,
) ([]*state.Track, error) {
	return nil, errors.New("saavn is a download-only platform")
}

func (s *SaavnPlatform) CanDownload(source state.PlatformName) bool {
	if len(config.SaavnAPIURLs) == 0 {
		return false
	}
	return source == PlatformYouTube
}

func (s *SaavnPlatform) Download(
	ctx context.Context,
	track *state.Track,
	statusMsg *telegram.NewMessage,
) (string, error) {
	// JioSaavn only serves audio - never claim tracks requested as video.
	if track.Video {
		return "", errors.New("saavn does not support video")
	}

	if f := findFile(track); f != "" {
		gologging.Debug("Saavn: Download -> Cached File -> " + f)
		return f, nil
	}
	return s.downloadToDisk(ctx, track)
}

func (s *SaavnPlatform) downloadToDisk(
	ctx context.Context,
	track *state.Track,
) (string, error) {
	path := getPath(track, ".mp3")

	markDownloading(downloadKey(track))
	defer unmarkDownloading(downloadKey(track))

	mediaURL, err := s.resolveDownloadURL(ctx, track.Title)
	if err != nil {
		return "", err
	}

	if err := s.fetchAndSave(ctx, mediaURL, path); err != nil {
		return "", err
	}

	if !fileExists(path) {
		return "", errors.New("empty file returned by API")
	}

	if err := verifyMediaFile(ctx, path); err != nil {
		os.Remove(path)
		return "", err
	}

	return path, nil
}

// resolveDownloadURL searches JioSaavn's catalog by the track's title and
// returns the best-quality direct media URL for the top match. Tries every
// configured base URL in order (self-hosted instances are interchangeable
// mirrors, not separate accounts, so there's no key rotation here like
// ShrutiAPI/FallenApi).
func (s *SaavnPlatform) resolveDownloadURL(
	ctx context.Context,
	title string,
) (string, error) {
	var lastErr error

	for _, base := range config.SaavnAPIURLs {
		searchURL := fmt.Sprintf(
			"%s/api/search/songs?query=%s&limit=1",
			base,
			url.QueryEscape(title),
		)

		resp, err := rc.R().SetContext(ctx).Get(searchURL)
		if err != nil {
			lastErr = fmt.Errorf("saavn search at %s failed: %w", base, err)
			continue
		}

		if resp.StatusCode() >= 400 {
			lastErr = fmt.Errorf(
				"saavn search at %s failed with status: %d body: %s",
				base, resp.StatusCode(), resp.String(),
			)
			gologging.Debug("Saavn: search failed, trying next -> " + lastErr.Error())
			continue
		}

		var parsed saavnSearchResponse
		if err := json.Unmarshal(resp.Bytes(), &parsed); err != nil {
			lastErr = fmt.Errorf("saavn: failed to parse response from %s: %w", base, err)
			continue
		}

		if !parsed.Success || len(parsed.Data.Results) == 0 {
			lastErr = fmt.Errorf("saavn: no results for %q on %s", title, base)
			gologging.Debug("Saavn: " + lastErr.Error())
			continue
		}

		mediaURL := parsed.bestDownloadURL()
		if mediaURL == "" {
			lastErr = fmt.Errorf("saavn: no downloadable link for %q on %s", title, base)
			continue
		}

		return mediaURL, nil
	}

	if lastErr == nil {
		lastErr = errors.New("saavn: no endpoints configured")
	}
	return "", lastErr
}

// fetchAndSave downloads the resolved media URL straight to disk.
func (s *SaavnPlatform) fetchAndSave(
	ctx context.Context,
	mediaURL, path string,
) error {
	resp, err := rc.R().SetContext(ctx).Get(mediaURL)
	if err != nil {
		return fmt.Errorf("saavn: failed to download media: %w", err)
	}

	if resp.StatusCode() >= 400 {
		return fmt.Errorf(
			"saavn: media download failed with status: %d", resp.StatusCode(),
		)
	}

	body := resp.Bytes()
	if len(body) == 0 {
		return errors.New("saavn: media download returned an empty response")
	}

	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
