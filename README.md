# occipital

## Development

To start the local dev server, run `go run main.go` which is aliased:

```
make dev
```

## Google Cloud Setup

- `gcloud projects create beatbrain-dev` - Create a new project
- `gcloud builds submit --tag gcr.io/beatbrain-dev/occipital` - Build and submit to Google Container Registry
- `gcloud run deploy occipital --image gcr.io/beatbrain-dev/occipital --platform managed` - Deploy to Cloud Run

## Setup locally

Google Cloud service credentials:

- `gcloud iam service-accounts create local-dev` - Create service account
- `gcloud projects add-iam-policy-binding beatbrain-dev --member="serviceAccount:local-dev@beatbrain-dev.iam.gserviceaccount.com" --role="roles/owner"` - Create policy
- `gcloud iam service-accounts keys create credentials.json --iam-account=local-dev@beatbrain-dev.iam.gserviceaccount.com` - Create keys

Neon db:

- https://console.neon.tech/app/projects/shiny-scene-55026371/branches/br-billowing-river-a529vdsd/computes?branchId=br-billowing-river-a529vdsd&database=users

Spotify creds:

- https://developer.spotify.com/dashboard/1b23a4171fa44ebda15488b3a26079a0

## Documentation

### Dependencies

- [mager/musicbrainz-go](https://github.com/mager/musicbrainz-go)

## Generate OpenAPI spec

```
go install github.com/swaggo/swag/cmd/swag@latest
npm i -g openapi-to-postmanv2
make openapi
```

### Endpoints

#### GET /track?source&spotifyId

- Call Spotify with track ID to get the ISRC
- Call Musicbrainz SearchRecordingsByISRC endpoint to get the recording
## Private API deployment (November 2026)

Set `BEATBRAIN_API_KEY` in Cloud Run and the trusted frontend server. All paths except `GET /health` require the `X-Beatbrain-Key` header. An unset key rejects data requests. The public health response contains only `{"server":true}`.

The active API serves search, track metadata, creators, recommendations, genres, discovery, and podcasts. The old `/user`, `/profile`, Spotify playback/OAuth, and `/np` routes are removed. The unused featured-tracks handler is also removed. Social profiles and account access remain in the frontend service. No database records are changed.

Occipital no longer opens a PostgreSQL connection or reads `OCCIPITAL_DATABASEURL`, `OCCIPITAL_NEXTAUTHSECRET`, or `OCCIPITAL_SPOTIFYREDIRECTURL`. Existing unused deployment variables can be removed separately. Spotify catalog credentials and Firestore access are still required. Keep `/genre/tracks` for the separate chat app; its server must also send the API key.

Build and check with `go test ./...`, `go vet ./...`, and `CGO_ENABLED=0 go build -mod=readonly .`. After deployment, check health, rejection without a key, search with the key, and HTTP 404 for retired routes with the key. Requests without the key return HTTP 401 even for retired routes.

### Credit lookup states

The v2 track response includes `metadata_status`: `matched`, `unmatched`, or `unavailable`. An upstream connection or work lookup failure is `unavailable` and must not be cached as a completed lookup. A recording match does not guarantee that MusicBrainz contains detailed credits.

`handler/track/supplemental_credits.go` contains separately reviewed credits for exact Spotify editions. Each entry has a source URL, review date, ISRC, title, primary artist, and duration. It fills missing roles only and preserves existing populated roles. The Yoda entry was verified against Apple Music on 2026-11-09. This data does not resolve the observed Cloud Run → MusicBrainz TLS connection failures; that service availability issue remains separate.

The v2 matcher first looks up the exact Spotify URL in MusicBrainz. A unique, current audio recording link must also agree on title (allowing explicit remaster/credited-guest suffixes), duration within two seconds, and any recorded ISRC. Conflicting links are rejected. When the URL is absent (404), the existing strict ISRC/name matcher is used. Recording requests include `work-level-rels`, and the local decoder retains embedded work relationships so songwriting normally needs no separate request. The overall eight-second request deadline remains in place.
