package track

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	mb "github.com/mager/musicbrainz-go/musicbrainz"
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	spot "github.com/zmb3/spotify/v2"
)

func normalizedName(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Remove only an explicit, credited guest suffix. Edition qualifiers stay intact.
func titleWithoutFeaturedArtist(title string, artists []spot.SimpleArtist) string {
	title = normalizedName(title)
	for _, artist := range artists[1:] {
		guest := normalizedName(artist.Name)
		if guest == "" {
			continue
		}
		for _, prefix := range []string{"feat. ", "ft. ", "featuring "} {
			suffix := " (" + prefix + guest + ")"
			if strings.HasSuffix(title, suffix) {
				base := strings.TrimSuffix(title, suffix)
				if base != "" {
					return base
				}
			}
		}
	}
	return title
}

// A title may omit a credited guest only with ISRC, artist, and duration evidence.
func selectRecording(candidates []musicbrainz.RecordingMatch, ft *spot.FullTrack, byISRC bool) string {
	selected := ""
	for _, candidate := range candidates {
		if candidate.ID == "" || candidate.ArtistCredits == nil {
			continue
		}
		credits := *candidate.ArtistCredits
		if len(ft.Artists) == 0 || len(credits) != len(ft.Artists) {
			continue
		}
		used := make([]bool, len(credits))
		matched := true
		for _, artist := range ft.Artists {
			found := false
			for index, credit := range credits {
				if used[index] || credit.Artist == nil {
					continue
				}
				name := normalizedName(artist.Name)
				if name != "" && (name == normalizedName(credit.Name) || name == normalizedName(credit.Artist.Name)) {
					used[index], found = true, true
					break
				}
			}
			if !found {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		exactTitle := normalizedName(candidate.Title) == normalizedName(ft.Name)
		if !exactTitle && (!byISRC || titleWithoutFeaturedArtist(candidate.Title, ft.Artists) != titleWithoutFeaturedArtist(ft.Name, ft.Artists)) {
			continue
		}
		if !byISRC || !exactTitle {
			// Weaker title evidence requires an unambiguous edition and matching duration.
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

// MusicBrainz keeps remasters on the original recording. Only remove an
// explicit mastering suffix; retain live, remix, edit, mono, and other versions.
var remasterSuffix = regexp.MustCompile(`(?i)(?: - (?:[0-9]{4} )?remaster(?:ed)?(?: [0-9]{4})?| \((?:[0-9]{4} )?remaster(?:ed)?(?: [0-9]{4})?\))$`)

func recordingTitle(s string) string { return remasterSuffix.ReplaceAllString(normalizedName(s), "") }

func linkedRecordingMatches(rec musicbrainz.RecordingDetails, ft *spot.FullTrack) bool {
	title := func(value string) string {
		value = recordingTitle(value)
		if len(ft.Artists) > 0 {
			value = titleWithoutFeaturedArtist(value, ft.Artists)
		}
		return value
	}
	if rec.Video || title(rec.Title) != title(ft.Name) || rec.Length <= 0 || ft.Duration <= 0 {
		return false
	}
	delta := rec.Length - int(ft.Duration)
	if delta < -2000 || delta > 2000 {
		return false
	}
	if isrc := ft.ExternalIDs["isrc"]; isrc != "" && rec.ISRCs != nil && len(*rec.ISRCs) > 0 {
		for _, candidate := range *rec.ISRCs {
			if strings.EqualFold(candidate, isrc) {
				return true
			}
		}
		return false
	}
	return true
}

func (h *GetTrackV2Handler) findRecording(ctx context.Context, ft *spot.FullTrack) (*musicbrainz.RecordingDetails, error) {
	if ft.ID != "" {
		ids, found, err := h.musicbrainzClient.LookupRecordingsByURL(ctx, "https://open.spotify.com/track/"+string(ft.ID))
		if err != nil {
			h.log.Warnw("MusicBrainz Spotify link unavailable", "error", err)
			return nil, err
		}
		if found {
			// Do not select the first recording from a conflicting explicit link.
			if len(ids) != 1 {
				return nil, nil
			}
			rec, err := h.musicbrainzClient.GetRecordingContext(ctx, ids[0])
			if err != nil {
				return nil, err
			}
			if rec.ID != ids[0] {
				return nil, fmt.Errorf("MusicBrainz returned an unexpected recording")
			}
			if !linkedRecordingMatches(rec, ft) {
				return nil, nil
			}
			return &rec, nil
		}
	}

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
