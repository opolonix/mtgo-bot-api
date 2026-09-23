# syntax=docker/dockerfile:1.7
FROM golang:1.26-bookworm AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN --mount=type=cache,id=telefeeds-go-mod,target=/go/pkg/mod,sharing=locked \
    go mod download
COPY . .
RUN --mount=type=cache,id=telefeeds-go-mod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=telefeeds-botapi-build,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/telefeeds-bot-api ./cmd/telefeeds-bot-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/telefeeds-bot-api /usr/local/bin/telefeeds-bot-api
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/telefeeds-bot-api"]
