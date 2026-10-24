# Architecture

## The stack

```
                     browser / phone
                           |
                  dashboard :8088   (Go binary + embedded React app)
        +--------+---------+-----------+-------------+
        |        |         |           |             |
   Frigate    go2rtc    printers    docker-socket   ntfy :8092 ---> phone
   :5000      :1984     (LAN)       -proxy :2375
   (+MQTT)  (inside Frigate)         (loopback)
        |
   Mosquitto :1883 (loopback)  <--- Home Assistant (optional)
```

| Service | Container | Role |
|---|---|---|
| Frigate | `frigate` | NVR, detection, and the bundled go2rtc restream. Host networking |
| Mosquitto | `mosquitto` | MQTT bus between Frigate, the dashboard and Home Assistant |
| Dashboard | `dashboard` | UI and API. Host networking, so it reaches Frigate, go2rtc, Mosquitto and printers |
| ntfy | `ntfy` | Self-hosted push notifications. Host networking |
| docker-socket-proxy | `docker-socket-proxy` | Least-privilege Docker API for health and restart |
| Tailscale (optional) | `tinkerview-tailscale` | Publishes the dashboard on your tailnet |

Why host networking: Frigate needs it for low-latency WebRTC (port 8555 over TCP and UDP)
and for ONVIF discovery. The cost is that Frigate shares the host's loopback, so a port
already taken on the host crash-loops it. `compose.bridge.yml` trades those for published
ports.

## Ports

[SECURITY.md](../SECURITY.md#what-listens-where) has the authoritative table, with what
each port is bound to. Summary: 8088 dashboard, 8092 ntfy, 5000 Frigate (unauthenticated),
8971 Frigate (authenticated), 1984 go2rtc, 8554 RTSP restream, 8555 WebRTC, 1883 Mosquitto
(loopback), 1884 Mosquitto with passwords (optional), 2375 docker-socket-proxy (loopback).

## Data layout

| Path (relative to the install folder) | Contents | Owner |
|---|---|---|
| `.env` | Settings and secrets | you |
| `frigate/` | `config.yml`, Frigate's database, model cache. Mounted at `/config` in the container | Frigate |
| `dashboard/data/` | `config.yaml`, `devices.yaml`, `notify-rules.json`, Frigate config backups. Mounted at `/data` | uid 65532 |
| `STORAGE_PATH` (default `storage/frigate`) | Recordings, clips, snapshots. Mounted at `/media/frigate`, read-only for the dashboard | Frigate |
| `ntfy/` | ntfy's user database | ntfy |
| `mosquitto/` | `config/` (read-only), `data/` (including the optional `passwd`), `log/` | mosquitto |
| `tailscale/` | `serve.json` and `state/` | tailscale |
| `storage/install-backups/` | Files the installer backed up before changing | you |

## The dashboard

One Go binary that embeds the built React app (`//go:embed`). It runs as a distroless,
non-root container (uid 65532), listens on `:8088` and answers `GET /healthz`. The image is
built in three stages (Node 22 for the web app, Go 1.26 for the binary, then distroless) and
published for amd64 and arm64.

```
adapters --Publish--> bus --> hub --WebSocket--> React app
(frigate, moonraker,     |
 octoprint, prusalink,   +--> automation --> ntfy
 bambu, docker)
```

- **`internal/bus`**: non-blocking in-process publish and subscribe. A slow subscriber is
  dropped, never stalls a producer.
- **`internal/adapter`**: the wire contracts (`Printer`, `CameraState`, `Container`,
  `Alert`), mirrored in `web/src/types/ws.ts`.
- **`internal/adapters/*`**: one package per integration: Frigate (MQTT and HTTP), go2rtc,
  Moonraker, OctoPrint, PrusaLink, Bambu, Docker, and a fake source for demo mode. Shared
  logic (clamping, home G-code, macro allowlisting) is in `printerutil`.
- **`internal/hub`**: caches state, fans deltas to clients over one typed WebSocket, and
  routes commands back to the adapters.
- **`internal/web`**: the embedded SPA, plus reverse proxies for go2rtc (`/go2rtc/`, read-only,
  WebSocket upgrades) and Frigate (`/frigate/`). Browsers only ever talk to port 8088. It
  also enforces the token, same-origin checks on mutating requests and WebSocket handshakes,
  and sets `nosniff`, framing and referrer headers.
- **`internal/camera`** and **`internal/devices`**: UI-managed cameras and printers, stored
  in `devices.yaml`, with edits applied live to go2rtc and (for promoted cameras) to
  Frigate's config through its API.
- **`internal/rules`** and **`internal/automation`**: notification rules and the logic that
  turns events into ntfy messages.

Camera rotate and flip are applied at the go2rtc source, so the live tile, recordings,
snapshots and detection stay in agreement.

## Trust boundaries

- The network is the perimeter; see [SECURITY.md](../SECURITY.md).
- The dashboard never touches the Docker socket; it talks to docker-socket-proxy.
- The dashboard container is passed only the variables it needs, not `.env`.
- go2rtc `exec:` sources are off by default and refused by the dashboard.

## Developing

`./dashboard/run-local.sh` builds the web app, embeds it and serves it in demo mode on
`http://127.0.0.1:8088`. For hot reload, run `DASHBOARD_DEMO=1 go run ./cmd/dashboard -config
config.yaml` in `dashboard/` and `npm run dev` in `dashboard/web/` (Vite on `:5173`, proxying
`/api`, `/ws`, `/go2rtc` and `/frigate`; `TV_BACKEND` changes the target). See
[dashboard/README.md](../dashboard/README.md) and [CONTRIBUTING.md](../CONTRIBUTING.md).
