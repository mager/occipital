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

- `gcloud iam service-accounts create local-dev` - Create service account
- `gcloud projects add-iam-policy-binding beatbrain-dev --member="serviceAccount:local-dev@beatbrain-dev.iam.gserviceaccount.com" --role="roles/owner"` - Create policy
- `gcloud iam service-accounts keys create credentials.json --iam-account=local-dev@beatbrain-dev.iam.gserviceaccount.com` - Create keys


## Documentation

### Dependencies

- [mager/musicbrainz-go](https://github.com/mager/musicbrainz-go)

### Endpoints

#### GET /track?source&spotifyId

- Call Spotify with track ID to get the ISRC
- Call Musicbrainz SearchRecordingsByISRC endpoint to get the recording