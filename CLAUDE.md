# TinkerView — guidance for AI coding assistants

This repo is built with AI assistance (see "AI assistance" in `README.md`). This file
is the context an assistant should load before changing anything.

If a private, gitignored `handoff.md` exists at the repo root, read it first — it holds
the maintainer's local ops notes and backlog. It is never committed; do not copy its
contents into tracked files.

## Layout
- `dashboard/` — Go backend (`cmd/dashboard`, `internal/**`) + React/Vite frontend (`dashboard/web`).
- `frigate/` — `config.yml` example and `host-presets/<preset>.yml` hardware presets.
- `compose.*.yml` — overlays on `docker-compose.yml` (hardware presets, bridge, tailscale, obico).

## Gates (run before proposing a change)
- Go (from `dashboard/`): `go vet ./... && go test -race ./...`
- Frontend (from `dashboard/web/`): `npm run typecheck && npm run build`

## Rules
- Examples, tests, placeholders, and docs use generic values only (`192.168.1.x`,
  `nas.local`, `192.0.2.10`). Never commit real LAN IPs, hostnames, credentials, or
  ntfy topics.
- Do not change the marker/section structure of `frigate/config.yml`; the installer and
  README depend on it.
- Never commit `.env`, `dashboard/config.yaml`, `dashboard/data/`, or tailscale node state.

## Code-intelligence tooling (optional)
If you use Mimir (`mcp__mimir__*`), its code graph answers structure questions
("what calls X", "what breaks if I change Y") more cheaply than reading files. The
older Graphify tooling is retired; `graphify-out/` is still gitignored.
