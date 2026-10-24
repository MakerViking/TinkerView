# Upgrading

Your settings and recordings are not in the image or the release files. They live in
`.env`, `dashboard/data/`, `frigate/`, `ntfy/`, `mosquitto/` and `STORAGE_PATH`, so a
normal update does not touch them.

## A normal update

Prebuilt image (only if `DASHBOARD_IMAGE` in `.env` is the `ghcr.io/makerviking/tinkerview`
image; the default `tinkerview:latest` is built locally and cannot be pulled):

```sh
cd TinkerView
docker compose pull
docker compose up -d
```

Building the dashboard from source:

```sh
git pull
docker compose build dashboard
docker compose up -d
```

If you started with explicit `-f` flags instead of `COMPOSE_FILE` in `.env`, repeat them
on every command. The version in the dashboard header (and at `GET /api/version`) tells
you what is running. `docker compose pull` also fetches newer Frigate, ntfy and Mosquitto
images if you changed their tags in `.env`.

### `frigate/config.yml` and `git pull`

`frigate/config.yml` is tracked in the repository and you edit it, so a `git pull` can
conflict with your changes. Before you pull:

```sh
cp frigate/config.yml frigate/config.yml.mine
git stash          # or: git checkout frigate/config.yml
git pull
# merge your cameras back in from frigate/config.yml.mine
```

If you use the installer, re-run `./install.sh` afterwards; it refreshes the preset block
between its markers and backs up what it changes. If you installed from a release archive
instead of a clone, back up `frigate/config.yml` before unpacking a new release over the
folder, and restore it afterwards. See [unraid/README.md](../unraid/README.md#updating).

Pin Frigate to a tag you have tested. Read Frigate's release notes before raising
`FRIGATE_IMAGE`.

## Upgrading to 1.1.0 from 1.0.0

1.1.0 tightens the defaults. Check these:

- **MQTT is now loopback-only.** Port 1883 is published on `127.0.0.1`. Home Assistant on
  another machine will stop connecting. Add `-f compose.mqtt-lan.yml`, create a user, and
  point Home Assistant at port 1884. See [INTEGRATIONS.md](INTEGRATIONS.md#home-assistant).
- **`GO2RTC_ALLOW_ARBITRARY_EXEC` defaults to `false`.** If you use hand-written `_hq`
  streams, set it to `true` in `.env` (and read the warning in
  [CONFIGURATION.md](CONFIGURATION.md#env)). An existing `.env` keeps whatever it already says.
- **Frigate no longer receives all of `.env` either.** It gets `TZ`, `FRIGATE_RTSP_USER`,
  `FRIGATE_RTSP_PASSWORD`, `FRIGATE_HOST_LAN_IP` and `GO2RTC_ALLOW_ARBITRARY_EXEC`. If your
  `frigate/config.yml` uses another `{FRIGATE_...}` name, add it to the `frigate:
  environment:` list in `docker-compose.yml` (or a `docker-compose.override.yml`), or the
  value will be empty.
- **The dashboard no longer receives all of `.env`.** It gets only `TZ`, `DASHBOARD_TOKEN`,
  `DASHBOARD_LISTEN`, `DASHBOARD_LOG_LEVEL`, `DASHBOARD_WS_ORIGINS`, `DOCKER_HOST`
  (set from `DASHBOARD_DOCKER_HOST` in `.env`), `NTFY_BASE_URL` and `NTFY_TOKEN`. `DASHBOARD_DEMO` and `DASHBOARD_CONFIG` set in `.env`
  no longer reach it; put such settings in `dashboard/data/config.yaml`.
- **Set a dashboard token.** With none, the dashboard logs a warning at every start.
- **`DOCKER_HOST` in `.env` is now `DASHBOARD_DOCKER_HOST`.** The compose file used to pass
  your shell's own `DOCKER_HOST` into the dashboard, which broke container health under
  `podman compose`. If you changed the value from `tcp://127.0.0.1:2375`, rename the key.
- **The container Restart button is now off by default.** The docker-socket-proxy has a new
  `DOCKERPROXY_POST` setting and it defaults to `0` (read-only), because the proxy setting
  that allows restart also allows creating containers. An existing `.env` has no such key,
  so after the upgrade the health panel works but Restart reports that the proxy refused
  it. To keep the button, add `DOCKERPROXY_POST=1` to `.env` and read
  [SECURITY.md](../SECURITY.md#docker-access-docker-socket-proxy).
- **`compose.bridge.yml` was fixed.** It now needs Docker Compose 2.24 or newer, publishes
  Frigate 5000 and go2rtc 1984 on localhost only, and the dashboard's `config.yaml` must
  use service names. See [INSTALL.md](INSTALL.md#bridge-networking).
- **The Tailscale image is pinned** to `v1.104.2` (override with `TAILSCALE_IMAGE`).
- **The dashboard image has a healthcheck** (`/dashboard -healthcheck`), and the compose file
  repeats it.

## Dashboard config moved into the data folder

Older versions bind-mounted `dashboard/config.yaml` as a single file. Docker creates a
directory when that file does not exist on the host, which left some installs with an
unusable folder. The config now lives in `dashboard/data/`. If you still have the old file:

```sh
mv ./dashboard/config.yaml ./dashboard/data/config.yaml
```

If `dashboard/config.yaml` is a folder, delete it (`rm -rf dashboard/config.yaml`) and
remove the old mount line from your compose file. Keeping the old mount still works, since
the legacy path is read as a fallback, but new installs should not add it.

## Rolling back

Pin the previous image tag and recreate:

```sh
DASHBOARD_IMAGE=ghcr.io/makerviking/tinkerview:1.0.0 docker compose up -d dashboard
```

(Or set it in `.env`.) Images are tagged with the full version, `major.minor`, and `latest`.
Before changing Frigate's own config, note that the dashboard saves a timestamped backup
(`frigate-config.backup.<time>.yml`) in `dashboard/data/` each time it edits it.
