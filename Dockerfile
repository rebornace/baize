# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/baize ./cmd/baize \
 && go build -o /out/mock-ticket ./examples/mock-ticket/cmd/mock-ticket

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/baize /app/baize
COPY --from=build /out/mock-ticket /app/mock-ticket
COPY configs/docker.yaml /app/configs/docker.yaml
COPY examples/mock-ticket/openapi.yaml /app/examples/mock-ticket/openapi.yaml
EXPOSE 8080
CMD ["/app/baize", "serve", "-config", "/app/configs/docker.yaml"]
