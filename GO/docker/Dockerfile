# --- 构建阶段 ---
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -mod=vendor -ldflags="-s -w" -o /maucache ./cmd/maucache

# --- 运行阶段 ---
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /maucache /usr/local/bin/maucache

VOLUME ["/data/maucache"]
ENTRYPOINT ["maucache"]
