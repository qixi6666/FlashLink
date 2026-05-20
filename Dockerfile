FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway

FROM alpine:3.20

RUN adduser -D -u 10001 appuser
USER appuser

WORKDIR /app
COPY --from=build /out/gateway /app/gateway

EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
