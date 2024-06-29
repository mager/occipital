dev:
	go mod tidy && go run main.go

test:
	go test -v ./...

build:
	gcloud builds submit --tag gcr.io/beatbrain-dev/occipital

deploy:
	gcloud run deploy occipital \
		--image gcr.io/beatbrain-dev/occipital \
		--platform managed

ship:
	make test && make build && make deploy

openapi:
	swag init --parseDependency --parseInternal
	openapi2postmanv2 -s docs/swagger.yaml -o docs/postman.json