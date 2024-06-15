dev:
	go mod tidy && go run main.go

test:
	go test ./...

build:
	gcloud builds submit --tag gcr.io/beatbrain-dev/occipital

deploy:
	gcloud run deploy bouncer \
		--image gcr.io/beatbrain-dev/occipital \
		--platform managed

ship:
	make test && make build && make deploy