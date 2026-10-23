# Integrations: Home Assistant, Tailscale, Obico, Caddy

## Home Assistant

TinkerView does not need Home Assistant, but Frigate's own integration works with it
unchanged.

1. **Point HA's MQTT integration at the broker.**
   - Home Assistant on the **same machine**, with host networking: broker
     `127.0.0.1`, port `1883`, no credentials. Mosquitto's anonymous port is published on
     the host's loopback only.
   - Home Assistant on **another machine**, or in a bridge container: use the
     password-protected listener on port **1884**:

     ```sh
     docker exec -it mosquitto mosquitto_passwd -c /mosquitto/data/passwd homeassistant
     docker compose -f docker-compose.yml -f compose.<preset>.yml -f compose.mqtt-lan.yml up -d mqtt
     ```

     Then use `<host-ip>` port `1884` with that user. Add more users with the same
     `mosquitto_passwd` command without `-c`. The password file is kept in
     `mosquitto/data/`, which is gitignored. `MQTT_LAN_BIND` in `.env` limits which
     interface 1884 listens on (default: all). This is plain MQTT with no TLS, so keep it
     on a trusted network.
2. **Install the Frigate integration** (through HACS) and Home Assistant's built-in
   **go2rtc** integration. `RTSPtoWebRTC` is deprecated.
3. **Match names.** Each go2rtc stream name must exactly match the Frigate camera name, or
   Home Assistant falls back to jsmpeg at about 5 fps.
4. **Dashboards.** The advanced-camera-card (grid mode) works well. Enable `media_source`
   for clip playback.

If entities are missing: the Frigate integration requires MQTT on both sides, pointed at
the same broker. Raw RTSP live view works without it.

On the bridge overlay Frigate reaches the broker as `mqtt:1883` (set `mqtt.host: mqtt` in
`frigate/config.yml`). Do not use `host.docker.internal`; that applies to a different
network layout.

## Tailscale (remote access)

You have two options. Neither opens a port on your router.

**The sidecar (`compose.tailscale.yml`).** A Tailscale container in userspace mode that
publishes **only the dashboard** (port 8088) on your tailnet through Tailscale Serve,
privately to the tailnet. It is not a public Funnel.

1. Create an auth key in the Tailscale admin console (Settings, Keys) and put it in
   `.env`: `TS_AUTHKEY=tskey-auth-...`. Without a key the container prints a one-time
   login URL in its logs instead: `docker logs tinkerview-tailscale`.
2. Set `DASHBOARD_TOKEN` too. Anyone on your tailnet can reach the dashboard through this,
   so decide who that should be with your ACLs and gate it.
3. Start it with your usual files plus the overlay (or run `./install.sh --tailscale`):

   ```sh
   docker compose -f docker-compose.yml -f compose.nvidia.yml \
     -f compose.tailscale.yml --env-file .env up -d tailscale
   ```

4. Open `https://tinkerview.<your-tailnet>.ts.net` from any device on the tailnet.

State is kept in `tailscale/state`, and the Serve config is `tailscale/serve.json`. The
image is pinned (`TAILSCALE_IMAGE` overrides it). The sidecar needs no `NET_ADMIN`
capability or tun device. Tailscale Serve also gives you HTTPS, so the dashboard token is
not sent in clear text.

**Tailscale on the host.** Install Tailscale on the host itself (Unraid plugin, Windows
app, Pi package) rather than as a container. Then every service on the host is reachable
over the tailnet, for example `http://<tailnet-host>:5000`, which also means Frigate's
unauthenticated ports are as well. Prefer the sidecar if you only want the dashboard
reachable.

## Obico (print-failure detection), optional

Obico detects spaghetti and other print failures from printer cameras. It is deliberately
separate from Frigate: do not run printer cameras through Frigate's person and car detector.

Obico self-hosted is its own multi-service stack (web, ML API, tasks, Redis, Postgres)
maintained upstream, so TinkerView does not ship it. [`compose.obico.yml`](../compose.obico.yml)
is only notes and a commented placeholder. The recommended flow:

1. Deploy Obico from <https://github.com/TheSpaghettiDetective/obico-server> and follow its
   own compose setup.
2. Add each printer in Obico and point it at the printer's **snapshot** URL, for example
   `http://<printer>/webcam/?action=snapshot`. go2rtc also serves a stable snapshot at
   `http://<frigate-host>:1984/api/frame.jpeg?src=printer1`.
3. Start in notify-only mode. Enable auto-pause only after you have tuned it.

## Caddy (public URL), optional and not recommended

The repository includes a [`Caddyfile`](../Caddyfile) for the case where you really want a
public URL with authentication and TLS in front of the services. It is not wired into the
compose stack on purpose. Frigate's port 5000 and go2rtc's 1984 are unauthenticated; never
expose them to the internet directly. Generate the password hash with
`docker run --rm caddy caddy hash-password --plaintext 'yourpass'`. Remote access through
Tailscale is the simpler and safer route, and is what I recommend.
