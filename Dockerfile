# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder

ENV CGO_ENABLED=0 \
    GOTOOLCHAIN=auto

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -trimpath -ldflags="-s -w" -o /out/rms-server ./cmd/rms-server \
 && go build -trimpath -ldflags="-s -w" -o /out/rms-init   ./cmd/rms-init

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app

COPY --from=builder /out/rms-server /app/rms-server
COPY --from=builder /out/rms-init   /app/rms-init

ENV CONFIG_PATH=/app/config/local.yaml

EXPOSE 8082

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=6 \
    CMD curl -s -o /dev/null "http://localhost:8082/usertokenvalid?token=healthcheck" || exit 1

ENTRYPOINT ["/app/rms-server"]
