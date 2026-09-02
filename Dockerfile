# syntax=docker/dockerfile:1

# ── Stage 1: Builder ──
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o switchblade ./cmd/switchblade

# ── Stage 2: Runtime ──
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /build/switchblade /switchblade

USER nonroot:nonroot

EXPOSE 1930 1931

ENTRYPOINT ["/switchblade"]
