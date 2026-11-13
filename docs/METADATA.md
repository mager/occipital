# Music metadata sources

## Spotify public catalog

The track endpoint supplies all credited display artists, album, duration, ISRC, explicit flag, disc and track numbers. The album endpoint adds label, copyrights and total tracks. The older SDK omits some album fields, so `spotify/catalog.go` decodes them with the same OAuth client.

Spotify's public track response does not supply songwriter, producer or engineer credits. Audio analysis/features are opt-in (`OCCIPITAL_SPOTIFY_AUDIO_ENABLED=true`); unavailable access must not turn zero values into musical claims. Personal taste/history would require new user OAuth scopes and is outside this release.

- https://developer.spotify.com/documentation/web-api/reference/get-track
- https://developer.spotify.com/documentation/web-api/reference/get-an-album
- https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide

## MusicBrainz

Use direct Spotify links, recording identifiers and conservative catalog matching. Load recording relationships and all linked works; verify returned IDs before adding writers. Community coverage varies. A matched recording does not imply complete credits.

- https://musicbrainz.org/doc/MusicBrainz_API

## Apple Music public credits

An iTunes catalog search must match title, all artists, album, duration within one second, and explicit rating. Ambiguous results are rejected. If a same-name single has left the catalog, exactly two different album editions can supply credits only when their title, all artists, duration and rating match and every named credit is identical. The page identity is checked against each catalog album. The public song page must agree on ID, title, album and artists. Only visible named writing, performance and production sections are retained, with a canonical source link. This is a public-page parser, not a documented credits API; schema changes are failures that remain retryable. No lyrics, account tokens or listening data are collected.

Requests use timeouts, cancellation, limited response sizes and a per-instance search pace of 20/minute. Verified source gaps retry after one hour; successful catalog results cache for seven days. Provider failures do not get that normal cache lifetime.

- https://performance-partners.apple.com/search-api
- https://music.apple.com/us/song/save-my-love/1876235280

## Discovery

Melodex collects catalog candidates; Occipital ranks source appearances. Apple Music's public US Top 50 replaces the inaccessible Reddit source. This is a US popularity signal, not a worldwide play-count estimate. The room and For you continue using private Beatbrain activity. Detailed MusicBrainz enrichment belongs on track pages, not in daily ingestion.

- https://performance-partners.apple.com/tools
- https://rss.marketingtools.apple.com/api/v2/us/music/most-played/50/songs.json

## Song connections and studios (2026-11-13)

Recording lookup now includes `recording-rels` and `place-rels` in the existing bounded request. Track responses expose `connections` for samples/remixes/edits and `studios` for recorded/mixed/produced/engineered places, with exact IDs, direction, optional area/date precision, and additional recording context. Cache schema is 10. Direct MBID navigation accepts canonical IDs after a MusicBrainz merge and avoids server-side cover-art fanout.

Cloud Run uses `GODEBUG=tlsmlkem=0`. The default Go 1.24 hybrid key exchange caused MusicBrainz TLS handshakes to end with EOF from Cloud Run. A tagged revision with this documented Go compatibility setting returned the real recording and 29 connections; it was verified before traffic moved. Certificate verification, HTTPS, and the existing MusicBrainz IPv4 setting remain enabled. Retain this environment setting on future deployments unless a live handshake check proves it is no longer needed.

The service has explicitly pinned traffic. A successful deploy alone does not move users: verify the image digest and revision, then update traffic to the verified revision in `beatbrain-dev`, `us-central1`.
