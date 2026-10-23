# syntax=docker/dockerfile:1
# Multi-stage, multi-arch: builds the React SPA, cross-compiles the Go binary,
# and ships a tiny static distroless image. Build for your hosts with:
#   docker buildx build --platform linux/amd64,linux/arm64 -t tinkerview:latest ./dashboard
# (or just `docker compose build dashboard` for the local arch).

# --- stage 1: build the React SPA ---
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build          # -> /web/dist (vendored go2rtc player included if present)

# --- stage 2: build the Go binary (pure cross-compile via BUILDPLATFORM) ---
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY . .
# Inject the built SPA so `//go:embed all:dist` picks up the real assets.
COPY --from=web /web/dist ./internal/web/dist
# Dependencies are pinned by the committed go.mod + go.sum; -mod=readonly makes
# the build fail loudly if they drift instead of silently rewriting them.
RUN go mod download
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -mod=readonly -trimpath -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/dashboard ./cmd/dashboard

# --- stage 3: runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/dashboard /dashboard
EXPOSE 8088
# Distroless has no shell or curl, so the binary probes its own /healthz.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD ["/dashboard", "-healthcheck"]
# No -config: the binary resolves it (<data_dir>/config.yaml, else the legacy
# /config.yaml mount). Passing it explicitly would pin old installs to the legacy
# path forever. Override with DASHBOARD_CONFIG if you mount it somewhere else.
ENTRYPOINT ["/dashboard"]
