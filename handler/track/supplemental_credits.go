package track

import "github.com/mager/occipital/occipital"

// applyReviewedCredits fills missing roles for one reviewed Spotify edition.
// Source: https://www.youtube.com/watch?v=xQ6qgGwrFHg
// Reviewed: 2026-11-09. The official Bb trickz audio description, supplied by
// Bb trickz, credits the 2025-03-26 single as follows:
// Belize Kazi: composer, lyricist; Clairo: composer, producer;
// leodaleo: mixing engineer, producer; Julio Gómez: recording engineer;
// Joe LaPorta: mastering engineer.
// Spotify's exact single ID supplies ISRC ES5022500431 and duration 105080 ms.
// MusicBrainz has no credit relations for this recording as of the review date.
//
// This is reviewed data, not a scraper or a title-based fallback. Do not apply it
// to other editions or infer MusicBrainz IDs, instruments, or additional roles.
// Existing populated roles take precedence, even when their artists differ.
// A source correction requires a new review and code change.
func applyReviewedCredits(track *occipital.Track) {
	if track == nil || track.Source != "SPOTIFY" ||
		track.SourceID != "7oL7jAZkNyolF1H15Fhlm1" || track.ISRC != "ES5022500431" ||
		normalizedName(track.Name) != "not a pretty girl" || normalizedName(track.Artist) != "bb trickz" ||
		track.Meta == nil || track.Meta.DurationMs != 105080 {
		return
	}

	songCredits := []*occipital.TrackSongCredit{
		{Credit: "composer", Artists: []occipital.CreditArtist{{Name: "Belize Kazi"}, {Name: "Clairo"}}},
		{Credit: "lyricist", Artists: []occipital.CreditArtist{{Name: "Belize Kazi"}}},
	}
	productionCredits := []*occipital.TrackProductionCredit{
		{Credit: "producer", Artists: []occipital.CreditArtist{{Name: "Clairo"}, {Name: "leodaleo"}}},
		{Credit: "mix", Artists: []occipital.CreditArtist{{Name: "leodaleo"}}},
		{Credit: "recording", Artists: []occipital.CreditArtist{{Name: "Julio Gómez"}}},
		{Credit: "mastering", Artists: []occipital.CreditArtist{{Name: "Joe LaPorta"}}},
	}
	added := false
	for _, credit := range songCredits {
		found := false
		for _, existing := range track.SongCredits {
			if existing != nil && normalizedName(existing.Credit) == credit.Credit {
				found = true
				if len(existing.Artists) == 0 {
					existing.Artists = credit.Artists
					added = true
				}
				break
			}
		}
		if !found {
			track.SongCredits = append(track.SongCredits, credit)
			added = true
		}
	}
	for _, credit := range productionCredits {
		found := false
		for _, existing := range track.ProductionCredits {
			if existing != nil && normalizedName(existing.Credit) == credit.Credit {
				found = true
				if len(existing.Artists) == 0 {
					existing.Artists = credit.Artists
					added = true
				}
				break
			}
		}
		if !found {
			track.ProductionCredits = append(track.ProductionCredits, credit)
			added = true
		}
	}
	if added {
		const source = "https://www.youtube.com/watch?v=xQ6qgGwrFHg"
		for _, link := range track.Links {
			if link.URL == source {
				return
			}
		}
		track.Links = append(track.Links, occipital.ExternalLink{Type: "youtube", URL: source})
	}
}
