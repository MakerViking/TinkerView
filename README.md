# TinkerView · by TinkerAtlas

> A self-hosted hub that puts your **cameras (NVR)** and your **3D printers** in one
> pane — your homelab observatory.

A self-hosted, multi-camera **restream / NVR** server for **3D-printer cams (MJPEG)**
and **RTSP/ONVIF security cams**, plus a single-binary **Go + React control dashboard**
for those cameras and your **Klipper/Moonraker printers** (temps, fans, speed/flow,
babystep, move, macros, files, history), container health, and notifications. One
Frigate container does the camera plumbing: it bundles **go2rtc**, which opens **a
single upstream connection per camera and fans it out to unlimited viewers** — so
cameras that allow only one client at a time can now be watched everywhere. On top:
a live dashboard grid, NVR recording with tiered retention, object detection,
low-latency WebRTC, and a Home Assistant feed.

Designed to be handed to a friend: one shared stack + a per-host overlay they pick.

---

## What runs

| Container | Role | When |
|---|---|---|
| **frigate** | The hub: bundled go2rtc fan-out + dashboard + NVR + detection + WebRTC + HA feed | always |
| **mqtt** (Mosquitto) | Message bus for the Frigate ⇄ Home Assistant integration | for full HA |
| Obico (separate) | Print-failure detection on printer cams | optional, later |
| Tailscale (host-level) | Secure remote access, no exposed ports | recommended |
| Caddy | Auth + TLS, **only** if exposing a public URL | optional |

---

## Prerequisites

- Docker + Docker Compose.
- **Unraid:** install the *Nvidia Driver* plugin (for the GTX 1060) and Docker Compose Manager.
- Camera URLs reachable from the Docker host.

---

## Quick start

```bash
cp .env.example .env          # then edit: camera creds, TZ, STORAGE_PATH
# Edit frigate/config.yml: set your real camera URLs (two worked examples are there)
# Paste your host preset into the HOST-SPECIFIC block of frigate/config.yml:
#   frigate/host-presets/nvidia.yml   (your Unraid GTX 1060)

# Start with the matching overlay (pick ONE):
docker compose -f docker-compose.yml -f compose.nvidia.yml   up -d   # Unraid (GTX 1060)
docker compose -f docker-compose.yml -f compose.intel.yml    up -d   # Intel iGPU
docker compose -f docker-compose.yml -f compose.amd-rocm.yml up -d   # AMD GPU
docker compose -f docker-compose.yml -f compose.cpu.yml      up -d   # safe CPU-only
docker compose -f docker-compose.yml -f compose.pi.yml       up -d   # Raspberry Pi
docker compose -f docker-compose.yml -f compose.windows.yml  up -d   # Windows/Docker Desktop
```

Open the UI: `http://<host>:5000` (Frigate) and `http://<host>:1984` (go2rtc, per-camera health).

> **Tip:** start with `compose.cpu.yml` to confirm cameras connect and the dashboard
> loads, then switch to your real GPU overlay + preset.

---

## Step 0 — Camera inventory & codec prep (do this first)

1. List every camera URL:
   - **Printer (MJPEG):** confirm the real endpoint —
     Mainsail/Fluidd → `http://printer/webcam/?action=stream`,
     raw OctoPi → `http://printer/?action=stream`,
     ustreamer/camera-streamer → `http://printer:8080/stream`. Strip trailing slashes.
   - **Security (RTSP):** the main `rtsp://…` URL and a low-res substream URL.
2. On each **security** cam: set the main stream to **H.264 + AAC**, create a
   **~1280×720 H.264 substream** for detection, set **keyframe interval = fps**, and
   **disable "smart"/H.264+/H.265+** codecs for anything you'll view in a browser.
3. Note which printer cams genuinely cap at one client (those are the win).

---

## Fill in the config

Edit `frigate/config.yml`:

- **go2rtc streams** — one entry per camera. Security cams = native RTSP (no
  re-encode). Printer cams = `ffmpeg:<MJPEG-URL>#video=h264#hardware` (transcode to
  H.264 on the GPU so they can be viewed via WebRTC and recorded).
- **cameras** — clone the `front_door` (security) and `printer1` (printer) examples.
  The **stream name must match the camera name** (and match in Home Assistant).
- **Secrets** — credentials come from `.env` via `{FRIGATE_RTSP_PASSWORD}`.

---

## Verify it works

- **Fan-out fix:** open the same printer cam from 3+ browser tabs *and* its printer
  UI at once — all stay live. On `:1984` confirm a single upstream consumer.
- **Low latency:** in the go2rtc stream page confirm the transport says **webrtc**
  (~0.5 s). *Video playing ≠ WebRTC active.*
- **Live wall:** in Frigate create a **Camera Group → "Continuous Streaming"** (the
  default "All Cameras" page is 1 fps stills and never WebRTC).
- **Detection:** walk past a security cam → object event + clip in Frigate.
- **Recording:** segments appear under `STORAGE_PATH` (`/media/frigate/recordings`).

---

## Home Assistant (full integration)

1. Mosquitto is already running. Point HA's **MQTT** integration at the same broker
   (`<host>:1883`). Frigate's `mqtt:` is enabled in `config.yml`.
2. Install the **Frigate** integration (HACS) and HA's built-in **go2rtc**
   integration. (`RTSPtoWebRTC` is deprecated.)
3. Ensure each go2rtc **stream name exactly matches** the Frigate camera name, or HA
   falls back to jsmpeg (~5 fps).
4. Add the **advanced-camera-card** (grid mode) for a dashboard; enable
   `media_source` for clip playback.

---

## Print-failure detection (Obico) — optional, later

See `compose.obico.yml`. Deploy Obico from its upstream project, point it at each
printer's **snapshot** URL, and start in **notify-only** mode. Kept separate from
Frigate on purpose (don't run printer cams through Frigate's person/car detector).

---

## Remote access (Tailscale)

Install **Tailscale on the host** (Unraid plugin / Windows app / Pi package) — not as
a container, so Frigate keeps host networking for WebRTC. Reach `http://<tailnet-host>:5000`
from anywhere. No port-forwarding, no public exposure. Only add Caddy if you want a
real public URL.

---

## Troubleshooting / known gotchas

- **WebRTC quietly becomes MSE (higher latency)** — Docker hides `172.16/12`
  candidates. On host networking (Unraid/Pi) it's automatic; on **Windows/bridge**
  set `FRIGATE_HOST_LAN_IP` in `.env` and uncomment the `go2rtc: webrtc: candidates:`
  block in `config.yml`. Always verify the transport on `:1984`.
- **`address already in use` on 8555** — only **one** go2rtc may own it. Don't run a
  separate go2rtc or an HA go2rtc add-on on the same host as Frigate.
- **Black tiles** — an H.265 / "smart" codec stream. Point the live view at the plain
  H.264 substream.
- **Detector won't start / wrong host** — the detector + `ffmpeg.hwaccel_args` are
  host-specific (no CPU fallback for hwaccel; OpenVINO is amd64-only). Use the
  matching `host-presets/` file; fall back to `cpu.yml` to get booting.
- **HA entities missing** — the Frigate HA integration **requires MQTT** on both
  sides pointed at the same broker. Raw RTSP live still works without it.
- **Windows MQTT** — on the bridge overlay, set `mqtt: host:` to `host.docker.internal`.
- **Pin your tags** — no `latest`. Verify the Frigate tag in `.env` against current
  releases; note 0.16+ disabled detection by default and moved Nvidia to ONNX.
- **VRAM (GTX 1060, 3 GB)** — keep the detect model at 320×320 and watch `nvidia-smi`.
- **Recording isn't a sole backup** — monitor disk; keep a second copy for footage you can't lose.

---

## File map

```
docker-compose.yml        base: frigate + mosquitto
compose.<host>.yml        host overlay: nvidia / intel / amd-rocm / cpu / pi / windows
compose.obico.yml         optional print-failure detection (deferred)
.env.example              copy to .env: camera creds, TZ, storage, image tags
frigate/config.yml        cameras, go2rtc streams, record/detect, mqtt
frigate/host-presets/     detector + hwaccel block to paste per host
mosquitto/config/         broker config
Caddyfile                 optional public-exposure proxy
```
