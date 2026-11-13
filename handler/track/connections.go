package track

import (
	"github.com/mager/occipital/musicbrainz"
	"github.com/mager/occipital/occipital"
	"regexp"
	"sort"
	"strings"
	"time"
)

var recordingIDPattern = regexp.MustCompile(`(?i)^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$`)

func getRecordingConnections(rec musicbrainz.RecordingDetails) []occipital.RecordingConnection {
	result := []occipital.RecordingConnection{}
	seen := map[string]bool{}
	order := map[string]int{"samples material": 0, "remix": 1, "edit": 2}
	for _, rel := range rec.RecordingRelations {
		target := rel.Recording
		if _, supported := order[rel.Type]; !supported || target == nil || rel.TargetType != "recording" {
			continue
		}
		id := strings.ToLower(target.ID)
		title := strings.TrimSpace(target.Title)
		if !recordingIDPattern.MatchString(id) || strings.EqualFold(id, rec.ID) || title == "" || (rel.Direction != "forward" && rel.Direction != "backward") {
			continue
		}
		key := rel.Type + ":" + rel.Direction + ":" + id
		if seen[key] {
			continue
		}
		seen[key] = true
		var artist strings.Builder
		for _, credit := range target.ArtistCredit {
			name := credit.Name
			if strings.TrimSpace(name) == "" {
				name = credit.Artist.Name
			}
			if strings.TrimSpace(name) != "" {
				artist.WriteString(name)
				artist.WriteString(credit.JoinPhrase)
			}
		}
		result = append(result, occipital.RecordingConnection{ID: id, Title: title, Artist: strings.TrimSpace(artist.String()), Disambiguation: strings.TrimSpace(target.Disambiguation), Type: rel.Type, Direction: rel.Direction})
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Type != b.Type {
			return order[a.Type] < order[b.Type]
		}
		if a.Direction != b.Direction {
			return a.Direction == "forward"
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
	return result
}

func getRecordingStudios(rec musicbrainz.RecordingDetails) []occipital.RecordingStudio {
	result := []occipital.RecordingStudio{}
	seen := map[string]bool{}
	order := map[string]int{"recorded at": 0, "mixed at": 1, "produced at": 2, "engineered at": 3}
	for _, rel := range rec.PlaceRelations {
		place := rel.Place
		if _, supported := order[rel.Type]; !supported || place == nil || rel.Direction != "backward" || !recordingIDPattern.MatchString(place.ID) || strings.TrimSpace(place.Name) == "" {
			continue
		}
		begin, end := recordingDate(rel.Begin), recordingDate(rel.End)
		additional := false
		for _, attribute := range rel.Attributes {
			if attribute == "additional" {
				additional = true
			}
		}
		key := strings.ToLower(place.ID) + ":" + rel.Type + ":" + begin + ":" + end
		if additional {
			key += ":additional"
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, occipital.RecordingStudio{ID: strings.ToLower(place.ID), Name: strings.TrimSpace(place.Name), Area: strings.TrimSpace(place.Area.Name), Type: rel.Type, Begin: begin, End: end, Additional: additional})
	}
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Type != b.Type {
			return order[a.Type] < order[b.Type]
		}
		if a.Begin != b.Begin {
			return a.Begin < b.Begin
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return result
}

func recordingDate(value string) string {
	for _, layout := range []string{"2006", "2006-01", "2006-01-02"} {
		if len(value) != len(layout) {
			continue
		}
		if date, err := time.Parse(layout, value); err == nil && date.Year() > 0 {
			return value
		}
	}
	return ""
}
