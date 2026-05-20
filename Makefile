.PHONY: test race run-gateway run-linksvc run-redirectsvc run-statsvc run-worker

test:
	go test ./...

race:
	go test -race ./...

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
