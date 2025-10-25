# syntax=docker/dockerfile:1.7

# Build stage
FROM golang:1.22-alpine AS build
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /src
COPY go.mod ./
COPY go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags "-s -w" -o /out/app ./cmd/app

# Runtime stage
FROM alpine:3.20
ENV GIN_MODE=release
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -H -s /sbin/nologin appuser
USER appuser
WORKDIR /home/appuser
COPY --from=build /out/app /usr/local/bin/app
EXPOSE 8080
# LEADERBOARD_DB_URL env must be provided at runtime; optional LEADERBOARD_ADDR, LEADERBOARD_TRUSTED_IP_HEADER, LEADERBOARD_PAGE_SIZE_DEFAULT
ENTRYPOINT ["/usr/local/bin/app"]
