# TinkerView on CasaOS

CasaOS is Debian with Docker underneath, and it can import a Docker Compose file. Two
routes work; the first is simpler and the one I recommend.

Honest status: a CasaOS first-run bug (the dashboard could not write its data folder)
was found and fixed during development. I have not re-run the steps below on a fresh
CasaOS install for this release, so treat them as the expected path and report
anything that differs.

## Route 1: installer over SSH (recommended)

CasaOS lists containers started outside its UI as external apps, and they keep
running and restart with the machine.

```bash
ssh <user>@casaos.local
sudo mkdir -p /DATA/AppData/tinkerview && sudo chown "$USER" /DATA/AppData/tinkerview
cd /DATA/AppData/tinkerview
curl -fsSL https://github.com/MakerViking/TinkerView/archive/refs/tags/v1.1.0.tar.gz \
  | tar xz --strip-components=1
bash install.sh
```

The installer detects your hardware, creates `.env`, creates the data folders with the
right owner (the dashboard runs as uid 65532, which is exactly what CasaOS's own volume
creation gets wrong: it creates folders as root), configures Frigate, and offers to
start the stack. Add `--dry-run` first to preview. If `sudo` asks for a password, the
installer prints the exact `chown` command instead of failing silently.

Then open `http://<casaos-ip>:8088`.

## Route 2: import a compose file in the CasaOS UI

CasaOS imports one compose file, and TinkerView is a base file plus a hardware
overlay. Merge them first, on the machine:

```bash
cd /DATA/AppData/tinkerview          # after steps 1-4 of Route 1, using --no-start
docker compose -f docker-compose.yml -f compose.cpu.yml config > /tmp/tinkerview-casaos.yml
```

(Use your preset's overlay instead of `compose.cpu.yml`.) The merged file has absolute
paths. It also has the values from your `.env` written into it, including camera
passwords, so do not share it. In CasaOS: App Store, **Custom Install**, **Import**,
paste the file, then **Install**. Before you start it, make sure the data folder is
owned by uid 65532:

```bash
sudo chown 65532:65532 /DATA/AppData/tinkerview/dashboard/data
```

(No `-R`: only the folder needs the owner, so you can still edit `config.yaml` there.)
The merged file also bind-mounts the engine socket (`/var/run/docker.sock`, or your
`DOCKER_SOCKET_PATH`) into docker-socket-proxy, and it contains your `DASHBOARD_TOKEN`
if you set one. Set a token in `.env` before you merge: the CasaOS import does not ask
for one.

I have not verified how every CasaOS version treats the `build:` key in the merged file;
if the import complains, remove the `build:` lines and set
`DASHBOARD_IMAGE=ghcr.io/makerviking/tinkerview:latest` in `.env` before running
`docker compose config` again (the installer's `--prebuilt` default does this).

## Updating

```bash
cd /DATA/AppData/tinkerview
docker compose pull && docker compose up -d                # DASHBOARD_IMAGE is the ghcr.io image
docker compose build dashboard && docker compose up -d     # dashboard built from the checkout (default)
```

To pick up newer compose files, download the new release over the folder and run
`bash install.sh` again (it keeps `.env` and `dashboard/data/`; it re-applies the Frigate
preset block, but copy `frigate/config.yml` aside first, because the download replaces it
with the shipped example).

## Troubleshooting

- **Dashboard cannot save printers or cameras:** the data folder is owned by root.
  `sudo chown 65532:65532 /DATA/AppData/tinkerview/dashboard/data`
- **Frigate crash-loops on port 5001 or 5000:** CasaOS and its apps use many ports.
  Find the owner with `ss -lntp | grep :5001`, or install with `--bridge`.
