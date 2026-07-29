# Webcam Dashboard

A single static **Go** binary that serves a **React** SPA — one pane to control
**cameras (Frigate/go2rtc)**, **3D printers (Moonraker / OctoPrint / Bambu)**,
**container health (via docker-socket-proxy)**, and **push notifications (ntfy)**.
Real-time over one WebSocket. Cross-compiles to arm64 for a Pi.

> **Build status:** the **foundation** is in place and runs end-to-end with a demo
> data source. Real device adapters land in phases — see *Roadmap* below.

## Quick start (with the rest of the stack)

```sh
docker compose -f docker-compose.yml -f compose.nvidia.yml up -d --build dashboard
```

Open `http://<host>:8088` over the tailnet. **No config file is required** — add
printers and cameras from the UI (they persist to `dashboard/data/devices.yaml`).
On first run the dashboard writes a commented `dashboard/data/config.yaml` for the
settings the UI doesn't cover; `config.example.yaml` here is the fully worked
version to copy from. With no printers configured (or `DASHBOARD_DEMO=1`) it shows
a **demo printer + camera tiles** so you can verify the UI before wiring real
devices. Camera tiles stream live from go2rtc via the proxy (MJPEG by default;
vendor go2rtc's player for WebRTC — see `web/public/README.md`).

## Local development

**One command** (builds the SPA, embeds it, runs in demo mode on `:8088`):

```sh
./dashboard/run-local.sh        # http://127.0.0.1:8088
```

No Go? Install it without root first:

```sh
curl -fsSL https://go.dev/dl/go1.23.4.linux-amd64.tar.gz | tar -C "$HOME/.local" -xz
export PATH="$HOME/.local/go/bin:$PATH"
```

Hot-reload frontend loop instead (Vite dev server proxies `/api,/ws,/go2rtc,/frigate` → `:8088`):

```sh
cd dashboard && DASHBOARD_DEMO=1 go run ./cmd/dashboard -config config.yaml   # terminal 1
cd dashboard/web && npm install && npm run dev                                # terminal 2 → :5173
```

> **Verified locally (2026-06-04):** `go vet`/`go build` clean, frontend build +
> typecheck clean, demo printer animates, the WS round-trip works (clicking Pause
> flips the printer to `paused` and reconciles the buttons). Camera tiles show
> live only when a real go2rtc is reachable (they 502 in standalone demo).

Production build (what the Docker image does): `npm run build` emits `web/dist`,
which the Go build embeds via `//go:embed all:dist`.

## Architecture

```
adapters ──Publish──▶ bus ──▶ hub ──WS──▶ React SPA
(frigate, moonraker,        │
 octoprint, bambu, docker)  └──▶ automation ──▶ ntfy
```

- **internal/bus** — non-blocking in-process pub/sub; a slow subscriber is dropped, never stalls a producer.
- **internal/adapter** — the wire contracts (`Printer`, `CameraState`, `Container`, `Alert`), mirrored in `web/src/types/ws.ts`.
- **internal/hub** — caches state, fans deltas to clients over one typed WS, routes commands back to adapters.
- **internal/web** — `embed.FS` SPA + reverse proxies (`/go2rtc/*` with WS upgrade, `/frigate/*` media) + REST mirror.
- **internal/adapters/** — one package per integration (added per phase).

## Configuration

Optional. The binary resolves its config in this order:

1. `-config <path>` or `DASHBOARD_CONFIG`
2. `<data_dir>/config.yaml` — i.e. `/data/config.yaml` in the container,
   `dashboard/data/config.yaml` on the host (written, commented, on first run)
3. `/config.yaml` — the legacy single-file mount, still read so older stacks keep
   working. New installs should not add it: Docker turns a bind-mounted file that
   doesn't exist on the host into a *directory*, which is why config moved into the
   data directory. A directory at either path is skipped, not loaded.

Nothing here is required to boot — missing, unreadable, or empty all fall back to
defaults, and devices can be added from the UI. A *malformed* file fails loudly.
Copy `config.example.yaml` for a fully worked reference. Secrets can also come from
env: `DASHBOARD_TOKEN`, `DOCKER_HOST`, `NTFY_BASE_URL`, `NTFY_TOKEN`. Restart to apply.

## Security

- Designed for **Tailscale-only** access; auth is light by default. Set
  `DASHBOARD_TOKEN` to require a bearer token on the API + ntfy action buttons.
- **No raw Docker socket.** The dashboard talks to **docker-socket-proxy**
  (`CONTAINERS=1, POST=1` only); container restart is allow-listed to the
  configured `watch` names.
- Frigate `:5000`, go2rtc `:1984`, and Mosquitto are unauthenticated — never expose
  them publicly; the dashboard re-proxies their media behind its own origin.
- Bambu control needs LAN + Developer Mode; until then the panel is read-only with
  a "Developer Mode required" badge (the `ControlBlocked` flag — flip with no code change).

## Roadmap (build phases)

1. ✅ **Foundation** — bus, WS hub, config, embed/proxy serving, React shell (cameras + printers + health + alerts), demo source, packaging.
2. ⬜ **Frigate adapter** — MQTT detect/record toggles, clips/alerts, live camera enumeration.
3. ⬜ **Printer adapters** — Moonraker, OctoPrint, Bambu behind the `Printer` interface.
4. ⬜ **Health + notifications** — docker-socket-proxy poller, ntfy rule engine.
