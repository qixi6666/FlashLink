.PHONY: build test race bench docker-up docker-down run-gateway run-linksvc run-redirectsvc run-statsvc run-worker

build:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

race:
	go test -race ./...

bench:
	go test ./internal/app/linkapp -run '^$$' -bench BenchmarkAsyncShortLinkWriterCreate -benchtime=1s

docker-up:
	docker compose up --build

docker-down:
	docker compose down

run-gateway:
	go run ./cmd/gateway

run-linksvc:
	go run ./cmd/linksvc

run-redirectsvc:
	go run ./cmd/redirectsvc

run-statsvc:
	go run ./cmd/statsvc

run-worker:
	go run ./cmd/worker
