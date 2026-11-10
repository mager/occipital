package track

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mager/occipital/occipital"
	spot "github.com/zmb3/spotify/v2"
	"golang.org/x/net/html"
)

// Public catalog lookup plus the credits visible on Apple's public song page.
// No lyrics, account tokens, artwork, or listening data are retained.
type appleCreditClient struct {
	http *http.Client
	wait func(context.Context) error
}
type appleCatalogTrack struct {
	ID       int64  `json:"trackId"`
	Name     string `json:"trackName"`
	Artist   string `json:"artistName"`
	Album    string `json:"collectionName"`
	Duration int    `json:"trackTimeMillis"`
	Rating   string `json:"trackExplicitness"`
}

var applePace struct {
	sync.Mutex
	next time.Time
}

func waitApple(ctx context.Context) error {
	applePace.Lock()
	delay := time.Until(applePace.next)
	if delay < 0 {
		delay = 0
	}
	if deadline, ok := ctx.Deadline(); ok && time.Now().Add(delay).After(deadline) {
		applePace.Unlock()
		return context.DeadlineExceeded
	}
	applePace.next = time.Now().Add(delay + 3*time.Second)
	applePace.Unlock()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func newAppleCreditClient() *appleCreditClient {
	return &appleCreditClient{wait: waitApple, http: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Host != "music.apple.com" {
			return fmt.Errorf("unexpected credits redirect")
		}
		return nil
	}}}
}
func (c *appleCreditClient) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Beatbrain/1.6 (https://github.com/mager/occipital)")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credit source request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("credit source status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 2*1024*1024 {
		return nil, fmt.Errorf("credit source response too large")
	}
	return body, nil
}
func (c *appleCreditClient) lookup(ctx context.Context, track *spot.FullTrack) (*reviewedCredits, error) {
	if track == nil || len(track.Artists) == 0 || track.Duration == 0 {
		return nil, nil
	}
	if c.wait != nil {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
	}
	query := url.Values{"term": {track.Artists[0].Name + " " + track.Name}, "entity": {"song"}, "limit": {"20"}, "country": {"US"}}
	body, err := c.get(ctx, "https://itunes.apple.com/search?"+query.Encode())
	if err != nil {
		return nil, err
	}
	match, err := selectAppleTrack(body, track)
	if err != nil {
		return nil, err
	}
	if match == nil {
		return c.lookupSingleAcrossAlbums(ctx, body, track)
	}
	page, err := c.get(ctx, "https://music.apple.com/us/song/"+strconv.FormatInt(match.ID, 10))
	if err != nil {
		return nil, err
	}
	return parseAppleCredits(page, *match, track)
}
func albumKey(s string) string {
	s = normalizedName(s)
	for _, suffix := range []string{" - single", " - ep"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}
func combinedArtistKey(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer(",", " ", "&", " ").Replace(normalizedName(s))), " ")
}
func selectAppleTrack(body []byte, track *spot.FullTrack) (*appleCatalogTrack, error) {
	var result struct {
		Results []appleCatalogTrack `json:"results"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Results == nil {
		return nil, fmt.Errorf("invalid catalog response")
	}
	var match *appleCatalogTrack
	names := []string{}
	for _, artist := range track.Artists {
		names = append(names, artist.Name)
	}
	for _, candidate := range result.Results {
		delta := candidate.Duration - int(track.Duration)
		if delta < 0 {
			delta = -delta
		}
		rating := candidate.Rating == "explicit" && track.Explicit || candidate.Rating == "notExplicit" && !track.Explicit
		if candidate.ID <= 0 || normalizedName(candidate.Name) != normalizedName(track.Name) || albumKey(candidate.Album) != albumKey(track.Album.Name) || combinedArtistKey(candidate.Artist) != combinedArtistKey(strings.Join(names, ", ")) || candidate.Duration <= 0 || delta > 1000 || !rating {
			continue
		}
		if match != nil && match.ID != candidate.ID {
			return nil, nil
		}
		copy := candidate
		match = &copy
	}
	return match, nil
}
func parseAppleCredits(body []byte, match appleCatalogTrack, track *spot.FullTrack) (*reviewedCredits, error) {
	var page struct {
		Data []struct {
			Intent struct {
				ContentDescriptor struct {
					Identifiers struct {
						ID string `json:"storeAdamID"`
					} `json:"identifiers"`
				} `json:"contentDescriptor"`
			} `json:"intent"`
			Data struct {
				URL      string `json:"canonicalURL"`
				Sections []struct {
					ID    string `json:"id"`
					Items []struct {
						Title   string   `json:"title"`
						Album   string   `json:"album"`
						Name    string   `json:"name"`
						Roles   []string `json:"roleNames"`
						Artists []struct {
							Title string `json:"title"`
						} `json:"artistLinks"`
						Descriptor struct {
							Identifiers struct {
								ID string `json:"storeAdamID"`
							} `json:"identifiers"`
						} `json:"contentDescriptor"`
					} `json:"items"`
				} `json:"sections"`
			} `json:"data"`
		} `json:"data"`
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(body))
	found := false
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			break
		}
		if kind != html.StartTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data != "script" {
			continue
		}
		target := false
		for _, a := range token.Attr {
			if a.Key == "id" && a.Val == "serialized-server-data" {
				target = true
			}
		}
		if target && tokenizer.Next() == html.TextToken {
			if err := json.Unmarshal(tokenizer.Text(), &page); err != nil {
				return nil, fmt.Errorf("invalid credits page")
			}
			found = true
			break
		}
	}
	if !found || len(page.Data) != 1 {
		return nil, fmt.Errorf("missing credits page data")
	}
	data := page.Data[0]
	id := strconv.FormatInt(match.ID, 10)
	if data.Intent.ContentDescriptor.Identifiers.ID != id {
		return nil, fmt.Errorf("credits page ID mismatch")
	}
	source, err := url.Parse(data.Data.URL)
	if err != nil || source.Scheme != "https" || source.Host != "music.apple.com" || !strings.HasSuffix(source.Path, "/"+id) || source.RawQuery != "" {
		return nil, fmt.Errorf("invalid credits source")
	}
	verified := false
	expected := []string{}
	for _, artist := range track.Artists {
		expected = append(expected, normalizedName(artist.Name))
	}
	sort.Strings(expected)
	for _, section := range data.Data.Sections {
		if section.ID != "songDetail" || len(section.Items) != 1 {
			continue
		}
		header := section.Items[0]
		artists := []string{}
		for _, artist := range header.Artists {
			artists = append(artists, normalizedName(artist.Title))
		}
		sort.Strings(artists)
		verified = header.Descriptor.Identifiers.ID == id && normalizedName(header.Title) == normalizedName(track.Name) && albumKey(header.Album) == albumKey(match.Album) && strings.Join(artists, "\x00") == strings.Join(expected, "\x00")
	}
	if !verified {
		return nil, fmt.Errorf("credits page identity mismatch")
	}
	result := &reviewedCredits{source: occipital.ExternalLink{Type: "apple_music", URL: source.String()}}
	for _, section := range data.Data.Sections {
		if section.ID != "composer-and-lyrics" && section.ID != "production-and-engineering" && section.ID != "performer" {
			continue
		}
		for _, person := range section.Items {
			name := strings.TrimSpace(person.Name)
			if name == "" || len(name) > 200 {
				continue
			}
			for _, rawRole := range person.Roles {
				role := normalizedName(rawRole)
				if role == "" || len(role) > 100 {
					continue
				}
				switch role {
				case "lyrics":
					role = "lyricist"
				case "mixing engineer":
					role = "mix"
				case "mastering engineer":
					role = "mastering"
				case "recording engineer":
					role = "recording"
				}
				artist := occipital.CreditArtist{Name: name}
				switch section.ID {
				case "composer-and-lyrics":
					result.songs = appendSongArtist(result.songs, role, artist)
				case "production-and-engineering":
					result.production = appendProductionArtist(result.production, role, artist)
				case "performer":
					result.performance = appendPerformanceArtist(result.performance, role, artist)
				}
			}
		}
	}
	return result, nil
}
func appendSongArtist(rows []*occipital.TrackSongCredit, role string, artist occipital.CreditArtist) []*occipital.TrackSongCredit {
	for _, row := range rows {
		if row.Credit == role {
			for _, a := range row.Artists {
				if a.Name == artist.Name {
					return rows
				}
			}
			row.Artists = append(row.Artists, artist)
			return rows
		}
	}
	return append(rows, &occipital.TrackSongCredit{Credit: role, Artists: []occipital.CreditArtist{artist}})
}
func appendProductionArtist(rows []*occipital.TrackProductionCredit, role string, artist occipital.CreditArtist) []*occipital.TrackProductionCredit {
	for _, row := range rows {
		if row.Credit == role {
			for _, a := range row.Artists {
				if a.Name == artist.Name {
					return rows
				}
			}
			row.Artists = append(row.Artists, artist)
			return rows
		}
	}
	return append(rows, &occipital.TrackProductionCredit{Credit: role, Artists: []occipital.CreditArtist{artist}})
}
func appendPerformanceArtist(rows []*occipital.TrackInstrumentArtists, role string, artist occipital.CreditArtist) []*occipital.TrackInstrumentArtists {
	for _, row := range rows {
		if row.Instrument == role {
			for _, a := range row.Artists {
				if a.Name == artist.Name {
					return rows
				}
			}
			row.Artists = append(row.Artists, artist)
			return rows
		}
	}
	return append(rows, &occipital.TrackInstrumentArtists{Instrument: role, Artists: []occipital.CreditArtist{artist}})
}

// A single can leave the catalog after its album arrives. Accept that case only
// when two distinct album editions independently expose identical named credits.
// Never relax title, all-artists, duration, rating, or page identity checks.
func (c *appleCreditClient) lookupSingleAcrossAlbums(ctx context.Context, body []byte, track *spot.FullTrack) (*reviewedCredits, error) {
	if track.Album.AlbumType != "single" || albumKey(track.Album.Name) != normalizedName(track.Name) {
		return nil, nil
	}
	var catalog struct {
		Results []appleCatalogTrack `json:"results"`
	}
	if json.Unmarshal(body, &catalog) != nil {
		return nil, fmt.Errorf("invalid catalog response")
	}
	candidates := []appleCatalogTrack{}
	seen := map[int64]bool{}
	for _, item := range catalog.Results {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		// An ambiguous exact-album result must not be replaced by weaker evidence.
		probe := *track
		probe.Album = track.Album
		probe.Album.Name = item.Album
		candidateJSON, _ := json.Marshal(struct {
			Results []appleCatalogTrack `json:"results"`
		}{[]appleCatalogTrack{item}})
		match, err := selectAppleTrack(candidateJSON, &probe)
		if err != nil {
			return nil, err
		}
		if match == nil {
			continue
		}
		if albumKey(item.Album) == albumKey(track.Album.Name) {
			return nil, nil
		}
		candidates = append(candidates, *match)
	}
	if len(candidates) != 2 || albumKey(candidates[0].Album) == albumKey(candidates[1].Album) {
		return nil, nil
	}
	var result *reviewedCredits
	signature := ""
	for _, match := range candidates {
		page, err := c.get(ctx, "https://music.apple.com/us/song/"+strconv.FormatInt(match.ID, 10))
		if err != nil {
			return nil, err
		}
		credits, err := parseAppleCredits(page, match, track)
		if err != nil {
			return nil, err
		}
		current := creditSignature(credits)
		if current == "" {
			return nil, nil
		}
		if result == nil {
			result = credits
			signature = current
		} else if current != signature {
			return nil, nil
		}
	}
	return result, nil
}
func creditSignature(credits *reviewedCredits) string {
	rows := []string{}
	for _, row := range credits.songs {
		for _, artist := range row.Artists {
			rows = append(rows, "song:"+row.Credit+":"+artist.Name)
		}
	}
	for _, row := range credits.production {
		for _, artist := range row.Artists {
			rows = append(rows, "production:"+row.Credit+":"+artist.Name)
		}
	}
	for _, row := range credits.performance {
		for _, artist := range row.Artists {
			rows = append(rows, "performance:"+row.Instrument+":"+artist.Name)
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\x00")
}
