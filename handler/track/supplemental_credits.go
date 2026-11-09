package track

import "github.com/mager/occipital/occipital"

// Reviewed credit data is matched to an exact Spotify edition, ISRC, title,
// primary artist and duration. Do not infer IDs or carry it to other editions.
// Existing populated roles take precedence. Corrections require a source review.
type reviewedCredits struct {
	spotifyID, isrc, title, artist string
	duration                       int
	source                         occipital.ExternalLink
	songs                          []*occipital.TrackSongCredit
	production                     []*occipital.TrackProductionCredit
	performance                    []*occipital.TrackInstrumentArtists
}

func reviewedCreditEntries() []reviewedCredits {
	return []reviewedCredits{
		{
			// Official audio description, reviewed 2026-11-09.
			spotifyID: "7oL7jAZkNyolF1H15Fhlm1", isrc: "ES5022500431", title: "not a pretty girl", artist: "bb trickz", duration: 105080,
			source: occipital.ExternalLink{Type: "youtube", URL: "https://www.youtube.com/watch?v=xQ6qgGwrFHg"},
			songs: []*occipital.TrackSongCredit{
				{Credit: "composer", Artists: []occipital.CreditArtist{{Name: "Belize Kazi"}, {Name: "Clairo"}}},
				{Credit: "lyricist", Artists: []occipital.CreditArtist{{Name: "Belize Kazi"}}},
			},
			production: []*occipital.TrackProductionCredit{
				{Credit: "producer", Artists: []occipital.CreditArtist{{Name: "Clairo"}, {Name: "leodaleo"}}},
				{Credit: "mix", Artists: []occipital.CreditArtist{{Name: "leodaleo"}}},
				{Credit: "recording", Artists: []occipital.CreditArtist{{Name: "Julio Gómez"}}},
				{Credit: "mastering", Artists: []occipital.CreditArtist{{Name: "Joe LaPorta"}}},
			},
		},
		{
			// Apple Music's 5816 Forest track credits, reviewed 2026-11-09.
			// MusicBrainz recording 39cb8699-66b0-462f-8f36-8298fa11a36a links
			// this Spotify edition and Apple track, with the same ISRC/duration.
			spotifyID: "5EQCcPs2QwFtT45eS1rQps", isrc: "USZHR2400271", title: "yoda", artist: "tech n9ne", duration: 210513,
			source: occipital.ExternalLink{Type: "apple_music", URL: "https://music.apple.com/us/song/yoda/1813191831"},
			songs: []*occipital.TrackSongCredit{
				{Credit: "songwriter", Artists: []occipital.CreditArtist{{Name: "Aaron D. Yates"}, {Name: "John P. Winkler"}, {Name: "Dwayne Michael Carter, Jr."}}},
			},
			production: []*occipital.TrackProductionCredit{
				{Credit: "producer", Artists: []occipital.CreditArtist{{Name: "JPZ"}}},
				{Credit: "mix", Artists: []occipital.CreditArtist{{Name: `Ben "Bengineer" Cybulsky`}}},
				{Credit: "mastering", Artists: []occipital.CreditArtist{{Name: `Ben "Bengineer" Cybulsky`}}},
				{Credit: "assistant recording engineer", Artists: []occipital.CreditArtist{{Name: "Brian Ammon"}}},
				{Credit: "recording", Artists: []occipital.CreditArtist{{Name: "Jimmy Hoyson"}, {Name: "Manny Galvez"}}},
			},
			performance: []*occipital.TrackInstrumentArtists{
				{Instrument: "vocals", Artists: []occipital.CreditArtist{{Name: "Tech N9ne"}, {Name: "Lil Wayne"}}},
				{Instrument: "background vocals", Artists: []occipital.CreditArtist{{Name: "Donnie Yates"}}},
			},
		},
	}
}

func applyReviewedCredits(track *occipital.Track) {
	if track == nil || track.Source != "SPOTIFY" || track.Meta == nil {
		return
	}
	for _, entry := range reviewedCreditEntries() {
		if track.SourceID == entry.spotifyID && track.ISRC == entry.isrc &&
			normalizedName(track.Name) == entry.title && normalizedName(track.Artist) == entry.artist && track.Meta.DurationMs == entry.duration {
			mergeReviewedCredits(track, entry)
			return
		}
	}
}

func mergeReviewedCredits(track *occipital.Track, entry reviewedCredits) {
	songCredits, productionCredits := entry.songs, entry.production
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

	for _, credit := range entry.performance {
		found := false
		for _, existing := range track.Instruments {
			if existing != nil && normalizedName(existing.Instrument) == credit.Instrument {
				found = true
				if len(existing.Artists) == 0 {
					existing.Artists = credit.Artists
					added = true
				}
				break
			}
		}
		if !found {
			track.Instruments = append(track.Instruments, credit)
			added = true
		}
	}
	if added {
		source := entry.source.URL
		for _, link := range track.Links {
			if link.URL == source {
				return
			}
		}
		track.Links = append(track.Links, entry.source)
	}
}
