# syntax=docker/dockerfile:1

FROM golang:1.27.3-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags "-s -w" -o /out/baize ./cmd/baize \
 && go build -trimpath -ldflags "-s -w" -o /out/weixin-adapter ./cmd/weixin-adapter

FROM alpine:3.23
RUN apk upgrade --no-cache \
 && apk add --no-cache ca-certificates tzdata \
 && addgroup -S baize \
 && adduser -S -G baize -h /app baize
WORKDIR /app
COPY --from=build --chown=baize:baize /out/baize /app/baize
COPY --from=build --chown=baize:baize /out/weixin-adapter /app/weixin-adapter
COPY --chown=baize:baize configs/docker-minimal.yaml /app/configs/docker-minimal.yaml
COPY --chown=baize:baize configs/docker-weixin-standalone.yaml /app/configs/docker-weixin-standalone.yaml
COPY --chown=baize:baize skills /app/skills
COPY --chown=baize:baize examples/skills /app/examples/skills
RUN mkdir -p /app/data && chown -R baize:baize /app/data
USER baize
EXPOSE 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
CMD ["/app/baize", "serve", "-config", "/app/configs/docker-minimal.yaml"]
