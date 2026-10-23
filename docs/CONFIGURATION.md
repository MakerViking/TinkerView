# Configuration

TinkerView has three layers of configuration. Most day-to-day changes (adding a printer
or camera, editing notification rules, picking the detection hardware) are done in the
dashboard itself. The rest is below.

| Layer | File | Edited by |
|---|---|---|
| Stack: images, ports, secrets, storage | `.env` | you, or `install.sh` |
| Video: cameras, go2rtc streams, retention, detector | `frigate/config.yml` | you, `install.sh` (preset block), and the dashboard for cameras you promote or edit |
| Dashboard: connections, auth, notifications | `dashboard/data/config.yaml` | you |
| Devices added in the UI | `dashboard/data/devices.yaml` | the dashboard |

`.env`, `dashboard/data/`, `frigate/` and `ntfy/` hold your settings, credentials and
state. Back them up and treat the backups as secrets.

## `.env`

Copy `.env.example` to `.env` and edit. The file is gitignored. Compose reads it for every
`${VARIABLE}` in the compose files, but neither container gets the whole file. The dashboard
is passed only the variables marked "(dashboard)" below, so camera passwords do not reach it.
Frigate is passed only `TZ`, `FRIGATE_RTSP_USER`, `FRIGATE_RTSP_PASSWORD`,
`FRIGATE_HOST_LAN_IP` and `GO2RTC_ALLOW_ARBITRARY_EXEC`, so `DASHBOARD_TOKEN`, `NTFY_TOKEN`
and `TS_AUTHKEY` are not in its environment. To use another `{FRIGATE_...}` variable in
`frigate/config.yml`, add it to the `frigate: environment:` list in `docker-compose.yml`
(or to a `docker-compose.override.yml`).

| Variable | Default | Meaning |
|---|---|---|
| `FRIGATE_IMAGE` | `ghcr.io/blakeblackshear/frigate:0.17.1` | Frigate image. Nvidia GPU detection needs the `-tensorrt` variant |
| `FRIGATE_IMAGE_ROCM` | `...:0.17.1-rocm` | Image used by `compose.amd-rocm.yml` |
| `MOSQUITTO_IMAGE` | `eclipse-mosquitto:2.0.22` | Broker image |
| `NTFY_IMAGE` | `binwiederhier/ntfy:v2.11.0` | ntfy image |
| `DOCKERPROXY_IMAGE` | `tecnativa/docker-socket-proxy:0.3.0` | Docker proxy image |
| `TAILSCALE_IMAGE` | `tailscale/tailscale:v1.104.2` | Tailscale sidecar image |
| `TZ` (dashboard) | `UTC` in compose; example `Europe/Amsterdam` | Timezone |
| `STORAGE_PATH` | `./storage/frigate` | Where recordings, clips and snapshots live. On a NAS, point it at an array share |
| `FRIGATE_SHM_SIZE` | `256mb` | Shared memory for decoded frames. Raise it as you add cameras |
| `FRIGATE_RTSP_USER`, `FRIGATE_RTSP_PASSWORD` | example values | Camera credentials, referenced in `frigate/config.yml` as `{FRIGATE_RTSP_USER}` and `{FRIGATE_RTSP_PASSWORD}` |
| `GO2RTC_ALLOW_ARBITRARY_EXEC` | `false` | Allow go2rtc `exec:` sources. Needed only for hand-written `_hq` streams. See the warning below |
| `FRIGATE_HOST_LAN_IP` | example | Bridge or Windows only: the host's LAN IP for the WebRTC candidate |
| `DASHBOARD_IMAGE` | `tinkerview:latest` | Dashboard image. Set to `ghcr.io/makerviking/tinkerview:latest` for the prebuilt one |
| `DASHBOARD_TOKEN` (dashboard) | empty | Shared bearer token. Empty means anyone who can reach port 8088 has full control. Set one: `openssl rand -hex 24` (`install.sh` does) |
| `DASHBOARD_LISTEN` (dashboard) | `:8088` | Listen address. Bind to one IP (a LAN or tailnet address) to narrow it |
| `DASHBOARD_LOG_LEVEL` (dashboard) | `info` | `debug`, `info`, `warn` or `error` |
| `DASHBOARD_WS_ORIGINS` (dashboard) | empty | Extra allowed WebSocket origins, comma separated, for a reverse proxy or dev server. Same-origin is always allowed |
| `DASHBOARD_DOCKER_HOST` (dashboard) | `tcp://127.0.0.1:2375` | The docker-socket-proxy endpoint. Reaches the dashboard container as `DOCKER_HOST`; the compose file does not read your own `DOCKER_HOST`, which `podman compose` sets for itself |
| `DOCKER_SOCKET_PATH` | `/var/run/docker.sock` | Host path of the engine API socket that docker-socket-proxy mounts. `install.sh` sets it for rootless podman (`/run/user/<uid>/podman/podman.sock`), rootless Docker and Docker Desktop |
| `DOCKERPROXY_POST` | `0` | `0`: read-only container health view, Restart disabled. `1`: the dashboard may restart containers, but the proxy then also permits creating them (root-equivalent). See [SECURITY.md](../SECURITY.md) |
| `MQTT_LAN_BIND` | `0.0.0.0` | With `compose.mqtt-lan.yml`: which interface port 1884 listens on |
| `NTFY_BASE_URL` (dashboard and ntfy) | `http://127.0.0.1:8092` | ntfy's own public URL |
| `NTFY_TOKEN` (dashboard) | empty | Publish token the dashboard uses |
| `TS_AUTHKEY` | empty | Tailscale auth key for `compose.tailscale.yml` |

The installer also writes `COMPOSE_FILE` here (see [INSTALL.md](INSTALL.md)).

**`GO2RTC_ALLOW_ARBITRARY_EXEC`.** While it is `true`, anyone who can reach go2rtc's API on
port 1984 can run commands inside the Frigate container. The dashboard does not need it
and refuses `exec:`, `echo:` and `expr:` sources itself. Turn it on only for the GPU
"HQ" streams you write by hand (see [CAMERAS.md](CAMERAS.md#gpu-hq-streams)), and only
where 1984 is firewalled to trusted machines or bound to localhost (`compose.bridge.yml`
does that).

## Dashboard `config.yaml`

The dashboard needs no config file to start. On first run it writes a commented
`dashboard/data/config.yaml` (`/data/config.yaml` in the container). The fully worked
reference is [`dashboard/config.example.yaml`](../dashboard/config.example.yaml). It is
resolved in this order:

1. `-config <path>` or `DASHBOARD_CONFIG`
2. `<data_dir>/config.yaml` (`/data/config.yaml`; `DASHBOARD_DATA_DIR` changes the folder)
3. `/config.yaml`, the legacy single-file mount

A missing, unreadable or empty file falls back to defaults. A malformed file fails
loudly. Environment variables override the file for `DASHBOARD_TOKEN`, `DASHBOARD_LISTEN`,
`DASHBOARD_DATA_DIR`, `DASHBOARD_LOG_LEVEL`, `DOCKER_HOST`, `NTFY_BASE_URL` and
`NTFY_TOKEN`. The file is not environment-expanded: writing `${DASHBOARD_TOKEN}` as a
value makes that literal string your token. Restart the dashboard to apply changes.

| Key | Meaning |
|---|---|
| `listen` | Listen address, default `:8088` |
| `log.level`, `log.file` | `debug`, `info`, `warn`, `error`; optional extra log file (stdout is always on) |
| `auth.token` | Same as `DASHBOARD_TOKEN` |
| `frigate.base_url` | Frigate's internal API, default `http://127.0.0.1:5000` |
| `frigate.mqtt_addr` | Broker, default `tcp://127.0.0.1:1883` |
| `frigate.topic_prefix` | Default `frigate` |
| `frigate.media_path` | Where the recordings volume is mounted (`/media/frigate`), used for disk usage |
| `go2rtc_url` | go2rtc, default `http://127.0.0.1:1984` |
| `data_dir` | Writable folder for UI-managed devices and Frigate backups, default `/data` |
| `hardware_transcode` | Transcode UI-added MJPEG cameras to H.264 on the GPU. Only on hosts with a usable encoder, for example NVENC |
| `docker.enabled`, `docker.host`, `docker.watch` | Container health. `watch` lists the container names to show and allow restarting |
| `ntfy.base_url`, `ntfy.topic`, `ntfy.token` | Push notifications |
| `printers` | Printers defined in the file (the UI can add more) |
| `quick_links` | Links shown in the dashboard |

### Printers

Add them in the UI (**Add printer** on the Printers tab) or in `config.yaml`.

| `kind` | Needs | Notes |
|---|---|---|
| `moonraker` | `base_url` (port 7125) | Klipper through Moonraker. Temperatures, speed and flow, babystep, move, macros, files, history |
| `octoprint` | `base_url`, `api_key` | OctoPrint |
| `prusalink` | `base_url`, `api_key` | PrusaLink password or key (Settings, Network, PrusaLink). Read plus pause, resume and cancel only: PrusaLink has no set-temperature, move or G-code. A Prusa running real OctoPrint should use `octoprint` |
| `bambu` | `ip`, `serial`, `access_code`, `model` | LAN mode. Control needs Developer Mode; until then the panel is read-only with a badge. `model` is one of `x1`, `x1c`, `p1s`, `p1p`, `a1`, `a1mini` |

A printer can name a go2rtc stream in `camera:` to show its camera in its panel. Control
commands are gated: home, move and extrude need an idle printer, extrusion needs 170 degrees
or more, and macros must be on an allowlist. Printer API keys and access codes are stored
in plain text in `dashboard/data/`.

## Frigate: `frigate/config.yml`

- **The host-specific block** at the top (`detectors:`, `model:`, `ffmpeg:`) comes from
  `frigate/host-presets/<preset>.yml`. The installer manages it between its markers
  ([INSTALL.md](INSTALL.md#how-the-preset-is-written)).
- **`go2rtc.streams`**: one entry per camera. Security cameras are native RTSP, with no
  re-encode. Printer MJPEG cameras use `ffmpeg:<url>#video=h264#hardware` to transcode to
  H.264 on the GPU so they can use WebRTC and be recorded.
- **`cameras`**: clone the `front_door` (security) and `printer1` (printer) examples. The
  **stream name must match the camera name**, and match in Home Assistant too. Both
  examples ship `enabled: false` because their URLs are placeholders.
- **Secrets** come from `.env` through `{FRIGATE_RTSP_USER}` and `{FRIGATE_RTSP_PASSWORD}`.
- **Retention** defaults: continuous 3 days (`mode: motion`), alerts and detections 14
  days, snapshots 14 days, printers `mode: all` for 2 days.
- Frigate may add or rewrite a `version:` key on first start. That is expected.
- Pin your image tags and read Frigate's release notes before changing them; 0.16 and
  later disabled detection by default and moved Nvidia to the ONNX detector.
- Frigate is its own project with its own outbound behaviour, for example its optional
  version check (`telemetry.version_check`). Review its documentation if that matters to you.

Cameras you add in the dashboard are written to `dashboard/data/devices.yaml`, and when
you promote one to Frigate the dashboard edits Frigate's config through its API. It does not
overwrite hand-written camera blocks; those stay read-only in the UI.

## Notifications (ntfy)

ntfy runs on port 8092 with default access `deny-all`, so you create a user and a token:

```sh
docker exec -it ntfy ntfy user add --role=admin admin
docker exec -it ntfy ntfy token add admin
```

Put the token in `.env` as `NTFY_TOKEN` (the dashboard's publish token) and recreate the
dashboard. Pick an unguessable topic in `dashboard/data/config.yaml` (`ntfy.topic`; the
installer generates one). Install the ntfy app on your phone, point it at
`http://<host>:8092` (or your tailnet name) and subscribe to the topic with a token that
may read it.

The rules (what alerts, at what priority, when) are edited in the dashboard on the
Recognition tab and stored in `notify-rules.json` in the data folder. The defaults keep
recognised faces and plates silent, push unknown people at night at priority 5, and push
people and cars at priorities 4 and 3.

### ntfy and ntfy.sh

The ntfy service is configured with `NTFY_UPSTREAM_BASE_URL=https://ntfy.sh` unless you
override it. That makes your server send a poll request with no message content to ntfy.sh,
which is how iOS gets instant push. Android and the web app do not need it. To stop your
server contacting ntfy.sh, put `NTFY_UPSTREAM_BASE_URL=` (empty) in `.env` and recreate
ntfy.

## Recognition and detection

- **Recognition tab**: enroll faces and licence plates and set notification rules.
- **Detection tab**: choose the detection hardware (the detector Frigate uses). This edits
  Frigate's config through Frigate's HTTP API (validate, save, optional restart), and the
  dashboard writes a timestamped backup (`frigate-config.backup.<time>.yml`) into its data
  folder before each change.
- **Recordings tab**: browse Frigate's recordings and clips.

Face and plate recognition is a Frigate feature (the dashboard manages the enrollment and
the rules); its accuracy depends on your cameras and hardware.

## Ports

The authoritative table is in [SECURITY.md](../SECURITY.md#what-listens-where). In short:
dashboard 8088, ntfy 8092, Frigate 5000 (unauthenticated) and 8971 (authenticated),
go2rtc 1984, RTSP restream 8554, WebRTC 8555, Mosquitto 1883 (loopback) and 1884
(optional, password), docker-socket-proxy 2375 (loopback).
