# Security

TinkerView is built to run as an appliance on a network you control: a home LAN
or a tailnet. It is not hardened to be put on the open internet, and I do not
recommend doing that. This page says what is exposed, what protects it, what the
known limits are, and how to report a problem.

## Reporting a vulnerability

Please report privately through GitHub Security Advisories:
<https://github.com/MakerViking/TinkerView/security/advisories/new>

Do not open a public issue for a security problem. Include the version
(`/api/version` or the container image tag), how you run it, and the steps to
reproduce. I will acknowledge a report within a few days and keep you posted. This
is a one-person project, so I cannot promise a fixed timeline, but security fixes
come before features.

Only the latest release is supported.

## Trust model

- **The network is the perimeter.** Everything below assumes only people and
  devices you trust can reach the host. Do not forward these ports from your
  router.
- **The dashboard is full control.** Anyone who can open it can see every camera,
  pause or cancel a print, restart containers (if the Docker proxy allows it) and
  add or remove devices. There are no user accounts or roles. There is one shared
  token, or none.
- **Frigate and go2rtc keep their own rules.** Frigate's port 5000 and go2rtc's
  1984 are unauthenticated by design. Frigate's authenticated UI is 8971.

## What listens where

The stack uses host networking for Frigate, the dashboard and ntfy, so those
services bind the host's interfaces directly.

| Service | Port | Reachable from | Purpose |
| --- | --- | --- | --- |
| Dashboard | 8088 | LAN (all interfaces) | The web UI and API. Set `DASHBOARD_LISTEN` to narrow it. |
| ntfy | 8092 | LAN (all interfaces) | Push notifications. Default access is deny-all; you create users and tokens. |
| Frigate UI (authenticated) | 8971 | LAN | Frigate's login-protected UI. |
| Frigate UI (internal) | 5000 | Host network: LAN. Bridge overlay: localhost. | Unauthenticated Frigate API. Keep it off untrusted networks. |
| go2rtc API | 1984 | Host network: LAN. Bridge overlay: localhost. | Unauthenticated stream API. See the exec warning below. |
| RTSP restream | 8554 | LAN | Camera streams for VLC, Home Assistant and the like. Unauthenticated video. |
| WebRTC | 8555 tcp+udp | LAN | Low-latency live view. |
| Mosquitto | 1883 | **localhost only** | Anonymous MQTT for Frigate, the dashboard and same-host Home Assistant. |
| Mosquitto (optional) | 1884 | LAN, password required | Only with `compose.mqtt-lan.yml`. |
| docker-socket-proxy | 2375 | **localhost only** | Docker API for container health and restart. |

In host-network mode Frigate decides how it binds ports 5000 and 1984, and
`docker-compose.yml` cannot narrow them. If your host has a firewall, allow 5000
and 1984 only from machines you trust.

## Dashboard token

Set a shared token and the REST API, the WebSocket and the proxied
Moonraker and go2rtc endpoints require it.

1. Generate one: `openssl rand -hex 24`
2. Put it in `.env` as `DASHBOARD_TOKEN=...` (or `auth.token` in
   `dashboard/data/config.yaml`).
3. Recreate the dashboard container, then open
   `http://<host>:8088/?token=<token>` once. The browser stores it.

With no token set the dashboard still works exactly as before, and logs this
warning at every start:

```
dashboard is open to anyone who can reach port 8088; set auth.token or DASHBOARD_TOKEN - see SECURITY.md
```

Limits to know about:

- The token is sent over plain HTTP unless you put TLS in front. On a tailnet
  that traffic is already encrypted; on a LAN it is not. Tailscale Serve
  (`compose.tailscale.yml`) gives you HTTPS on your tailnet.
- It is one shared secret. Anyone who has it has full control. Rotate it by
  changing the value and recreating the container.
- Printer API keys, printer access codes and camera credentials you enter are
  stored in plain text in `dashboard/data/` and, for cameras you promote to
  Frigate, in `frigate/config.yml`. Protect those directories and any backups.

## Docker access (docker-socket-proxy)

The dashboard never touches `/var/run/docker.sock`. It talks to
`tecnativa/docker-socket-proxy`, which is bound to `127.0.0.1:2375`.

Be clear about what that buys you. The dashboard needs list, stats and restart.
This proxy version can allow `POST` on containers only as a whole, so with
`POST=1` it also allows creating containers, and a container created with a host
mount is root on the host. So:

- Treat anything that can reach `127.0.0.1:2375` (and the dashboard itself) as
  holding root-equivalent access to the host.
- Reading container details also reveals each container's environment variables.
- The proxy is **read-only by default** (`DOCKERPROXY_POST=0`): the health view works
  and the Restart button is disabled. Set `DOCKERPROXY_POST=1` in `.env` only if you want
  the button and accept the above. `install.sh --allow-restart` does the same.
- Frigate, ntfy and the dashboard all share the host network, so a compromise of any of
  them can reach `127.0.0.1:2375`. That is the reason for the read-only default.
- With rootless podman the proxy mounts your user's engine socket (`:ro` on a socket
  does not restrict API calls). The same reasoning applies.

## What each container sees of `.env`

Neither container is given the whole file.

- **Frigate** gets `TZ`, `FRIGATE_RTSP_USER`, `FRIGATE_RTSP_PASSWORD`,
  `FRIGATE_HOST_LAN_IP` and `GO2RTC_ALLOW_ARBITRARY_EXEC`. It sits on the host network
  with unauthenticated ports, so `DASHBOARD_TOKEN`, `NTFY_TOKEN` and `TS_AUTHKEY` are
  deliberately not in its environment. Your camera passwords are, because it needs them.
- **The dashboard** gets only `TZ`, `DASHBOARD_TOKEN`, `DASHBOARD_LISTEN`,
  `DASHBOARD_LOG_LEVEL`, `DASHBOARD_WS_ORIGINS`, the proxy address, `NTFY_BASE_URL` and
  `NTFY_TOKEN`. No camera passwords.

## Outbound connections

TinkerView has no account system and does not report to me. What leaves your network:

- **ntfy to `ntfy.sh`**: a poll request containing a hash of the topic and no message
  text, so iOS gets instant push. Auth stays deny-all. Switch it off with
  `NTFY_UPSTREAM_BASE_URL=` (empty) in `.env`; Android does not need it.
- **Frigate** is its own project with its own outbound behaviour (for example an
  update check). Read its documentation and adjust its config if that matters to you.
- **Image pulls** from the registries named in the compose files (ghcr.io and Docker Hub).
- **Tailscale**, only if you add `compose.tailscale.yml`.

The dashboard itself makes no outbound calls beyond the devices and services you
configure on your network.

## go2rtc `exec:` sources

`GO2RTC_ALLOW_ARBITRARY_EXEC` is `false` in `.env.example`. The dashboard does not
need it. It exists for the optional GPU-enhanced `<camera>_hq` streams that you
write yourself in `frigate/config.yml`.

If you turn it on, anyone who can reach go2rtc's API on port 1984 can run
commands inside the Frigate container, which holds your camera credentials and
recordings. Only enable it on a host where port 1984 is firewalled to trusted
machines or bound to localhost (the bridge overlay does that). The dashboard
itself refuses `exec:`, `echo:` and `expr:` sources, and exposes go2rtc
read-only.

## MQTT

Mosquitto is published on the host's loopback only (`127.0.0.1:1883`), with
anonymous access, because Frigate and the dashboard run on the host network and
the dashboard has no MQTT credential setting.

For Home Assistant on another machine, do not publish 1883 to the LAN. Use the
password-protected second listener:

```sh
docker exec -it mosquitto mosquitto_passwd -c /mosquitto/data/passwd homeassistant
docker compose -f docker-compose.yml -f compose.<preset>.yml -f compose.mqtt-lan.yml up -d mqtt
```

Then point Home Assistant at `<host-ip>:1884` with that user. Add more users with
the same `mosquitto_passwd` command without `-c`. This is plain MQTT without TLS,
so keep it on a trusted network.

## Printer connections

The dashboard talks to printers on your LAN with the credentials you give it.

- **Moonraker (Klipper):** the dashboard's read-only file and history browser goes
  through a proxy that adds the printer's API key. That proxy only allows the
  endpoints the browser needs (file list, file metadata, G-code thumbnails, print
  history). Everything else, including Moonraker's `/access/*` credential endpoints,
  config files and machine endpoints, is refused.
- **Bambu Lab:** the printer's MQTT connection uses TLS, but the dashboard does not
  verify the printer's certificate yet (Bambu printers present a certificate from
  Bambu's own CA, which is not in the system trust store). Someone who can intercept
  traffic on your LAN could impersonate the printer and capture its access code.
  Keep Bambu printers on a trusted network segment. Proper certificate pinning is
  planned.

## Hardening checklist

- [ ] Set `DASHBOARD_TOKEN`.
- [ ] Do not forward any of these ports from your router.
- [ ] Keep `GO2RTC_ALLOW_ARBITRARY_EXEC=false` unless you use `_hq` streams.
- [ ] Leave `DOCKERPROXY_POST=0` (the default) unless you want the Restart button.
- [ ] Firewall ports 5000 and 1984 to trusted machines (host-network mode).
- [ ] Create ntfy users and tokens; leave its default access at deny-all.
- [ ] Use Tailscale (or another VPN) for remote access instead of opening ports.
      `compose.tailscale.yml` is an optional sidecar for that.
- [ ] Give your cameras their own VLAN, or block their internet access.
- [ ] Keep images current: `docker compose pull && docker compose up -d`.
- [ ] Back up `dashboard/data/`, `frigate/` and `ntfy/`, and treat the backups as secrets.

## Supply chain

Container images are pinned to specific versions in `docker-compose.yml`. The
GitHub Actions workflows pin every action to a full commit SHA and run with
read-only permissions, except the release job, which can push to GHCR. The
dashboard image is distroless and runs as a non-root user.
