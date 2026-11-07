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
