# TinkerView on Unraid

Unraid has no native Docker Compose support, so there are two routes. Pick one.

| Route | Gives you | Use it when |
|---|---|---|
| **A. Full stack (recommended)** | Frigate + go2rtc, Mosquitto, ntfy, docker-socket-proxy and the dashboard | You are starting fresh |
| **B. Dashboard only** | Just the dashboard container, from a Community Applications style template | You already run Frigate and only want the dashboard |

Honest status: the full stack runs daily on an Unraid server with an Nvidia GPU (the
`nvidia` preset). The Intel, AMD and Coral presets have not been run on real Unraid
hardware by the author. The template in route B has been validated as XML but has not
been submitted to Community Applications, so it does not appear in the Apps tab; you
add it by hand (below).

---

## Route A: the full stack

### 1. Prerequisites

- Unraid 6.12 or newer, with the Docker service enabled (Settings, Docker).
- **Docker Compose Manager** from the Apps tab (the plugin by dcflachs). It installs
  the `docker compose` command into the Unraid terminal and adds a Compose section to
  the Docker tab.
- For GPU use, the driver plugin for your hardware (see [GPUs](#gpus-and-accelerators)).

### 2. Put the files in appdata

Open the Unraid terminal (top right of the web UI) and run:

```bash
mkdir -p /mnt/user/appdata/tinkerview
cd /mnt/user/appdata/tinkerview
curl -fsSL https://github.com/MakerViking/TinkerView/archive/refs/tags/v1.1.0.tar.gz \
  | tar xz --strip-components=1
```

(Unraid does not ship `git`. If you have it, `git clone` into that folder works too.)

Keep everything in `/mnt/user/appdata/tinkerview`. The compose files use relative
paths such as `./frigate`, so they must be run from this folder.

### 3. Run the installer

```bash
cd /mnt/user/appdata/tinkerview
bash install.sh
```

It detects your hardware, suggests a preset, creates `.env`, creates the data
directories with the right owner (the dashboard runs as uid 65532), writes the matching
Frigate preset into `frigate/config.yml`, and offers to start the stack. Add `--dry-run`
first if you want to see what it will do. `bash install.sh --help` lists every option.

Useful on Unraid:

- Point recordings at an array share: `bash install.sh --storage /mnt/user/frigate`
  (create the share first in the Shares tab).
- Not sure about the GPU? Start with `--preset cpu`, confirm the cameras work, then
  re-run with your real preset. Re-running is safe; it backs up what it changes to
  `storage/install-backups/`.

### 4. Start it

If you let the installer start the stack, skip this. Otherwise, with the preset you
chose (this example is Nvidia):

```bash
cd /mnt/user/appdata/tinkerview
docker compose -f docker-compose.yml -f compose.nvidia.yml pull
docker compose -f docker-compose.yml -f compose.nvidia.yml up -d
```

The installer also records the files in `.env` (`COMPOSE_FILE=...`), so plain
`docker compose up -d`, `docker compose ps` and `docker compose logs -f frigate` work
from this folder.

### 5. Open it

| What | URL |
|---|---|
| Dashboard | `http://<unraid-ip>:8088` |
| Frigate | `http://<unraid-ip>:5000` (authenticated UI on `https://<unraid-ip>:8971`) |
| go2rtc | `http://<unraid-ip>:1984` |
| ntfy | `http://<unraid-ip>:8092` |

Then edit `frigate/config.yml` to add your cameras (the two examples ship disabled),
put camera credentials in `.env`, and restart Frigate:
`docker compose restart frigate`. Printers and extra cameras can be added from the
dashboard itself.

### Optional: manage the stack from the Compose Manager UI

Compose Manager keeps each stack's files on the flash drive, and relative paths such as
`./frigate` would resolve there, not in appdata. So do not paste `docker-compose.yml`
into a new stack. Instead create a stack named `tinkerview` and make its compose file a
small wrapper that points at the appdata copy, using Docker Compose's `include`
(needs Compose 2.20 or newer; check with `docker compose version`):

```yaml
include:
  - path:
      - /mnt/user/appdata/tinkerview/docker-compose.yml
      - /mnt/user/appdata/tinkerview/compose.nvidia.yml   # your preset overlay
    env_file: /mnt/user/appdata/tinkerview/.env
```

I have not verified the exact menu labels in current versions of the plugin, and I
have not tested this wrapper on Unraid. If it gives you trouble, the terminal route
above is the reliable one, and the containers still show up in the Docker tab either
way. All services use `restart: unless-stopped`, so they come back after a reboot once
the array and Docker service are up.

### Updating

```bash
cd /mnt/user/appdata/tinkerview
docker compose pull
docker compose up -d
```

That updates the dashboard (and the pinned Frigate/ntfy/Mosquitto images, if the tags
in `.env` changed). To also pick up a newer set of compose files, download the new
release over the folder. This overwrites `frigate/config.yml` with the shipped example,
so keep your copy:

```bash
cd /mnt/user/appdata/tinkerview
cp frigate/config.yml /tmp/frigate-config.keep
curl -fsSL https://github.com/MakerViking/TinkerView/archive/refs/tags/<new-tag>.tar.gz \
  | tar xz --strip-components=1
cp /tmp/frigate-config.keep frigate/config.yml
bash install.sh              # re-applies the hardware preset block and your COMPOSE_FILE
docker compose pull && docker compose up -d      # or: docker compose build dashboard && docker compose up -d
```

Your `.env` and `dashboard/data/` are not part of the download and are untouched. Do not
skip the copy-back: the download replaces `frigate/config.yml` with the shipped example,
cameras and all. (`pull` applies when `DASHBOARD_IMAGE` in `.env` is the ghcr.io image;
otherwise build.)

---

## GPUs and accelerators

Check what Unraid sees before choosing a preset.

### Nvidia

1. Apps tab, install **Nvidia Driver** (by ich777). Choose a driver version in its
   settings and let it download. In the terminal, `nvidia-smi` should now list the card.
2. Preset: `nvidia` (`compose.nvidia.yml` uses `runtime: nvidia`). The installer sets the
   `-tensorrt` Frigate image in `.env`, which GPU detection needs.
3. GPU detection also needs a model file that is not bundled. The export command is in
   the header of `frigate/host-presets/nvidia.yml`. Until it exists, Frigate cannot load
   the detector; use `--preset cpu` to get started. The installer warns about this.

### Intel iGPU / Arc

1. Check `ls /dev/dri`. If it does not exist, Unraid has the Intel driver
   blacklisted by default; the usual fix is to create an empty file
   `/boot/config/modprobe.d/i915.conf` and reboot.
2. Preset: `intel` (`compose.intel.yml` passes `/dev/dri`). On older Intel chips
   (before Gen9), edit `device: GPU` to `device: CPU` in `frigate/config.yml`; see
   `frigate/host-presets/README.md`.

### AMD

Preset `amd-rocm` needs both `/dev/dri` and `/dev/kfd`. ROCm support is patchy on
consumer cards and APUs; read the header of `frigate/host-presets/amd-rocm.yml`
(including the `HSA_OVERRIDE_GFX_VERSION` note). It also needs the YOLO-NAS model file
described there. If you only want hardware video decode, a Coral or the `cpu` preset
for detection is the dependable fallback.

### Google Coral

A USB Coral needs nothing extra. A PCIe/M.2 Coral needs the **Coral Accelerator Module
Driver** plugin so that `/dev/apex_0` exists, and you must uncomment the `/dev/apex_0`
line in `compose.coral.yml`. Install with `bash install.sh --coral` (it also offers it
when it detects one). Coral stacks on top of any decode preset.

---

## Route B: the dashboard only

Use this if Frigate and go2rtc already run on your network. The template starts one
container from `ghcr.io/makerviking/tinkerview:latest` on the host network.

### 1. Create the data folder

The container runs as uid 65532, not as `nobody`, so the folder needs that owner:

```bash
mkdir -p /mnt/user/appdata/tinkerview/data
chown 65532:65532 /mnt/user/appdata/tinkerview/data
```

### 2. Add the template

```bash
mkdir -p /boot/config/plugins/dockerMan/templates-user
curl -fsSL -o /boot/config/plugins/dockerMan/templates-user/my-tinkerview-dashboard.xml \
  https://raw.githubusercontent.com/MakerViking/TinkerView/main/unraid/tinkerview-dashboard.xml
```

### 3. Create the container

Docker tab, **Add Container**, choose **TinkerView** from the template drop-down (under
User templates), review the settings, **Apply**. Open `http://<unraid-ip>:8088`.

| Setting | What it does |
|---|---|
| WebUI port (8088) | Host network, so this is the port the dashboard binds |
| Data (`/data`) | `config.yaml`, UI-added printers and cameras, Frigate backups |
| Recordings (`/media/frigate`) | Optional, read-only, for the disk-usage widget |
| Access token (`DASHBOARD_TOKEN`) | Required by the template. Use a long random value (`openssl rand -hex 24`). Open once as `http://<ip>:8088/?token=...` |
| Docker proxy (`DOCKER_HOST`) | A docker-socket-proxy address for the container-health panel (read-only unless you run the proxy with `POST=1`; see [SECURITY.md](../SECURITY.md)) |
| ntfy URL / token | Push notifications, if you run ntfy |

### 4. Point it at your Frigate

On first start the dashboard writes a commented `config.yaml` into the data folder. If
Frigate is not on this same server (or not on the default ports), edit
`/mnt/user/appdata/tinkerview/data/config.yaml`, uncomment and set:

```yaml
frigate:
  base_url: "http://nas.local:5000"
  mqtt_addr: "tcp://nas.local:1883"
go2rtc_url: "http://nas.local:1984"
docker:
  enabled: false      # unless you run a docker-socket-proxy
```

Then restart the container. The container-health and restart features need a
`docker-socket-proxy` (never the raw Docker socket); without one set `docker.enabled`
to `false` as above.

### Notes on the template

- The icon is a PNG (`unraid/tinkerview-icon.png`, rendered from
  `assets/tinkerview-mark.svg`) because PNG is the safe format for Unraid and
  Community Applications icons.
- The template URL and icon URL point at the `main` branch on GitHub, so they only work
  once the repository is public and these files are on `main`.
- To update, use the Unraid Docker tab's "check for updates" / force update on the
  container as usual.

---

## Troubleshooting on Unraid

- **`docker compose` not found:** install Docker Compose Manager (Apps tab), then open
  a fresh terminal.
- **Frigate keeps restarting with `[Errno 98] ... ('127.0.0.1', 5001)`:** another
  container already owns a port Frigate needs (a Frigate from the Apps tab is the usual
  culprit). Find it with `ss -lntp | grep :5001`, stop it, or add `--bridge` when
  running the installer.
- **Dashboard cannot save printers or cameras:** `dashboard/data` (or the Route B data
  folder) is not owned by uid 65532. Run `chown 65532:65532 <folder>` (no `-R`, so `config.yaml` stays editable).
- **`nvidia-smi` works in the terminal but the container fails to start:** the driver
  plugin must be installed before the container is created; restart the Docker service
  after installing it.
- **Recordings fill the cache pool:** point `STORAGE_PATH` in `.env` at an array share.
