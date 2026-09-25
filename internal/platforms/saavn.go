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
	"html"
	"net/url"
	"os"
	"strings"
	"unicode"

	"github.com/Laky-64/gologging"
	"github.com/amarnathcjd/gogram/telegram"

	"main/internal/config"
	state "main/internal/core/models"
)

const PlatformSaavn state.PlatformName = "Saavn"

// saavnSearchResponse covers the JSON shape returned by the
// sumitkolhe/jiosaavn-api (a.k.a. saavn.dev) "/api/search/songs" endpoint.
// Only the fields this platform actually needs are declared.
type saavnResult struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Duration    json.Number `json:"duration"`
	DownloadURL []struct {
		Quality string `json:"quality"`
		URL     string `json:"url"`
	} `json:"downloadUrl"`
}

type saavnSearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Results []saavnResult `json:"results"`
	} `json:"data"`
}

// bestDownloadURL picks the highest-quality direct link from the
// downloadUrl array. The API lists qualities from lowest to highest
// (e.g. 12kbps ... 320kbps), so the last entry is normally the best - but
// this scans explicitly for "320kbps" first in case ordering ever changes.
func (r *saavnResult) bestDownloadURL() string {
	links := r.DownloadURL
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

// saavnNoiseWords are common YouTube-title decorations that say nothing
// about which song it is, so they're ignored when comparing titles.
var saavnNoiseWords = map[string]bool{
	"official": true, "video": true, "audio": true, "lyrics": true,
	"lyrical": true, "lyric": true, "full": true, "song": true, "hd": true,
	"4k": true, "music": true, "visualizer": true, "ft": true, "feat": true,
	"the": true, "a": true, "and": true, "with": true,
}

// saavnTokens lowercases s, drops any (bracketed) / [bracketed] text and
// splits what's left into alphanumeric words, skipping noise words.
func saavnTokens(s string) []string {
	s = strings.ToLower(html.UnescapeString(s))

	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(', '[':
			depth++
			b.WriteRune(' ')
			continue
		case ')', ']':
			if depth > 0 {
				depth--
			}
			b.WriteRune(' ')
			continue
		}
		if depth > 0 {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}

	var out []string
	for _, w := range strings.Fields(b.String()) {
		if !saavnNoiseWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// saavnMatches reports whether a JioSaavn search hit is really the same
// song as the requested YouTube track. JioSaavn is searched by title text
// only, so without this check the top hit is often a different song, a
// cover or another version - and because it's fast it wins the download
// race and the wrong song ends up playing (and cached under the YouTube
// ID, so it keeps coming back).
func saavnMatches(track *state.Track, res *saavnResult) bool {
	if d, err := res.Duration.Int64(); err == nil && d > 0 && track.Duration > 0 {
		diff := int(d) - track.Duration
		if diff < 0 {
			diff = -diff
		}
		if diff > 10 {
			return false
		}
	}

	nameTokens := saavnTokens(res.Name)
	if len(nameTokens) == 0 {
		return false
	}

	titleSet := make(map[string]bool)
	for _, w := range saavnTokens(track.Title) {
		titleSet[w] = true
	}
	for _, w := range nameTokens {
		if !titleSet[w] {
			return false
		}
	}
	return true
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

	mediaURL, err := s.resolveDownloadURL(ctx, track)
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
// returns the best-quality direct media URL for the first hit that really
// matches the requested track (see saavnMatches). Tries every configured
// base URL in order (self-hosted instances are interchangeable mirrors, not
// separate accounts, so there's no key rotation here like
// ShrutiAPI/FallenApi).
func (s *SaavnPlatform) resolveDownloadURL(
	ctx context.Context,
	track *state.Track,
) (string, error) {
	// The raw YouTube title (often full of decorations like "Slowed +
	// Reverb", "Official Video", channel names, etc.) is a weak search
	// key and is a common reason JioSaavn comes back with "no confident
	// match". Rather than trying the raw title first and only falling
	// back to Spotify's clean "Song - Artist" title after it fails (which
	// adds the Spotify lookup's own latency on top), both searches run at
	// the same time - whichever finds a confident match first is used,
	// and the other is abandoned.
	if track.Source == PlatformSpotify {
		// Already has a clean "Song - Artist" title - a second Spotify
		// lookup of the same title would be redundant.
		return s.searchAllBases(ctx, track, track.Title)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		url string
		err error
	}

	// queryCh carries the raw text the user actually typed after /play
	// (e.g. "jheel by hardik"). When present it's normally a far cleaner
	// search key than the YouTube video's own title, which is often
	// stuffed with channel names, hashtags and decorations that make
	// JioSaavn's search come back with no confident match - so it's tried
	// first, alongside (not instead of) the other two.
	queryCh := make(chan result, 1)
	rawCh := make(chan result, 1)
	spotifyCh := make(chan result, 1)

	pending := 0

	if track.Query != "" && track.Query != track.Title {
		pending++
		go func() {
			gologging.DebugF(
				"[Saavn] Also trying user's raw query: %q",
				track.Query,
			)
			u, err := s.searchAllBases(ctx, track, track.Query)
			queryCh <- result{u, err}
		}()
	}

	pending++
	go func() {
		u, err := s.searchAllBases(ctx, track, track.Title)
		rawCh <- result{u, err}
	}()

	pending++
	go func() {
		betterTitle := SearchTitle(track.Title)
		if betterTitle == "" || betterTitle == track.Title {
			spotifyCh <- result{"", errors.New("saavn: no Spotify-resolved title available")}
			return
		}
		gologging.DebugF(
			"[Saavn] Also trying Spotify-resolved title: %q -> %q",
			track.Title,
			betterTitle,
		)
		retryTrack := *track
		retryTrack.Title = betterTitle
		u, err := s.searchAllBases(ctx, &retryTrack, betterTitle)
		spotifyCh <- result{u, err}
	}()

	// select (not a fixed read order) so whichever candidate actually
	// finishes first with a confident match wins immediately - the rest
	// are canceled and their results, if they ever arrive, are ignored.
	var firstErr error
	for i := 0; i < pending; i++ {
		select {
		case r := <-queryCh:
			if r.err == nil {
				cancel()
				return r.url, nil
			}
			firstErr = r.err
		case r := <-rawCh:
			if r.err == nil {
				cancel()
				return r.url, nil
			}
			if firstErr == nil {
				firstErr = r.err
			}
		case r := <-spotifyCh:
			if r.err == nil {
				cancel()
				return r.url, nil
			}
			if firstErr == nil {
				firstErr = r.err
			}
		}
	}

	return "", firstErr
}

// searchAllBases tries every configured JioSaavn base URL in order,
// searching by title and validating each hit against origTrack (duration +
// title tokens, see saavnMatches). origTrack is kept separate from title so
// a retry can search using a Spotify-resolved title while still validating
// against the real track's duration/ID.
func (s *SaavnPlatform) searchAllBases(
	ctx context.Context,
	origTrack *state.Track,
	title string,
) (string, error) {
	var lastErr error

	for _, base := range config.SaavnAPIURLs {
		searchURL := fmt.Sprintf(
			"%s/api/search/songs?query=%s&limit=5",
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

		var mediaURL string
		for i := range parsed.Data.Results {
			res := &parsed.Data.Results[i]
			if !saavnMatches(origTrack, res) {
				continue
			}
			mediaURL = res.bestDownloadURL()
			if mediaURL != "" {
				break
			}
		}

		if mediaURL == "" {
			lastErr = fmt.Errorf("saavn: no confident match for %q on %s", title, base)
			gologging.Debug("Saavn: " + lastErr.Error())
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
