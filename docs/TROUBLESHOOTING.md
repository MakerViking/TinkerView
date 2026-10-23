# Troubleshooting

Start with the logs. They name most problems directly:

```sh
docker compose ps
docker compose logs --tail 100 frigate dashboard
```

Before you open an issue, collect that output, your preset and the compose command you ran,
and remove credentials and tokens from it.

## Install and start-up

**`docker compose` not found, or `unknown shorthand flag`.** You need Docker Compose v2 (the
`docker compose` plugin), not the old `docker-compose` v1. `compose.bridge.yml` needs 2.24
or newer. Check with `docker compose version`. On Unraid, install Docker Compose Manager.

**Frigate crash-loops with `[Errno 98] ... bind on ('127.0.0.1', 5001)`.** That is Frigate's
internal API port, not something you configured. On host networking the container shares
the host's loopback, so a port that is busy on the host is busy for Frigate. Find the other
listener with `ss -lntp | grep :5001`. On NAS platforms it is often a second Frigate from
an app catalog. Stop it, or switch to `compose.bridge.yml`. The same applies to 5000,
1984, 8554 and 8555.

**`address already in use` on 8555.** Only one go2rtc may own it. Do not run a separate
go2rtc, or a Home Assistant go2rtc add-on, on the same host as Frigate.

**The dashboard cannot save printers or cameras.** The data folder is not writable by the
container user (uid 65532). Fix the owner:

```sh
sudo chown 65532:65532 dashboard/data
```

No `-R`: the folder itself is what matters, and `config.yaml` inside stays editable by you.
Use `sudo chown -R 65532:65532 dashboard/data` only if files inside were created as root
and the dashboard reports them unreadable (then you need `sudo` to edit `config.yaml`).

The dashboard logs `data dir not creatable; adding/editing devices will fail` in this case.
CasaOS creates volume folders as root, which is why its guide has this step.

**Dashboard says "no config file", or `dashboard/config.yaml` is a folder.** See
[UPGRADING.md](UPGRADING.md#dashboard-config-moved-into-the-data-folder). Nothing else is
needed: printers and cameras can be added from the UI.

**Camera passwords or the dashboard token seem to be ignored.** Compose reads `.env` for
every `${VARIABLE}` in the files. Without it (`cp .env.example .env` first) the defaults
apply: no dashboard token and empty camera credentials. Frigate receives only the variables
listed in `docker-compose.yml`; if your `frigate/config.yml` uses another `{FRIGATE_...}`
name, add it to the `frigate: environment:` list (or a `docker-compose.override.yml`).

**The dashboard container shows `unhealthy`.** Check `docker compose logs dashboard`. You can
test the check by hand: `docker exec dashboard /dashboard -healthcheck`. If you changed
`DASHBOARD_LISTEN`, the check follows it. Under podman the image's own healthcheck is
dropped, which is why the compose file repeats it.

### SELinux (Fedora, RHEL)

Symptoms: Frigate or the dashboard cannot read `/config` or
`/data` (`permission denied` although the owner is right), or `docker-socket-proxy` cannot
open the engine socket. Check with `getenforce`. If it prints `Enforcing`, add
`-f compose.selinux.yml` to your compose command (the installer does this itself); it sets
`label=disable` on the TinkerView containers. The alternative is `:z` on each bind mount.
I have not run this on an SELinux host, so treat it as the expected fix, not a tested one.

### Rootless Docker or Docker Desktop

Symptoms: the container panel is empty, or `up` complains about
`/var/run/docker.sock`. There is no socket there. Set `DOCKER_SOCKET_PATH` in `.env` to
your engine's socket (rootless Docker: `/run/user/<uid>/docker.sock`). The installer does
this for you. Not tested by the author.

## Cameras and video

**`Unable to read frames from ffmpeg process` on a fresh install.** The two demo cameras in
`frigate/config.yml` ship disabled with placeholder URLs. If you enabled one before pointing
its `go2rtc:` stream at a real camera, this is the log you get every few seconds. Fix the
URL or set `enabled: false` again, then restart Frigate.

**Black tiles.** An H.265 or "smart codec" stream. Point the live view at the plain H.264
substream. See [CAMERAS.md](CAMERAS.md).

**A camera tile freezes.** Known issue: a WebRTC tile can occasionally stall. The dashboard
watches for it and reconnects the tile after about 15 seconds. If it stays frozen, reload
the tab. The root cause is still under investigation, so if you see it often, note the
browser, the camera and whether the tab was in the background, and open an issue.

**WebRTC quietly becomes MSE (higher latency).** Docker hides `172.16/12` candidates. On
host networking it works automatically. On Windows or bridge networking, set
`FRIGATE_HOST_LAN_IP` in `.env` and uncomment the `go2rtc: webrtc: candidates:` block in
`frigate/config.yml`. Always verify the transport on the go2rtc page (`:1984`). If your ISP
gives you broken IPv6 STUN candidates, the file has a commented `ice_servers: []` block.

**A printer camera works in one place but not two.** The camera allows one client, and
something else is connecting to it directly. Point every viewer at go2rtc, not at the camera.

**`OpenEncodeSessionEx failed: incompatible client key (21)` or `No capable devices found`.**
The Nvidia encoder is out of sessions, usually because of orphaned HQ encoders. Run
`pkill -f h264_nvenc` on the host and keep `-timeout 15000000` in your `_hq` recipe. See
[CAMERAS.md](CAMERAS.md#gpu-hq-streams).

**HQ streams do not play.** `GO2RTC_ALLOW_ARBITRARY_EXEC` must be `true` in `.env`, and
Frigate must be recreated after you change it. Read the security note first.

## Detection hardware

**The detector will not start, or Frigate picked the wrong hardware.** The detector and
`ffmpeg.hwaccel_args` are host-specific. There is no CPU fallback for hwaccel, and OpenVINO
is amd64 only. Use the matching file from `frigate/host-presets/`, and fall back to `cpu.yml`
to get booting.

**Intel: Frigate aborts with `device: GPU`.** That setting aborts on a chip with no OpenVINO
GPU plugin (before Gen9). Use `device: CPU`. `NPU` is for Core Ultra.

**Nvidia: detection runs on the CPU, or the detector fails to load.** GPU detection needs the
`-tensorrt` Frigate image (`FRIGATE_IMAGE=...-tensorrt`) and the exported model file.
`nvidia-smi` should list the card on the host. The first start builds a TensorRT engine and
takes several minutes. On a 3 GB card keep the model at 320x320 and watch `nvidia-smi`. If
`nvidia-smi` works on the host but the container fails, the driver must be installed before
the container is created; restart the Docker service after installing it.

**AMD ROCm misdetects the GPU.** Set `HSA_OVERRIDE_GFX_VERSION` as described in
`frigate/host-presets/amd-rocm.yml`. On APUs and iGPUs ROCm is unreliable; a Coral or
OpenVINO on CPU is the dependable fallback. The `amd-rocm` preset has not been run on real
hardware by the author.

**Coral not found.** A USB Coral can re-enumerate on its first inference; uncomment
`privileged: true` in `compose.coral.yml` if it is missing right after plugging it in. A
PCIe or M.2 Coral needs the host driver so `/dev/apex_0` exists, and the `/dev/apex_0` line
uncommented in `compose.coral.yml`.

## Home Assistant and MQTT

**HA entities are missing.** The Frigate integration requires MQTT on both sides, pointed at
the same broker. Raw RTSP live view works without it.

**HA cannot connect to MQTT after upgrading to 1.1.0.** Port 1883 is now loopback-only.
Use `compose.mqtt-lan.yml` and port 1884; see
[INTEGRATIONS.md](INTEGRATIONS.md#home-assistant).

## Access and security

**I set a token and now the dashboard says unauthorized.** Open it once as
`http://<host>:8088/?token=<token>`; the browser stores the token. A different browser or
profile needs the link again. Do not write `${DASHBOARD_TOKEN}` in `config.yaml`; the file
is not env-expanded and the literal string would become the token.

**A web socket or POST is rejected as cross-origin.** The dashboard accepts same-origin
requests only. Behind a reverse proxy, set `DASHBOARD_WS_ORIGINS` to the extra host
patterns.

**Container health shows "No container data yet".** The dashboard cannot reach the
docker-socket-proxy. Check `DASHBOARD_DOCKER_HOST` in `.env` (default `tcp://127.0.0.1:2375`) and that the
`docker-socket-proxy` container is up. With the default `DOCKERPROXY_POST=0` the list works
and Restart is disabled; the Restart button then reports that the proxy refused it. Set
`DOCKERPROXY_POST=1` only after reading [SECURITY.md](../SECURITY.md).

## Storage

**Recordings fill the cache pool or the disk.** Point `STORAGE_PATH` in `.env` at a large
share and lower the retention in `frigate/config.yml`. Recording is not a backup; keep a
second copy of footage you cannot lose.

**Not sure whether a setting took effect.** Check `docker compose config` to see the merged
result of your files, and `docker compose up -d` again after editing `.env`.
