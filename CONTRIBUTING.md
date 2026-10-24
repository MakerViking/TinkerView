# Contributing to TinkerView

Thanks for your interest in TinkerView! A few ground rules keep the project healthy
and its licensing clean.

## License of your contributions

TinkerView is released under **AGPL-3.0-only** (see [`LICENSE`](LICENSE)). By
submitting a contribution (a pull request, patch, or any code or content), you
agree that:

1. Your contribution is licensed to the project and its users under
   **AGPL-3.0-only**; and
2. You grant the project maintainer, **MakerViking** (the copyright holder), a
   perpetual, irrevocable, worldwide, royalty-free right to **relicense your
   contribution under other terms**, including a commercial license.

This dual-licensing grant is what lets TinkerView stay fully open-source under the
AGPL while the maintainer can also offer a commercial license to organizations that
can't use AGPL software, the revenue that keeps the project maintained. It's the
same inbound-relicensable model used by projects like GitLab and Qt.

You confirm you have the right to grant this (the work is yours, or your employer
has authorized it).

> This is a lightweight contributor agreement, not legal advice; a formal
> CLA/DCO document may replace this note later.

Third-party components and their licenses are listed in [`NOTICE.md`](NOTICE.md).
If you add a dependency, add it there — and note that AGPL-3.0 constrains what is
acceptable inbound: permissive licenses (MIT, ISC, BSD, Apache-2.0) are fine, GPL/AGPL
dependencies are not, and anything EPL-licensed needs a permissive dual-license branch.

## Development

For running the app locally, see [`dashboard/README.md`](dashboard/README.md)
(`./dashboard/run-local.sh` serves it in demo mode) and
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Before opening a PR, these must pass; CI
runs the same checks:

- From `dashboard/`: `gofmt -l .` (no output), `go mod tidy` (no diff), `go build ./...`,
  `go vet ./...`, and `go test -race ./...`.
- From `dashboard/web/`: `npm ci`, `npm run typecheck` and `npm run build`.
- If you touch a compose file, `docker compose config -q` must still pass for the
  overlay combinations in `.github/workflows/ci.yml`.

Never put real IP addresses, hostnames, tokens, camera credentials or ntfy topics in tracked
files; use examples such as `192.168.1.50` and `nas.local`. Please follow the
[Code of Conduct](CODE_OF_CONDUCT.md), and report vulnerabilities privately as described in
[SECURITY.md](SECURITY.md).

> A standalone `tsc` run in your editor may report `window` / `document` / `location`
> "not found" — that's a false alarm from ignoring the project's `lib: dom`.
> `npm run typecheck` is the source of truth.

## Conventions

- **Printer adapters** live in `internal/adapters/<kind>/` behind the shared `Printer`
  interface. Common logic (clamping, home G-code, macro allowlisting) belongs in
  `internal/adapters/printerutil`, not copied per adapter.
- **Keep the safety gates.** Printer control is gated on an idle state for
  home/move/extrude, a 170 °C minimum extrude temperature, a two-stage macro
  allowlist plus injection guard, and absolute/feedrate clamping. New adapters mirror
  these; don't loosen them for convenience.
- **Camera image transforms** (rotate/flip) are applied at the go2rtc source so the
  live tile, recordings, snapshots, and detection all stay in agreement.
  `CameraRecord.Src` stays the pristine base URL; the transform fields drive an
  idempotent wrapper.
- **Never overwrite hand-authored `config.yml` camera blocks.** Adopted cameras stay
  UI-read-only (`managed: false`).
- The vendored go2rtc player files in `dashboard/web/public/` are pinned by hand.
  If you re-pin them, work through the checklist in that directory's `README.md` —
  `VideoStream.tsx` depends on two player internals that are not public API.
