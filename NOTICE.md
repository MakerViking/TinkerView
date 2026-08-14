# Third-Party Notices

TinkerView incorporates the following third-party components. This file satisfies the
attribution requirements of their licenses. TinkerView itself is licensed under
**AGPL-3.0-only** (see [LICENSE](LICENSE)).

No dependency is under the GPL or AGPL, and every component below is either permissive
(MIT, ISC, BSD, Apache-2.0, OFL-1.1) or dual-licensed with a permissive option. All are
one-way compatible with AGPL-3.0, which permits distributing TinkerView under its own
terms.

> **On the Eclipse components.** Eclipse Paho and Mosquitto are dual-licensed
> `EPL-2.0 OR EDL-1.0`. The **EPL-2.0 branch is *not* compatible with the AGPL** —
> TinkerView takes the **EDL-1.0** branch (the Eclipse Distribution License, which is
> BSD-3-Clause in substance) for both. This is a deliberate election, not an oversight.

## Vendored source

These files are copied into this repository and shipped in built artifacts.

- **go2rtc web player** — `dashboard/web/public/video-rtc.js` (VideoRTC v1.6.0) and
  `dashboard/web/public/video-stream.js` — **MIT**.
  Copyright (c) 2022 Alexey Khit. Source: https://github.com/AlexxIT/go2rtc
  Vendored **unmodified** and pinned by hand; there is no npm package, so they must be
  re-pinned to match the go2rtc version bundled in your Frigate on upgrade (see
  `dashboard/web/public/README.md`).

## Bundled fonts

Embedded into the dashboard build (and therefore into the single Go binary) via
`@fontsource`. All three are **SIL Open Font License 1.1 (OFL-1.1)**.

- **Archivo** — Copyright 2020 The Archivo Project Authors.
  https://github.com/Omnibus-Type/Archivo
- **JetBrains Mono** — Copyright 2020 The JetBrains Mono Project Authors.
  https://github.com/JetBrains/JetBrainsMono
- **Chakra Petch** — Copyright 2018 The Chakra Petch Project Authors.
  https://github.com/m4rc1e/Chakra-Petch

## Go dependencies

Linked into the `tinkerview` dashboard binary. Direct dependencies are deliberately
minimal (see the note in `dashboard/go.mod`).

- **github.com/coder/websocket** v1.8.14 — **ISC**. Copyright (c) 2025 Coder.
  The dashboard's WebSocket hub.
- **github.com/eclipse/paho.mqtt.golang** v1.5.1 — **EPL-2.0 OR EDL-1.0**; used under
  **EDL-1.0** (see the note above). MQTT client for the Frigate event feed and the
  Bambu printer adapter.
- **gopkg.in/yaml.v3** v3.0.1 — **Apache-2.0**, with eight files ported from libyaml
  remaining under **MIT**. Copyright (c) 2011-2019 Canonical Ltd; MIT portions
  Copyright (c) 2006-2011 Kirill Simonov. Config and Frigate `config.yml` handling.
- **github.com/gorilla/websocket** v1.5.3 (indirect) — **BSD-2-Clause**.
  Copyright (c) 2013 The Gorilla WebSocket Authors.
- **golang.org/x/net** v0.44.0, **golang.org/x/sync** v0.17.0 (indirect) —
  **BSD-3-Clause**. Copyright 2009 The Go Authors.

## Frontend dependencies

Bundled into the dashboard's JavaScript build.

- **React** and **React DOM** v19 — **MIT**. Copyright (c) Meta Platforms, Inc. and
  affiliates.
- **Zustand** v5 — **MIT**. Copyright (c) 2019 Paul Henschel.

Build-time only (not shipped): TypeScript (Apache-2.0), Vite (MIT), Tailwind CSS (MIT),
`@vitejs/plugin-react` (MIT), and the `@types/*` packages (MIT, via DefinitelyTyped).

## Container images

TinkerView **orchestrates** these images — the compose files reference them and Docker
pulls them from their upstream registries at run time. They are **not redistributed by
this project**, are not modified, and each runs as a separate process under its own
license. This is mere aggregation; combining them with TinkerView via compose does not
create a combined work.

- **Frigate NVR** (`ghcr.io/blakeblackshear/frigate`) — **MIT**.
  Copyright (c) 2026 Frigate, Inc. (Frigate™). https://github.com/blakeblackshear/frigate
  Also supplies the bundled **go2rtc** (MIT) that does the camera fan-out.
- **Eclipse Mosquitto** (`eclipse-mosquitto`) — **EPL-2.0 OR EDL-1.0**; see the note
  above. https://github.com/eclipse-mosquitto/mosquitto
- **ntfy** (`binwiederhier/ntfy`) — **Apache-2.0**.
  Copyright 2021 Philipp C. Heckel. https://github.com/binwiederhier/ntfy
- **Tecnativa docker-socket-proxy** (`tecnativa/docker-socket-proxy`) — **Apache-2.0**.
  https://github.com/Tecnativa/docker-socket-proxy
- **Tailscale** (`tailscale/tailscale`) — **BSD-3-Clause**.
  Copyright (c) 2020 Tailscale Inc & contributors. https://github.com/tailscale/tailscale

## Trademarks

Frigate™ is a trademark of Frigate, Inc. Home Assistant, Klipper, Moonraker,
OctoPrint, PrusaLink, Bambu Lab, Obico, Tailscale, and Coral are trademarks of their
respective owners. TinkerView is an independent project and is **not affiliated with,
endorsed by, or sponsored by** any of them; the names are used only to describe
interoperability.
