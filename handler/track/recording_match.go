package track

import (
	"context"
	"fmt"
	"sort"
	"strings"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	spot "github.com/zmb3/spotify/v2"
)

func normalizedName(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Exact names retain edition qualifiers. Never strip "live", "remix", or "remaster".
func selectRecording(candidates []musicbrainz.RecordingMatch, ft *spot.FullTrack, byISRC bool) string {
	selected := ""
	for _, candidate := range candidates {
		if candidate.ID == "" || normalizedName(candidate.Title) != normalizedName(ft.Name) || candidate.ArtistCredits == nil {
			continue
		}
		artists := map[string]bool{}
		for _, credit := range *candidate.ArtistCredits {
			if credit.Artist != nil {
				artists[normalizedName(credit.Artist.Name)] = true
			}
		}
		if len(ft.Artists) == 0 || len(artists) != len(ft.Artists) {
			continue
		}
		matched := true
		for _, artist := range ft.Artists {
			if !artists[normalizedName(artist.Name)] {
				matched = false
			}
		}
		if !matched {
			continue
		}
		if !byISRC {
			// Name searches must also supply an unambiguous edition and matching duration.
			delta := candidate.Length - int(ft.Duration)
			if candidate.Disambiguation != "" || candidate.Length <= 0 || ft.Duration <= 0 || delta < -2000 || delta > 2000 {
				continue
			}
		}
		if selected != "" && selected != candidate.ID {
			return ""
		}
		selected = candidate.ID
	}
	return selected
}

func quoteSearch(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func (h *GetTrackV2Handler) findRecording(ctx context.Context, ft *spot.FullTrack) (*mb.Recording, error) {
	id := ""
	if isrc := ft.ExternalIDs["isrc"]; isrc != "" {
		result, err := h.musicbrainzClient.SearchRecordingsContext(ctx, "isrc:"+quoteSearch(isrc))
		if err != nil {
			h.log.Warnw("MusicBrainz ISRC unavailable", "error", err)
			return nil, err
		}
		if result.Count <= len(result.Recordings) {
			id = selectRecording(result.Recordings, ft, true)
		}
		// Ambiguous ISRC evidence must not be replaced by a weaker name match.
		if id == "" && len(result.Recordings) > 0 {
			return nil, nil
		}
	}
	if id == "" && len(ft.Artists) > 0 && ft.Name != "" {
		query := fmt.Sprintf("artist:%s AND recording:%s", quoteSearch(ft.Artists[0].Name), quoteSearch(ft.Name))
		result, err := h.musicbrainzClient.SearchRecordingsContext(ctx, query)
		if err != nil {
			h.log.Warnw("MusicBrainz name search unavailable", "error", err)
			return nil, err
		}
		if result.Count <= len(result.Recordings) {
			id = selectRecording(result.Recordings, ft, false)
		}
	}
	if id == "" {
		return nil, nil
	}
	rec, err := h.musicbrainzClient.GetRecordingContext(ctx, id)
	if err != nil {
		h.log.Warnw("MusicBrainz recording unavailable", "error", err)
		return nil, err
	}
	if rec.ID != id {
		return nil, fmt.Errorf("MusicBrainz returned an unexpected recording")
	}
	return &rec, nil
}

// Restore release information without a network request for every cover image.
func getRecordingReleases(rec mb.Recording) *[]occipital.Release {
	if rec.Releases == nil || rec.ArtistCredits == nil {
		return nil
	}
	artists := map[string]bool{}
	for _, credit := range *rec.ArtistCredits {
		if credit.Artist != nil && credit.Artist.ID != "" {
			artists[credit.Artist.ID] = true
		}
	}
	releases := make([]occipital.Release, 0)
	seen := map[string]bool{}
	for _, release := range *rec.Releases {
		if release.ID == "" || release.Status != "Official" || seen[release.ID] || release.ArtistCredit == nil {
			continue
		}
		matches := false
		for _, credit := range *release.ArtistCredit {
			if credit.Artist != nil && artists[credit.Artist.ID] {
				matches = true
			}
		}
		if !matches {
			continue
		}
		seen[release.ID] = true
		releases = append(releases, occipital.Release{ID: release.ID, Title: release.Title, Date: release.Date, Country: release.Country, Disambiguation: release.Disambiguation, Image: getCoverArtArchiveImageURL(release.ID, "front", 250).String()})
	}
	sort.Slice(releases, func(i, j int) bool {
		if releases[i].Date != releases[j].Date {
			return releases[i].Date < releases[j].Date
		}
		return releases[i].ID < releases[j].ID
	})
	if len(releases) > 12 {
		releases = releases[:12]
	}
	return &releases
}
