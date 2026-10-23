# Changelog

All notable changes to TinkerView are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [1.1.0] - 2026-10-23

The polish and launch release: more printers, a recognition and recording UI, a redesigned
interface, a hardened default configuration, and an installer so other people can run it.

### Added

- **Installer.** `install.sh` detects your hardware, suggests a preset, creates `.env` and
  the data folders with the right owner, writes the Frigate preset into
  `frigate/config.yml` between markers, starts the stack and waits for the dashboard to
  answer. It supports `--dry-run`, `--yes`, `--build`, `--bridge`, `--tailscale`,
  `--storage`, `--token` and more, and backs up anything it changes.
- **Platform guides.** `unraid/` (full stack through Compose Manager, plus a Docker
  template for the dashboard alone) and `casaos/`.
- **Printers.** OctoPrint, PrusaLink (with fallback to the legacy API on older firmware)
  and Bambu Lab (LAN mode) adapters beside Moonraker. Any camera can be attached to any
  printer kind.
- **Edit flows.** Edit any camera or printer from the UI, including source URL changes that
  apply live through go2rtc, camera credentials, camera rename that follows through to
  printers, and rotate and flip controls.
- **Recording and recognition.** Record-on-detect, zones, a recordings browser, face and
  licence-plate enrollment, and prioritised ntfy rules with a "specific name" match.
- **Detection hardware picker** and an inference-speed report after applying it, plus a
  broader set of presets: Nvidia, Intel (iGPU, Arc, NPU), AMD ROCm, Raspberry Pi, Coral
  and CPU.
- **Shared-token auth** (`DASHBOARD_TOKEN`) for the API, WebSocket and proxied endpoints.
- **Tailscale sidecar** (`compose.tailscale.yml`) that publishes only the dashboard.
- **`compose.mqtt-lan.yml`**: a password-protected MQTT listener on port 1884 for Home
  Assistant on another machine.
- **Container healthcheck**: `/dashboard -healthcheck`, in the image and in compose.
- **CI** (`.github/workflows/ci.yml`): gofmt, `go mod tidy` check, `go vet`,
  `go test -race`, `npm ci`, typecheck, production build, `docker compose config` for every
  overlay combination, and a no-push image build. Actions are pinned to commit SHAs.
- **Documentation**: a rewritten README and `docs/` (install, configuration, cameras,
  integrations, upgrading, troubleshooting, architecture), `SECURITY.md`,
  `CODE_OF_CONDUCT.md`, and `NOTICE.md` and `CONTRIBUTING.md`.
- **Licence**: AGPL-3.0-only, with third-party notices.
- A startup warning when the dashboard runs without a token.
- `compose.selinux.yml` (`label=disable`), added by the installer on SELinux Enforcing
  hosts. Written, not run on such a host.
- The installer finds a rootless Docker or Docker Desktop socket (as it does for podman),
  offers the Restart opt-in, and defaults to building the dashboard image until the
  prebuilt one can be pulled.

### Changed

- **Redesigned interface** ("quiet instrument"): a warm graphite palette with an amber
  accent, camera tiles that fill the frame with controls on hover, a dense Server list,
  grouped navigation and a mobile tab bar. The active tab is kept in the URL hash.
- **Dashboard state and config moved into `dashboard/data/`** (a directory mount) instead
  of a bind-mounted single file. The demo cameras ship disabled.
- **Mosquitto is published on `127.0.0.1` only.** Home Assistant on another machine must
  use `compose.mqtt-lan.yml` (port 1884).
- **`GO2RTC_ALLOW_ARBITRARY_EXEC` defaults to `false`.** The GPU "HQ" streams need it set to
  `true`; see the security note.
- **Neither the dashboard nor Frigate receives the whole `.env` any more.** Each gets only
  the variables it needs, so the dashboard token, ntfy token and Tailscale key stay out of
  Frigate (which is on the host network with unauthenticated ports) and camera passwords
  stay out of the dashboard. Extra `{FRIGATE_...}` variables go in the `frigate:
  environment:` list.
- **`compose.bridge.yml`** (renamed from `compose.windows.yml`) now actually uses the
  compose network, so service names resolve. It needs Docker Compose 2.24 or newer and
  publishes Frigate 5000 and go2rtc 1984 on localhost only.
- **docker-socket-proxy is read-only by default.** `DOCKERPROXY_POST` defaults to `0`, so
  the container panel shows health but the Restart button is disabled; `1` turns restart on
  but also permits creating containers (root-equivalent). `install.sh --allow-restart` or a
  prompt opts in. It stays on loopback.
- **Image build**: `npm ci` with the lockfile, `go build -mod=readonly` instead of
  `go mod tidy`, and a `HEALTHCHECK`.
- **Camera tiles** tear down their WebSocket and peer connection on unmount and on the HQ
  toggle, and reconnect after a stall watchdog fires (about 15 seconds).
- **HTTP hardening**: token gating for the go2rtc and Frigate proxies, same-origin checks on
  mutating requests and WebSocket handshakes, gzip, immutable caching for hashed assets, and
  `nosniff`, framing and referrer headers.
- The Tailscale image is pinned.

### Fixed

- Dashboard shutdown no longer hangs until SIGKILL when Mosquitto is unreachable (the MQTT
  connect no longer blocks the shutdown path).
- The Restart button explains a proxy refusal instead of showing a bare HTTP 403.
- A CasaOS first-run failure: the dashboard could not write its data folder, which Docker
  or CasaOS had created as root.
- The first-run crash when the config file was missing or a directory.
- The HQ NVENC recipe leaked encoder sessions when go2rtc restarted a stalled producer; it
  now has `-timeout` so an orphaned ffmpeg ends itself.
- Face uploads are normalised to JPEG in the browser (registration returned HTTP 500), and
  enrollment through the Frigate proxy no longer fails with 401 (Origin and Referer are
  stripped).
- PrusaLink: tolerant parsing, MD5-sess digest authentication, and a workaround for a bogus
  nozzle temperature on Prusa XL stock firmware.
- Hub and Moonraker robustness: a closed-channel panic, reconnect back-off that now resets,
  log de-spam, a liveness timeout and a re-fire guard on print completion.
- Camera credential keep and clear semantics, and store changes roll back when a save fails.
- `compose.bridge.yml` could not resolve `mqtt`, `frigate` or `dockerproxy` on the legacy
  bridge network.
- `config.example.yaml` no longer ships a literal `${VAR}` token that locked users out.
- Rootless podman: `docker compose up` failed outright because the socket proxy bind-mounts
  `/var/run/docker.sock`. The mount is now `DOCKER_SOCKET_PATH` (default unchanged), and
  `install.sh` sets it to the podman user socket.
- Under `podman compose` the dashboard received podman's own `DOCKER_HOST` instead of the
  socket proxy address, so the container panel stayed empty. The `.env` key is now
  `DASHBOARD_DOCKER_HOST`.
- `install.sh` left `dashboard/data/config.yaml` owned by uid 65532, so you needed root to
  edit it. Only the folder changes owner now; the file stays yours.

### Security

- `exec:`, `echo:` and `expr:` sources are rejected in UI-entered camera sources.
- Secrets are never serialised back out of the API.
- Frigate's environment is an explicit list instead of `env_file: .env`.
- `install.sh` cleans up its temporary files (which can hold secrets) on interrupt.
- See [SECURITY.md](SECURITY.md) for the trust model and how to report a vulnerability.

### Known issues

- A WebRTC camera tile can occasionally stall. The dashboard reconnects it automatically
  after about 15 seconds; the root cause is under investigation.
- The `intel`, `amd-rocm`, `pi` and `coral` presets have not been run on real hardware,
  and bridge mode has only run on Linux under podman, not on Windows.

## [1.0.0] - 2026-06-06

First release: a Frigate and go2rtc camera hub with a hardware preset per host (Nvidia,
Intel, AMD, Raspberry Pi, CPU), Mosquitto, ntfy and docker-socket-proxy, and a single-binary
Go and React dashboard with a live camera wall, Klipper/Moonraker printer control,
container health and push notifications. Multi-arch (amd64 and arm64) images are published
to `ghcr.io/makerviking/tinkerview` by the release workflow.

[1.1.0]: https://github.com/MakerViking/TinkerView/releases/tag/v1.1.0
[1.0.0]: https://github.com/MakerViking/TinkerView/releases/tag/v1.0.0
