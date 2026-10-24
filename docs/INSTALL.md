# Installing TinkerView

This page covers every way to install the stack: the installer script, plain Docker
Compose, bridge networking, and the dashboard on its own. For Unraid and CasaOS, start
with [unraid/README.md](../unraid/README.md) or [casaos/README.md](../casaos/README.md).

## Requirements

- A Linux host (amd64 or arm64) or a NAS, with Docker and **Docker Compose v2**. Podman
  with a compose provider works for most of it (see the note at the end).
- Camera URLs reachable from that host. Read [CAMERAS.md](CAMERAS.md) first if you have
  not prepared your cameras yet.
- For GPU detection or decode, the host driver for that hardware. The presets and their
  requirements are in [`frigate/host-presets/README.md`](../frigate/host-presets/README.md).
- About 6 GB of disk for the images (the Frigate image alone is 5.5 GB unpacked) plus
  room for recordings.
- Free ports on the host: 5000, 5001, 8971, 1984, 8554, 8555, 8088, 8092, 1883 and 2375.
  Frigate uses host networking and shares the host's loopback, so a busy port crash-loops
  it. The installer checks these before it starts anything.

## The installer

Run it from a clone of the repository:

```sh
git clone https://github.com/MakerViking/TinkerView.git
cd TinkerView
./install.sh --dry-run    # prints what it would do, changes nothing
./install.sh              # guided
```

With no options it is interactive: it detects your hardware, suggests a preset, and asks
before it changes or starts anything. With no terminal on stdin it behaves like `--yes`.

### Options

| Option | Meaning |
|---|---|
| `--preset NAME` | `nvidia`, `intel`, `amd-rocm`, `cpu` or `pi`. `coral` means `cpu` plus a Coral. Default: detected, then confirmed |
| `--coral` / `--no-coral` | Add the Coral overlay and detector. Default: only if one is detected |
| `--bridge` | Also add `compose.bridge.yml` (Windows, or a port conflict). Needs extra manual config |
| `--tailscale` | Also add the Tailscale sidecar. Needs `TS_AUTHKEY` in `.env` or in the environment |
| `--prebuilt` / `--build` | Use `ghcr.io/makerviking/tinkerview`, or build the dashboard from this checkout. Default: prebuilt only if the matching `1.1.0` tag can be pulled without logging in, otherwise build |
| `--storage PATH` | Where Frigate keeps recordings (`STORAGE_PATH` in `.env`) |
| `--token` / `--no-token` | Generate a `DASHBOARD_TOKEN` (default) or leave the dashboard open |
| `--allow-restart` | Turn on the dashboard's container Restart button (`DOCKERPROXY_POST=1`). Off by default; read [SECURITY.md](../SECURITY.md) first |
| `--selinux` / `--no-selinux` | Add `compose.selinux.yml`. Default: added automatically when `getenforce` says Enforcing |
| `--dir PATH` | The checkout to install into (default: the script's own directory) |
| `--start` / `--no-start` | Start the stack at the end without asking, or never |
| `-y`, `--yes` | Accept every default and ask nothing |
| `-n`, `--dry-run` | Change nothing |

Examples:

```sh
./install.sh --yes --preset cpu                  # unattended, safe first boot
./install.sh --preset intel --coral --build
./install.sh --preset nvidia --storage /mnt/user/frigate --tailscale
```

### What it does

- **Detects hardware.** `nvidia-smi` gives `nvidia`; a Raspberry Pi on aarch64 gives
  `pi`; an Intel GPU with `/dev/dri` gives `intel`; an AMD GPU with `/dev/kfd` gives
  `amd-rocm`; anything else gives `cpu`. A Coral is found through `/dev/apex_0` or the
  USB IDs `1a6e:089a` and `18d1:9302`.
- **Creates `.env`** from `.env.example` if it does not exist (mode 600). On later runs
  it only updates `TZ`, `STORAGE_PATH`, `DASHBOARD_IMAGE`, `DASHBOARD_TOKEN`,
  `TS_AUTHKEY`, `COMPOSE_FILE`, `DOCKERPROXY_POST` (only with `--allow-restart`),
  `DOCKER_SOCKET_PATH` (rootless podman, rootless Docker and Docker Desktop only) and, on
  `nvidia`, the `-tensorrt` suffix of `FRIGATE_IMAGE`. If you later switch away from
  `nvidia` it offers to take the suffix off again. It backs `.env` up first.
- **Records `COMPOSE_FILE`** in `.env` (for example
  `docker-compose.yml:compose.nvidia.yml`), so plain `docker compose up -d`, `ps` and
  `logs` work from the folder.
- **Writes the Frigate preset** into `frigate/config.yml`: the `detectors:`, `model:` and
  `ffmpeg:` blocks from `frigate/host-presets/<preset>.yml`. See below.
- **Creates folders**: `dashboard/data/` (the folder is owned by uid 65532, so the
  dashboard can write there; `config.yaml` inside stays yours), `ntfy/`,
  `mosquitto/data`, `mosquitto/log`, the storage path, and `tailscale/state` with
  `--tailscale`. If it cannot `chown` it prints the exact `sudo chown` command.
- **Writes `dashboard/data/config.yaml`** if absent: the commented template, with
  `ntfy.topic` set to `tinkerview-` plus 12 random characters.
- **Finds your engine socket.** The socket proxy mounts `/var/run/docker.sock` by default.
  For rootless podman, rootless Docker and Docker Desktop the installer asks the CLI where
  the real socket is and records it as `DOCKER_SOCKET_PATH`. It warns if the CLI talks to a
  remote engine or no socket exists.
- **Handles SELinux.** If `getenforce` prints `Enforcing` (Fedora, RHEL) it adds
  `compose.selinux.yml`. See [TROUBLESHOOTING.md](TROUBLESHOOTING.md#selinux-fedora-rhel).
- **Checks ports**, pulls (or builds) the image, runs `docker compose ... up -d`, and
  waits up to 60 seconds for `http://127.0.0.1:8088/healthz`.
- **Skips the automatic start** when the Nvidia or AMD detection model file is missing,
  or when `dashboard/data` could not be chowned. `--start` forces it.

It prints the command it used:

```sh
docker compose -f docker-compose.yml -f compose.<preset>.yml [-f compose.coral.yml] \
  [-f compose.bridge.yml] [-f compose.tailscale.yml] [-f compose.selinux.yml] up -d
```

Backups of anything it changes go to `storage/install-backups/<name>.<timestamp>` (mode
600). `storage/` is gitignored.

### How the preset is written

`frigate/config.yml` ships with a `HOST-SPECIFIC BLOCK` at the top. On the first run the
installer replaces that block with the preset and wraps it in markers:

```
# >>> tinkerview:host-preset ...
...
# <<< tinkerview:host-preset <<<
```

Later runs replace only what is between the markers. Delete the markers and the
installer never touches the file again. It refuses to write a duplicate top-level key or
invalid YAML (the YAML check needs PyYAML and is skipped without it). If it finds neither
the markers nor the original header it leaves the file alone and prints the manual steps.
With Coral it uses the detector from `coral.yml` and the `ffmpeg` block of your host
preset.

## Docker Compose by hand

```sh
cp .env.example .env     # required: compose reads .env for every ${VARIABLE} in the files
openssl rand -hex 24     # put this in DASHBOARD_TOKEN first (see below)
```

**Set `DASHBOARD_TOKEN` before the first start.** The hand-written route does not
generate one, and with it empty anyone on your network can use the dashboard (cameras,
printers, settings). Then edit the rest of `.env` (at least `TZ`, `STORAGE_PATH`,
`FRIGATE_RTSP_USER` and `FRIGATE_RTSP_PASSWORD`; see [CONFIGURATION.md](CONFIGURATION.md)):

```sh
mkdir -p dashboard/data && chown 65532:65532 dashboard/data
```

Paste your preset's `detectors:`, `model:` and `ffmpeg:` blocks over the
`HOST-SPECIFIC BLOCK` at the top of `frigate/config.yml` (copy them from
`frigate/host-presets/<preset>.yml`), then start with exactly one hardware overlay:

```sh
docker compose -f docker-compose.yml -f compose.nvidia.yml   up -d
docker compose -f docker-compose.yml -f compose.intel.yml    up -d
docker compose -f docker-compose.yml -f compose.amd-rocm.yml up -d
docker compose -f docker-compose.yml -f compose.cpu.yml      up -d
docker compose -f docker-compose.yml -f compose.pi.yml       up -d
```

Optional overlays stack on top of any of those:

| Add | For |
|---|---|
| `-f compose.coral.yml` | A Coral EdgeTPU (detection) with your GPU still decoding |
| `-f compose.mqtt-lan.yml` | Password-protected MQTT for Home Assistant on another machine |
| `-f compose.tailscale.yml` | The Tailscale sidecar (needs `TS_AUTHKEY`) |
| `-f compose.bridge.yml` | Bridge networking instead of host networking |
| `-f compose.selinux.yml` | SELinux Enforcing hosts (Fedora, RHEL). Untested by the author |

To make the command shorter, set `COMPOSE_FILE` in `.env` (the installer does this):

```sh
COMPOSE_FILE=docker-compose.yml:compose.nvidia.yml
```

By default the dashboard image is `tinkerview:latest`, built from `./dashboard` with
`docker compose build dashboard`. To use the prebuilt multi-arch image instead (once the
tag for your version is published), put this in `.env` and run `docker compose pull`:

```sh
DASHBOARD_IMAGE=ghcr.io/makerviking/tinkerview:latest
```

### Nvidia notes

GPU detection needs the TensorRT image: set
`FRIGATE_IMAGE=ghcr.io/blakeblackshear/frigate:0.17.1-tensorrt` in `.env`. It also needs
a detection model file that is not bundled; the export command is in the header of
`frigate/host-presets/nvidia.yml`, and it writes `yolov9-t-320.onnx` to
`frigate/model_cache` in the install folder. The first start builds a TensorRT engine and
takes several minutes. Start with `compose.cpu.yml` if you want to get cameras working
first.

### Check that it works

```sh
docker compose ps
curl -fsS http://127.0.0.1:8088/healthz
docker exec dashboard /dashboard -healthcheck && echo healthy
```

| What | URL |
|---|---|
| Dashboard | `http://<host>:8088` |
| Frigate | `http://<host>:5000` (authenticated UI: `https://<host>:8971`) |
| go2rtc | `http://<host>:1984` (per-camera stream health) |
| ntfy | `http://<host>:8092` |

In the go2rtc stream page, check that the transport for a live view says `webrtc`. A
playing video does not prove WebRTC is active.

## Bridge networking

`compose.bridge.yml` replaces host networking with published ports. Use it on Windows
with Docker Desktop (WSL2), or on a host where something else already owns a port Frigate
needs. Windows is Frigate's least-supported target; a Pi or mini PC makes a better
primary host.

```sh
docker compose -f docker-compose.yml -f compose.cpu.yml -f compose.bridge.yml up -d
```

It needs Docker Compose **2.24 or newer** (the overlay uses the `!reset` tag). Services
reach each other by name (`frigate`, `mqtt`, `dockerproxy`, `ntfy`). You must also:

1. Edit `dashboard/data/config.yaml` so the dashboard reaches the other services by name:
   `frigate.base_url: http://frigate:5000`, `frigate.mqtt_addr: tcp://mqtt:1883` and
   `go2rtc_url: http://frigate:1984`. The overlay already sets `DOCKER_HOST` and
   `NTFY_BASE_URL` for the dashboard.
2. In `frigate/config.yml`, set `mqtt.host` to `mqtt`.
3. For WebRTC, set `FRIGATE_HOST_LAN_IP` in `.env` to the machine's LAN IP and uncomment
   the `go2rtc: webrtc: candidates:` block in `frigate/config.yml`. Without it WebRTC
   quietly falls back to MSE, with higher latency.

Ports in this mode: Frigate 5000 and go2rtc 1984 on `127.0.0.1` only; 8971, 8554 and 8555
(tcp and udp) on all interfaces; the dashboard on 8088 and ntfy on 8092. There is no ONVIF
autodiscovery. Bridge mode has run once, on Linux under rootless podman with the CPU preset
(Frigate online, Mosquitto connected, container panel populated). It has not run on
Windows with Docker Desktop.

## Dashboard only

Use this if Frigate and go2rtc already run on your network. The dashboard is one container
from `ghcr.io/makerviking/tinkerview:latest`, on the host network.

```sh
mkdir -p data && chown 65532:65532 data
# The token below is generated once and printed by `docker inspect`; keep it, or pass your own.
docker run -d --name tinkerview --restart unless-stopped --network host \
  -v "$PWD/data:/data" \
  -e DASHBOARD_TOKEN="$(openssl rand -hex 24)" \
  ghcr.io/makerviking/tinkerview:latest
```

I ran this line with `podman run` on a clean machine: the container went healthy and wrote
its `config.yaml`. I have not run it under Docker Engine itself. It mirrors the compose
service and the Unraid template. Add `-v /path/to/recordings:/media/frigate:ro` for the disk-usage widget. Note
the token you generated (`docker inspect tinkerview` shows it) or set your own.

On first start the dashboard writes a commented `data/config.yaml`, owned by uid 65532.
Edit it with `sudo` (rootless podman: `podman unshare`, for example
`podman unshare nano data/config.yaml`). If Frigate is not on the same host, edit it and
restart the container:

```yaml
frigate:
  base_url: "http://nas.local:5000"
  mqtt_addr: "tcp://nas.local:1883"
go2rtc_url: "http://nas.local:1984"
docker:
  enabled: false      # unless you run a docker-socket-proxy
```

Container health and restart need a docker-socket-proxy and never the raw Docker socket;
without one, keep `docker.enabled: false`. Note that your Mosquitto must accept the
dashboard's connection: the dashboard has no MQTT credential setting.

## Podman

Rootless podman works with the installer and the compose files; this is the setup the
clean-machine test used.

Tested on a clean Linux machine with rootless podman 6.1 and the standalone
`docker-compose` binary as the compose provider (CPU preset, dashboard built with
`--build`, then again with `compose.bridge.yml`). Rootful podman, other distributions and
`podman-compose` are untested.

Set it up once:

```sh
systemctl --user enable --now podman.socket      # the API socket the proxy mounts
# Put the docker-compose v2 binary on your PATH (distro package 'docker-compose', or the
# release binary from https://github.com/docker/compose/releases).
./install.sh --yes --preset cpu --build
```

`podman compose` hands the work to that provider and prints a banner saying so. It sets
`DOCKER_HOST` for the provider itself, so you do not need to.

What is different from Docker, and what the installer does about it:

- Rootless podman has no `/var/run/docker.sock`. The installer finds your user socket
  (`/run/user/<uid>/podman/podman.sock`) and records it as `DOCKER_SOCKET_PATH` in `.env`,
  which the socket proxy mounts. If you start the stack by hand, set that key yourself, or
  install your distro's `podman-docker` package, or run rootful podman. With neither,
  `up` fails with `making volume mountpoint for volume /var/run/docker.sock`.
- The container panel lists every container the podman socket can see, not only
  TinkerView's.
- `dashboard/data` is chowned through `podman unshare`. By hand:
  `podman unshare chown 65532:65532 dashboard/data`. Edit files in it with
  `podman unshare` (or an editor that writes in place; `sed -i` cannot create its temp file
  there).
- Podman's default OCI image format drops `HEALTHCHECK`. The compose file repeats the
  dashboard healthcheck for that reason.
- The Nvidia preset needs the CDI or nvidia runtime configured for podman. It has not been
  run under podman.

## Uninstall

```sh
docker compose down            # keeps your data
```

Your data stays in `.env`, `dashboard/data/`, `frigate/`, `ntfy/`, `mosquitto/` and
`STORAGE_PATH`. Delete those folders to remove everything.
