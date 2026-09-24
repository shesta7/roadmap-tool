FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/roadmap-tool ./cmd/roadmap

FROM alpine:3.22

RUN apk add --no-cache ca-certificates \
    && addgroup -S roadmap \
    && adduser -S -G roadmap roadmap \
    && mkdir -p /data \
    && chown roadmap:roadmap /data

COPY --from=build /out/roadmap-tool /usr/local/bin/roadmap-tool
COPY --chown=roadmap:roadmap config.example.json /data/config.json

USER roadmap
EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/api/config || exit 1

ENTRYPOINT ["roadmap-tool"]
CMD ["-config", "/data/config.json", "-secrets", "/data/secrets.json", "-listen", "0.0.0.0:8080"]
