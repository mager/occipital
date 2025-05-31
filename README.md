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

## Generate OpenAPI

```
go install github.com/swaggo/swag/cmd/swag@latest
npm i -g openapi-to-postmanv2
make openapi
```

### Endpoints

#### GET /track?source&spotifyId

- Call Spotify with track ID to get the ISRC
- Call Musicbrainz SearchRecordingsByISRC endpoint to get the recording