FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/linksvc ./cmd/linksvc
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/redirectsvc ./cmd/redirectsvc
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/statsvc ./cmd/statsvc
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM alpine:3.20

RUN adduser -D -u 10001 appuser
USER appuser

WORKDIR /app
COPY --from=build /out/gateway /app/gateway
COPY --from=build /out/linksvc /app/linksvc
COPY --from=build /out/redirectsvc /app/redirectsvc
COPY --from=build /out/statsvc /app/statsvc
COPY --from=build /out/worker /app/worker

EXPOSE 8080
CMD ["/app/gateway"]
