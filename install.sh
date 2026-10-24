#!/usr/bin/env bash
# =============================================================================
# TinkerView installer — run it from a clone of the repository.
#
#   ./install.sh                 interactive: detect hardware, confirm, set up
#   ./install.sh --yes           accept every default (scriptable)
#   ./install.sh --dry-run       show what would happen, change nothing
#   ./install.sh --help          all options
#
# What it does (and nothing more):
#   1. checks for docker/podman + a compose provider
#   2. detects your hardware and suggests a preset (you confirm or override)
#   3. creates .env from .env.example if it does not exist (never overwritten)
#   4. creates the data directories the containers need, with the right owner
#   5. writes the matching frigate/host-presets/<preset>.yml into the managed
#      HOST-SPECIFIC block of frigate/config.yml (backup first)
#   6. prints the exact compose command and offers to run it
#
# It is safe to re-run: existing files are kept, edited files are backed up to
# storage/install-backups/ first.
# =============================================================================
set -euo pipefail

# ----------------------------------------------------------------- defaults --
REPO_DIR=""
PRESET=""
ASSUME_YES=0
DRY_RUN=0
START="auto" # auto | yes | no
CORAL="auto" # auto | yes | no
BRIDGE=0
TAILSCALE=0
IMAGE_MODE="" # prebuilt | build
STORAGE_ARG=""
WANT_TOKEN=auto # auto | yes | no
ALLOW_RESTART=auto # auto | yes | no  (DOCKERPROXY_POST: the dashboard Restart button)
SELINUX=auto # auto | yes | no
PRESETS="nvidia intel amd-rocm cpu pi"

PRESET_BEGIN="# >>> tinkerview:host-preset"
PRESET_END="# <<< tinkerview:host-preset"
PREBUILT_IMAGE="ghcr.io/makerviking/tinkerview:latest"
# The first published tag whose image matches these compose files (healthcheck flag, /data
# layout). The installer only defaults to the prebuilt image when this tag can be pulled.
PREBUILT_PROBE_TAG="1.1.0"

# Temp files are removed on any exit, including Ctrl-C. They can hold secrets (.env contents).
TMPFILES=()
cleanup() { [ "${#TMPFILES[@]}" -eq 0 ] || rm -f "${TMPFILES[@]}"; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ------------------------------------------------------------------- output --
if [ -t 1 ]; then
  C_BOLD=$'\033[1m'
  C_DIM=$'\033[2m'
  C_RED=$'\033[31m'
  C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'
  C_RESET=$'\033[0m'
else
  C_BOLD="" C_DIM="" C_RED="" C_GREEN="" C_YELLOW="" C_RESET=""
fi

say() { printf '%s\n' "$*"; }
step() { printf '\n%s==>%s %s%s%s\n' "$C_GREEN" "$C_RESET" "$C_BOLD" "$*" "$C_RESET"; }
info() { printf '    %s\n' "$*"; }
ok() { printf '    %s[ok]%s %s\n' "$C_GREEN" "$C_RESET" "$*"; }
warn() { printf '    %s[warn]%s %s\n' "$C_YELLOW" "$C_RESET" "$*" >&2; }
err() { printf '%s[error]%s %s\n' "$C_RED" "$C_RESET" "$*" >&2; }
die() {
  err "$*"
  exit 1
}
dry() { printf '    %s[dry-run]%s %s\n' "$C_DIM" "$C_RESET" "$*"; }

usage() {
  cat <<'EOF'
TinkerView installer

Usage: ./install.sh [options]

Run it from a clone of the repository (or point --dir at one). With no options
it is interactive: it detects your hardware, suggests a preset, and asks before
it changes or starts anything.

Options:
  --preset NAME     Hardware preset: nvidia | intel | amd-rocm | cpu | pi
                    ("coral" is accepted as cpu + a Coral accelerator).
                    Default: auto-detected, then confirmed.
  --coral           Also add the Coral EdgeTPU overlay + detector. It stacks on any
  --no-coral        preset: that preset keeps decoding, the Coral only detects.
                    Default: only if a Coral is detected.
  --bridge          Also add compose.bridge.yml (Windows/Docker Desktop, or a
                    host with a port conflict). Needs extra manual config.
  --tailscale       Also add the Tailscale sidecar (dashboard only). Needs
                    TS_AUTHKEY in .env (or exported when you run this).
  --prebuilt        Use the prebuilt dashboard image from ghcr.io. This is the
                    default once that image can be pulled; until then (or offline)
                    the default is --build.
  --build           Build the dashboard image from this checkout instead.
  --storage PATH    Where Frigate keeps recordings (STORAGE_PATH in .env).
  --token           Generate a DASHBOARD_TOKEN and require it for the UI/API
  --no-token        (default: yes, recommended). --no-token leaves the dashboard open.
  --allow-restart   Enable the dashboard's container Restart button (sets
                    DOCKERPROXY_POST=1). Off by default: that setting is
                    root-equivalent for anything that can reach 127.0.0.1:2375.
  --selinux         Add compose.selinux.yml (label=disable on the containers). Default:
  --no-selinux      added automatically when SELinux is Enforcing (Fedora, RHEL).
  --dir PATH        The TinkerView checkout to install (default: this script's
                    directory).
  --start           Start the stack at the end without asking.
  --no-start        Set everything up but do not start the stack.
  -y, --yes         Accept all defaults; ask nothing. Also used automatically
                    when stdin is not a terminal.
  -n, --dry-run     Print what would be done; change nothing.
  -h, --help        Show this help.

Environment:
  TS_AUTHKEY        Tailscale auth key, used with --tailscale.

Examples:
  ./install.sh                         # guided
  ./install.sh --dry-run               # see what it would do
  ./install.sh --yes --preset cpu      # unattended, safe first boot
  ./install.sh --preset intel --coral --build
EOF
}

# ------------------------------------------------------------ argument parse --
need_arg() { if [ "$#" -lt 2 ] || [ -z "$2" ]; then die "option $1 needs a value (see --help)"; fi; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    --preset)
      need_arg "$@"
      PRESET="$2"
      shift 2
      ;;
    --preset=*)
      PRESET="${1#*=}"
      shift
      ;;
    --dir)
      need_arg "$@"
      REPO_DIR="$2"
      shift 2
      ;;
    --dir=*)
      REPO_DIR="${1#*=}"
      shift
      ;;
    --storage)
      need_arg "$@"
      STORAGE_ARG="$2"
      shift 2
      ;;
    --storage=*)
      STORAGE_ARG="${1#*=}"
      shift
      ;;
    --coral)
      CORAL=yes
      shift
      ;;
    --no-coral)
      CORAL=no
      shift
      ;;
    --bridge)
      BRIDGE=1
      shift
      ;;
    --tailscale)
      TAILSCALE=1
      shift
      ;;
    --prebuilt)
      IMAGE_MODE=prebuilt
      shift
      ;;
    --build)
      IMAGE_MODE=build
      shift
      ;;
    --token)
      WANT_TOKEN=yes
      shift
      ;;
    --no-token)
      WANT_TOKEN=no
      shift
      ;;
    --allow-restart)
      ALLOW_RESTART=yes
      shift
      ;;
    --selinux)
      SELINUX=yes
      shift
      ;;
    --no-selinux)
      SELINUX=no
      shift
      ;;
    --start)
      START=yes
      shift
      ;;
    --no-start)
      START=no
      shift
      ;;
    -y | --yes)
      ASSUME_YES=1
      shift
      ;;
    -n | --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      err "unknown option: $1"
      usage >&2
      exit 2
      ;;
  esac
done

# Non-interactive when there is no terminal to ask on.
INTERACTIVE=1
if [ "$ASSUME_YES" -eq 1 ] || [ ! -t 0 ]; then
  INTERACTIVE=0
fi

if [ -z "$REPO_DIR" ]; then
  REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi
[ -d "$REPO_DIR" ] || die "--dir: not a directory: $REPO_DIR"
REPO_DIR="$(cd "$REPO_DIR" && pwd)"
[ -f "$REPO_DIR/docker-compose.yml" ] || die "docker-compose.yml not found in $REPO_DIR — run this from a TinkerView checkout (or use --dir)."
[ -f "$REPO_DIR/.env.example" ] || die ".env.example not found in $REPO_DIR — is this a complete checkout?"
cd "$REPO_DIR"

if [ -n "$PRESET" ]; then
  case "$PRESET" in
    coral)
      PRESET=cpu
      CORAL=yes
      ;;
    nvidia | intel | amd-rocm | cpu | pi) ;;
    *) die "unknown preset '$PRESET' — choose one of: $PRESETS (or coral)" ;;
  esac
fi

if [ "$DRY_RUN" -eq 1 ]; then
  say "${C_BOLD}Dry run:${C_RESET} nothing will be changed, created, or started."
fi

# -------------------------------------------------------------------- utils --
TS="$(date +%Y%m%d-%H%M%S)"
BACKUP_DIR="$REPO_DIR/storage/install-backups"
BACKED_UP=" "
PENDING=() # commands the user must run by hand (e.g. chown needing root)

have() { command -v "$1" >/dev/null 2>&1; }

# ask_yn "question" default(y|n) -> 0 for yes
ask_yn() {
  local q="$1" def="$2" hint ans
  if [ "$INTERACTIVE" -eq 0 ]; then
    [ "$def" = y ]
    return
  fi
  if [ "$def" = y ]; then hint="[Y/n]"; else hint="[y/N]"; fi
  while true; do
    printf '    %s %s ' "$q" "$hint"
    read -r ans || ans=""
    ans="$(printf '%s' "$ans" | tr '[:upper:]' '[:lower:]')"
    case "$ans" in
      "") [ "$def" = y ]; return ;;
      y | yes) return 0 ;;
      n | no) return 1 ;;
      *) say "    please answer y or n" ;;
    esac
  done
}

# ask_text "question" default -> prints the answer
ask_text() {
  local q="$1" def="$2" ans
  if [ "$INTERACTIVE" -eq 0 ]; then
    printf '%s' "$def"
    return
  fi
  printf '    %s [%s] ' "$q" "$def" >&2
  read -r ans || ans=""
  printf '%s' "${ans:-$def}"
}

rand_str() { # rand_str LENGTH -> lowercase alphanumerics
  local n="$1" s
  s="$(LC_ALL=C tr -dc 'a-z0-9' </dev/urandom 2>/dev/null | head -c "$n" || true)"
  if [ "${#s}" -lt "$n" ]; then
    s="$(printf '%s%s' "$(date +%s)" "$$" | od -An -tx1 | tr -d ' \n' | head -c "$n")"
  fi
  printf '%s' "$s"
}

backup_file() { # backup_file PATH — once per run per file
  local f="$1" base
  [ -e "$f" ] || return 0
  case "$BACKED_UP" in *" $f "*) return 0 ;; esac
  base="$(basename "$f")"
  if [ "$DRY_RUN" -eq 1 ]; then
    dry "back up $base to storage/install-backups/$base.$TS"
  else
    mkdir -p "$BACKUP_DIR"
    cp -p "$f" "$BACKUP_DIR/$base.$TS"
    chmod 600 "$BACKUP_DIR/$base.$TS" 2>/dev/null || true
    info "backed up $base to storage/install-backups/$base.$TS"
  fi
  BACKED_UP="$BACKED_UP$f "
}

# The file to READ env values from: .env, or (dry run, no .env yet) the template.
env_read_file() {
  if [ -f .env ]; then printf '.env'; else printf '.env.example'; fi
}

env_get() { # env_get KEY — last assignment wins, like a shell would
  awk -v k="$1" '
    index($0, k "=") == 1 { v = substr($0, length(k) + 2) }
    END { print v }
  ' "$(env_read_file)"
}

env_set() { # env_set KEY VALUE — replace in place or append; backs up .env
  local key="$1" val="$2" cur tmp
  cur="$(env_get "$key")"
  if [ "$cur" = "$val" ] && grep -q "^$key=" "$(env_read_file)" 2>/dev/null; then
    return 0
  fi
  if [ "$DRY_RUN" -eq 1 ]; then
    dry "set $key in .env"
    return 0
  fi
  backup_file .env
  tmp="$(mktemp "${TMPDIR:-/tmp}/tinkerview-env.XXXXXX")"
  TMPFILES+=("$tmp")
  if grep -q "^$key=" .env; then
    KEY="$key" VAL="$val" awk '
      index($0, ENVIRON["KEY"] "=") == 1 { print ENVIRON["KEY"] "=" ENVIRON["VAL"]; next }
      { print }
    ' .env >"$tmp"
  else
    cat .env >"$tmp"
    [ -z "$(tail -c1 .env)" ] || printf '\n' >>"$tmp"
    printf '%s=%s\n' "$key" "$val" >>"$tmp"
  fi
  cat "$tmp" >.env # keep the existing file's owner and mode
  rm -f "$tmp"
  info "set $key in .env"
}

quote_cmd() { # shell-quoted, space separated, no trailing space
  local out
  out="$(printf '%q ' "$@")"
  printf '%s' "${out% }"
}

# ------------------------------------------------------------ 1. prerequisites --
RUNTIME=""
COMPOSE=()

step "Checking prerequisites"

detect_unraid() { [ -f /etc/unraid-version ]; }
if detect_unraid; then
  info "Unraid detected. Run this from your appdata share (for example"
  info "/mnt/user/appdata/tinkerview) — see unraid/README.md for the plugin route."
fi

compose_hint() {
  cat >&2 <<'EOF'

  Install a container runtime and a compose provider, then re-run this script:

    Debian / Ubuntu / Raspberry Pi OS:
      Docker Engine + compose plugin: https://docs.docker.com/engine/install/
      sudo usermod -aG docker "$USER"               # then log out and back in
    Fedora / RHEL:      sudo dnf install docker-compose-plugin  (or: dnf install podman docker-compose)
    Arch / CachyOS:     sudo pacman -S docker docker-compose   (or: pacman -S podman docker-compose)
    Podman:             install 'podman' plus 'docker-compose' (or 'podman-compose'),
                        then enable the socket:  systemctl --user enable --now podman.socket
    Unraid:             Apps -> "Docker Compose Manager" (see unraid/README.md)

EOF
}

try_docker() {
  have docker || return 1
  if docker compose version >/dev/null 2>&1; then
    RUNTIME=docker
    COMPOSE=(docker compose)
    return 0
  fi
  if have docker-compose; then
    local v
    v="$(docker-compose version --short 2>/dev/null || true)"
    case "$v" in
      1.*)
        warn "docker-compose $v is the obsolete v1; install the compose v2 plugin."
        return 1
        ;;
    esac
    RUNTIME=docker
    COMPOSE=(docker-compose)
    return 0
  fi
  return 1
}

try_podman() {
  have podman || return 1
  if have podman-compose; then
    RUNTIME=podman
    COMPOSE=(podman-compose)
    return 0
  fi
  # 'podman compose' only forwards to a provider (docker-compose / podman-compose).
  if podman compose version >/dev/null 2>&1; then
    RUNTIME=podman
    COMPOSE=(podman compose)
    return 0
  fi
  return 1
}

if try_docker || try_podman; then
  ok "container runtime: $RUNTIME, compose: ${COMPOSE[*]}"
else
  if have docker; then
    err "docker is installed but no compose provider was found ('docker compose' / 'docker-compose')."
  elif have podman; then
    err "podman is installed but has no compose provider ('podman-compose' or 'docker-compose')."
  else
    err "neither docker nor podman was found."
  fi
  compose_hint
  if [ "$DRY_RUN" -eq 1 ]; then
    warn "continuing the dry run without a runtime; a real run would stop here."
    RUNTIME="docker"
    COMPOSE=(docker compose)
  else
    exit 1
  fi
fi

# Can we talk to the daemon?
if [ "$DRY_RUN" -eq 0 ] || have "$RUNTIME"; then
  if have "$RUNTIME" && ! "$RUNTIME" info >/dev/null 2>&1; then
    if [ "$(id -u)" -ne 0 ] && [ "$RUNTIME" = docker ]; then
      msg="cannot talk to the Docker daemon as $(id -un). Add yourself to the docker group (sudo usermod -aG docker $(id -un), then log in again) or run this script with sudo."
    else
      msg="'$RUNTIME info' failed — is the daemon/socket running?"
    fi
    if [ "$DRY_RUN" -eq 1 ]; then warn "$msg"; else die "$msg"; fi
  fi
fi

ENGINE_SOCK="" # host path of the engine API socket, when it is not /var/run/docker.sock
if [ "$RUNTIME" = podman ]; then
  if [ ! -S /var/run/docker.sock ]; then
    # Rootless podman keeps its API socket under $XDG_RUNTIME_DIR. docker-socket-proxy
    # mounts whatever DOCKER_SOCKET_PATH points at (see docker-compose.yml).
    cand="$(podman info --format '{{.Host.RemoteSocket.Path}}' 2>/dev/null || true)"
    case "$cand" in unix://*) cand="${cand#unix://}" ;; esac
    [ -S "$cand" ] || cand="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/podman/podman.sock"
    if [ -S "$cand" ]; then
      ENGINE_SOCK="$cand"
      ok "podman: using its API socket $ENGINE_SOCK for docker-socket-proxy (DOCKER_SOCKET_PATH)"
    else
      warn "podman: no API socket found, and /var/run/docker.sock does not exist. The"
      warn "docker-socket-proxy service bind-mounts one, so 'up' will fail until you start it:"
      warn "  systemctl --user enable --now podman.socket   (then re-run this script)"
      warn "or install your distro's 'podman-docker' package."
    fi
  fi
elif [ "$RUNTIME" = docker ] && have docker; then
  # Rootless Docker and Docker Desktop do not listen on /var/run/docker.sock. Ask the CLI
  # which daemon it really talks to (DOCKER_HOST, or the current docker context).
  endpoint="${DOCKER_HOST:-}"
  if [ -z "$endpoint" ]; then
    endpoint="$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || true)"
  fi
  case "$endpoint" in
    unix://*)
      cand="${endpoint#unix://}"
      if [ "$cand" != /var/run/docker.sock ] && [ "$cand" != /run/docker.sock ]; then
        if [ -S "$cand" ]; then
          ENGINE_SOCK="$cand"
          ok "docker: using the daemon socket $ENGINE_SOCK for docker-socket-proxy (DOCKER_SOCKET_PATH)"
        else
          warn "docker: the CLI points at $cand, which is not a socket here."
        fi
      fi
      ;;
    "") ;;
    *)
      warn "docker: the CLI talks to a remote engine ($endpoint). docker-socket-proxy mounts a LOCAL"
      warn "socket, so the container health panel would show the wrong machine. Run this on the"
      warn "engine's own host, or set DOCKER_SOCKET_PATH in .env."
      ;;
  esac
  if [ -z "$ENGINE_SOCK" ] && [ ! -S /var/run/docker.sock ]; then
    warn "docker: /var/run/docker.sock is not a socket. Compose would create a directory there and"
    warn "docker-socket-proxy would fail. Set DOCKER_SOCKET_PATH in .env to your engine's socket"
    warn "(rootless Docker: /run/user/$(id -u)/docker.sock)."
  fi
fi

# SELinux (Fedora, RHEL, Rocky, Alma): enforcing mode blocks containers from reading the
# bind mounts and the engine socket unless they are relabelled or confinement is disabled.
SELINUX_ENFORCING=0
if have getenforce && [ "$(getenforce 2>/dev/null || true)" = Enforcing ]; then
  SELINUX_ENFORCING=1
fi

ARCH="$(uname -m)"

# ---------------------------------------------------------- 2. hardware/preset --
step "Detecting hardware"

SUGGEST=""
SUGGEST_WHY=""
CORAL_FOUND=""
CORAL_KIND=""
HAS_KFD=0
[ -e /dev/kfd ] && HAS_KFD=1

pi_model=""
if [ -r /proc/device-tree/model ]; then
  pi_model="$(tr -d '\0' </proc/device-tree/model 2>/dev/null || true)"
fi

gpu_vendors=""
for v in /sys/class/drm/card[0-9]*/device/vendor; do
  [ -r "$v" ] || continue
  gpu_vendors="$gpu_vendors $(cat "$v" 2>/dev/null || true)"
done

has_nvidia=0
if have nvidia-smi && nvidia-smi -L >/dev/null 2>&1; then
  has_nvidia=1
elif [ -e /proc/driver/nvidia/version ]; then
  has_nvidia=1
fi

case "$gpu_vendors" in *0x8086*) has_intel=1 ;; *) has_intel=0 ;; esac
case "$gpu_vendors" in *0x1002*) has_amd=1 ;; *) has_amd=0 ;; esac

if [ "$has_nvidia" -eq 1 ]; then
  gname="$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -n1 || true)"
  SUGGEST=nvidia
  SUGGEST_WHY="Nvidia GPU${gname:+ ($gname)} found"
elif [ "$ARCH" = aarch64 ] && printf '%s' "$pi_model" | grep -qi 'raspberry pi'; then
  SUGGEST=pi
  SUGGEST_WHY="${pi_model:-Raspberry Pi}"
elif [ "$has_intel" -eq 1 ] && [ -e /dev/dri ]; then
  SUGGEST=intel
  SUGGEST_WHY="Intel GPU with /dev/dri"
elif [ "$has_amd" -eq 1 ] && [ "$HAS_KFD" -eq 1 ]; then
  SUGGEST=amd-rocm
  SUGGEST_WHY="AMD GPU with /dev/dri and /dev/kfd"
else
  SUGGEST=cpu
  if [ "$has_amd" -eq 1 ]; then
    SUGGEST_WHY="AMD GPU seen, but /dev/kfd (ROCm) is missing; CPU is the safe start"
  else
    SUGGEST_WHY="no supported accelerator found"
  fi
fi
info "suggested preset: $SUGGEST ($SUGGEST_WHY)"

# Coral: PCIe node, or USB ids (Unichip 1a6e:089a before first use, Google 18d1:9302 after).
if [ -e /dev/apex_0 ]; then
  CORAL_FOUND="PCIe/M.2 Coral (/dev/apex_0)"
  CORAL_KIND=pci
fi
for d in /sys/bus/usb/devices/*; do
  if [ ! -r "$d/idVendor" ] || [ ! -r "$d/idProduct" ]; then continue; fi
  usbid="$(cat "$d/idVendor" 2>/dev/null || true):$(cat "$d/idProduct" 2>/dev/null || true)"
  case "$usbid" in
    1a6e:089a | 18d1:9302)
      CORAL_FOUND="${CORAL_FOUND:+$CORAL_FOUND, }USB Coral ($usbid)"
      [ -n "$CORAL_KIND" ] || CORAL_KIND=usb
      ;;
  esac
done
if [ -n "$CORAL_FOUND" ]; then
  info "Coral accelerator: $CORAL_FOUND"
else
  info "Coral accelerator: none detected"
fi

# Choose the preset.
if [ -z "$PRESET" ]; then
  if [ "$INTERACTIVE" -eq 1 ]; then
    say ""
    say "    Which hardware preset do you want?"
    i=1
    for p in $PRESETS; do
      mark=""
      [ "$p" = "$SUGGEST" ] && mark="  <- suggested"
      printf '      %d) %s%s\n' "$i" "$p" "$mark"
      i=$((i + 1))
    done
    say "    (cpu is the safe first boot: it works anywhere. Details: frigate/host-presets/README.md)"
    while true; do
      ans="$(ask_text "Preset (name or number)" "$SUGGEST")"
      chosen=""
      i=1
      for p in $PRESETS; do
        if [ "$ans" = "$p" ] || [ "$ans" = "$i" ]; then chosen="$p"; fi
        i=$((i + 1))
      done
      if [ "$ans" = coral ]; then
        chosen=cpu
        CORAL=yes
      fi
      if [ -n "$chosen" ]; then
        PRESET="$chosen"
        break
      fi
      say "    not a valid choice: $ans"
    done
  else
    PRESET="$SUGGEST"
    info "using suggested preset: $PRESET (override with --preset)"
  fi
fi
ok "preset: $PRESET"

# Sanity checks against what we saw.
case "$PRESET" in
  nvidia)
    [ "$has_nvidia" -eq 1 ] || warn "no working nvidia-smi here; the nvidia preset needs the Nvidia driver + container toolkit (Unraid: Nvidia Driver plugin)."
    [ "$RUNTIME" != podman ] || warn "podman: 'runtime: nvidia' (compose.nvidia.yml) needs the CDI/nvidia runtime configured."
    if [ "$RUNTIME" = docker ] && have docker && ! docker info 2>/dev/null | grep -qi nvidia; then
      warn "docker does not list an 'nvidia' runtime; compose.nvidia.yml uses 'runtime: nvidia'."
    fi
    ;;
  intel) [ -e /dev/dri ] || warn "/dev/dri does not exist; compose.intel.yml passes it into the container." ;;
  amd-rocm)
    [ -e /dev/dri ] || warn "/dev/dri does not exist."
    [ "$HAS_KFD" -eq 1 ] || warn "/dev/kfd does not exist; compose.amd-rocm.yml passes it and the container will not start without it."
    ;;
  pi)
    [ "$ARCH" = aarch64 ] || warn "the pi preset expects a 64-bit ARM OS (this is $ARCH)."
    [ -e /dev/dri ] || warn "/dev/dri does not exist."
    ;;
esac

# Coral add-on.
USE_CORAL=0
case "$CORAL" in
  yes) USE_CORAL=1 ;;
  no) USE_CORAL=0 ;;
  *)
    if [ -n "$CORAL_FOUND" ]; then
      if ask_yn "Use the detected Coral for object detection (stacks with $PRESET decode)?" y; then USE_CORAL=1; fi
    fi
    ;;
esac
if [ "$USE_CORAL" -eq 1 ]; then
  ok "Coral EdgeTPU detection enabled"
  if [ "$CORAL_KIND" = pci ]; then
    warn "PCIe/M.2 Coral: compose.coral.yml passes /dev/bus/usb by default. Edit it and"
    warn "uncomment the /dev/apex_0 line before starting (this script does not edit overlays)."
  fi
  if [ -z "$CORAL_FOUND" ]; then
    warn "no Coral was detected on this machine; continuing because you asked for it."
  fi
fi

# ------------------------------------------------------------ choices: image etc --
step "Choosing options"

# Is the prebuilt image (at the tag that matches this checkout) pullable without logging in?
# Needs curl; any failure (offline, package not public yet, tag not built yet) means no.
prebuilt_available() {
  local repo="makerviking/tinkerview" tok
  have curl || return 1
  tok="$(curl -fsS -m 8 "https://ghcr.io/token?scope=repository:$repo:pull" 2>/dev/null |
    sed -n 's/.*"token" *: *"\([^"]*\)".*/\1/p' || true)"
  [ -n "$tok" ] || return 1
  curl -fsS -m 8 -o /dev/null -I -H "Authorization: Bearer $tok" \
    -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json" \
    "https://ghcr.io/v2/$repo/manifests/$PREBUILT_PROBE_TAG" >/dev/null 2>&1
}

if [ -z "$IMAGE_MODE" ]; then
  if prebuilt_available; then
    if [ "$INTERACTIVE" -eq 1 ]; then
      say "    Dashboard image: use the prebuilt multi-arch image ($PREBUILT_IMAGE)"
      say "    or build it from this checkout (slower, always matches the code you cloned)."
      if ask_yn "Use the prebuilt image?" y; then IMAGE_MODE=prebuilt; else IMAGE_MODE=build; fi
    else
      IMAGE_MODE=prebuilt
    fi
  else
    IMAGE_MODE=build
    info "the prebuilt image (ghcr.io/makerviking/tinkerview:$PREBUILT_PROBE_TAG) could not be reached"
    info "(not published yet, or no network/curl), so the dashboard will be built from this checkout."
  fi
fi
ok "dashboard image: $IMAGE_MODE"

# Storage location for recordings.
DEFAULT_STORAGE="$(env_get STORAGE_PATH)"
[ -n "$DEFAULT_STORAGE" ] || DEFAULT_STORAGE="./storage/frigate"
STORAGE_PATH_VAL="$DEFAULT_STORAGE"
if [ -n "$STORAGE_ARG" ]; then
  STORAGE_PATH_VAL="$STORAGE_ARG"
elif [ "$INTERACTIVE" -eq 1 ] && [ ! -f .env ]; then
  if detect_unraid; then
    info "Tip (Unraid): point recordings at an array share, for example /mnt/user/frigate."
  fi
  STORAGE_PATH_VAL="$(ask_text "Where should recordings be stored?" "$DEFAULT_STORAGE")"
fi
ok "recordings: $STORAGE_PATH_VAL"

if [ "$WANT_TOKEN" = auto ]; then
  if [ -n "$(env_get DASHBOARD_TOKEN)" ]; then
    WANT_TOKEN=no # already set in .env
  else
    say "    The dashboard can control printers and cameras. Without an access"
    say "    token, anyone who can reach port 8088 can use it. A token is recommended."
    if ask_yn "Require an access token?" y; then WANT_TOKEN=yes; else WANT_TOKEN=no; fi
  fi
fi

# The Restart button needs POST on the docker-socket-proxy, which can also create containers
# (root-equivalent on the host), so it is an explicit opt-in. Existing .env values win.
if [ "$ALLOW_RESTART" = auto ]; then
  ALLOW_RESTART=no
  if [ "$(env_get DOCKERPROXY_POST)" = 1 ]; then
    ALLOW_RESTART=keep
  elif [ ! -f .env ] && [ "$INTERACTIVE" -eq 1 ]; then
    say "    The dashboard can show container health read-only (default), or also restart"
    say "    containers. Restart needs a Docker API permission that can also create"
    say "    containers, which is root-equivalent for anything that reaches 127.0.0.1:2375."
    if ask_yn "Enable the container Restart button?" n; then ALLOW_RESTART=yes; fi
  fi
fi

# SELinux overlay.
USE_SELINUX=0
case "$SELINUX" in
  yes) USE_SELINUX=1 ;;
  no) USE_SELINUX=0 ;;
  *) USE_SELINUX="$SELINUX_ENFORCING" ;;
esac
if [ "$USE_SELINUX" -eq 1 ]; then
  if [ "$SELINUX_ENFORCING" -eq 1 ]; then
    ok "SELinux is Enforcing: adding compose.selinux.yml (label=disable on the TinkerView containers)"
  else
    ok "adding compose.selinux.yml (label=disable on the TinkerView containers)"
  fi
elif [ "$SELINUX_ENFORCING" -eq 1 ]; then
  warn "SELinux is Enforcing and you chose --no-selinux: the containers will probably be denied"
  warn "access to ./frigate, ./dashboard/data and the engine socket (see docs/TROUBLESHOOTING.md)."
fi

# ---------------------------------------------------------------- 3. .env file --
step "Environment file (.env)"

NEW_TOPIC=""
GEN_TOKEN=""
if [ -f .env ]; then
  ok ".env already exists — keeping it (only the keys listed below may be updated, with a backup)"
else
  if [ "$DRY_RUN" -eq 1 ]; then
    dry "create .env from .env.example"
  else
    cp .env.example .env
    chmod 600 .env
    BACKED_UP="$BACKED_UP.env " # brand new: nothing worth backing up
    ok "created .env from .env.example"
  fi
  tzv=""
  if [ -r /etc/timezone ]; then
    tzv="$(head -n1 /etc/timezone 2>/dev/null || true)"
  fi
  if [ -z "$tzv" ] && [ -L /etc/localtime ]; then
    tzv="$(readlink /etc/localtime 2>/dev/null | sed 's|.*/zoneinfo/||' || true)"
  fi
  case "$tzv" in
    */*) env_set TZ "$tzv" ;;
  esac
fi

[ "$STORAGE_PATH_VAL" = "$(env_get STORAGE_PATH)" ] || env_set STORAGE_PATH "$STORAGE_PATH_VAL"

# Frigate image: Nvidia GPU detection needs the -tensorrt build; other presets do not.
cur_img="$(env_get FRIGATE_IMAGE)"
if [ "$PRESET" = nvidia ]; then
  case "$cur_img" in
    *-tensorrt) ;;
    "") warn "FRIGATE_IMAGE is not set in .env; set it to a -tensorrt tag for Nvidia detection." ;;
    *) env_set FRIGATE_IMAGE "$cur_img-tensorrt" ;;
  esac
else
  case "$cur_img" in
    *-tensorrt)
      # Left over from an earlier nvidia run. It works, but it is a much larger image.
      if [ "$INTERACTIVE" -eq 1 ] && ask_yn "FRIGATE_IMAGE still ends in -tensorrt (Nvidia only, a much larger download). Switch it back?" y; then
        env_set FRIGATE_IMAGE "${cur_img%-tensorrt}"
      else
        info "FRIGATE_IMAGE ends in -tensorrt, which only the nvidia preset needs. Remove the suffix in .env to save space."
      fi
      ;;
  esac
fi

# Dashboard image.
if [ "$IMAGE_MODE" = prebuilt ]; then
  cur_dash="$(env_get DASHBOARD_IMAGE)"
  case "$cur_dash" in
    ghcr.io/makerviking/tinkerview:*) ;;
    *) env_set DASHBOARD_IMAGE "$PREBUILT_IMAGE" ;;
  esac
else
  cur_dash="$(env_get DASHBOARD_IMAGE)"
  case "$cur_dash" in
    ghcr.io/*) env_set DASHBOARD_IMAGE "tinkerview:latest" ;;
  esac
fi

if [ -n "$ENGINE_SOCK" ] && [ "$(env_get DOCKER_SOCKET_PATH)" != "$ENGINE_SOCK" ]; then
  env_set DOCKER_SOCKET_PATH "$ENGINE_SOCK"
fi

[ "$ALLOW_RESTART" != yes ] || env_set DOCKERPROXY_POST 1

if [ "$WANT_TOKEN" = yes ] && [ -z "$(env_get DASHBOARD_TOKEN)" ]; then
  GEN_TOKEN="$(rand_str 32)"
  env_set DASHBOARD_TOKEN "$GEN_TOKEN"
fi

if [ "$TAILSCALE" -eq 1 ]; then
  if [ -n "$(env_get TS_AUTHKEY)" ]; then
    ok "TS_AUTHKEY already set in .env"
  elif [ -n "${TS_AUTHKEY:-}" ]; then
    env_set TS_AUTHKEY "$TS_AUTHKEY"
  else
    warn "--tailscale without TS_AUTHKEY: the sidecar will print a one-time login URL in"
    warn "its logs (docker logs tinkerview-tailscale) that you must open to authorize it."
  fi
fi

# ------------------------------------------------------- compose file selection --
FILES=(docker-compose.yml)
case "$PRESET" in
  nvidia) FILES+=(compose.nvidia.yml) ;;
  intel) FILES+=(compose.intel.yml) ;;
  amd-rocm) FILES+=(compose.amd-rocm.yml) ;;
  cpu) [ "$USE_CORAL" -eq 1 ] || FILES+=(compose.cpu.yml) ;;
  pi) FILES+=(compose.pi.yml) ;;
esac
[ "$USE_CORAL" -eq 1 ] && FILES+=(compose.coral.yml)
[ "$USE_SELINUX" -eq 1 ] && FILES+=(compose.selinux.yml)
[ "$BRIDGE" -eq 1 ] && FILES+=(compose.bridge.yml)
[ "$TAILSCALE" -eq 1 ] && FILES+=(compose.tailscale.yml)
for f in "${FILES[@]}"; do
  [ -f "$f" ] || die "missing compose file: $f"
done

if [ "$BRIDGE" -eq 1 ]; then
  warn "bridge networking needs manual edits this script does not make:"
  warn "  - dashboard/data/config.yaml: reach services by name (frigate, mqtt, ntfy, dockerproxy)"
  warn "  - frigate/config.yml: mqtt host (the service name 'mqtt'), webrtc candidates"
  warn "  Details: the header of compose.bridge.yml."
fi

FILE_ARGS=()
COMPOSE_FILE_VAL=""
for f in "${FILES[@]}"; do
  FILE_ARGS+=(-f "$f")
  COMPOSE_FILE_VAL="${COMPOSE_FILE_VAL:+$COMPOSE_FILE_VAL:}$f"
done
# Lets plain `docker compose up -d` / `logs` / `pull` work from this folder too.
env_set COMPOSE_FILE "$COMPOSE_FILE_VAL"

# --------------------------------------------------------------- 4. data dirs --
step "Data directories"

# The dashboard image is distroless :nonroot (uid 65532). The mosquitto image's entrypoint
# fixes its own data/log ownership; ntfy and Frigate run as root.
chown_dir() { # chown_dir UID:GID PATH [recursive]
  local own="$1" path="$2" rec="${3:-}" flag=() uid
  uid="${own%%:*}"
  [ -n "$rec" ] && flag=(-R)
  if [ "$DRY_RUN" -eq 1 ]; then
    dry "chown ${flag[*]:+${flag[*]} }$own $path"
    return 0
  fi
  if [ "$(id -u)" -eq 0 ]; then
    chown ${flag[@]+"${flag[@]}"} "$own" "$path"
    return 0
  fi
  if [ "$RUNTIME" = podman ] && [ "$(id -u)" -ne 0 ] && podman unshare true >/dev/null 2>&1; then
    # Rootless podman: ids inside the container map through the user namespace.
    podman unshare chown ${flag[@]+"${flag[@]}"} "$own" "$path" && return 0
  fi
  if [ -z "$rec" ] && [ -n "$(find "$path" -maxdepth 0 -user "$uid" 2>/dev/null)" ]; then
    return 0 # already owned by the right uid
  fi
  if have sudo; then
    if [ "$INTERACTIVE" -eq 1 ]; then
      info "need root to set the owner of $path (sudo may ask for your password)"
      if sudo chown ${flag[@]+"${flag[@]}"} "$own" "$path"; then return 0; fi
    elif sudo -n true 2>/dev/null; then
      if sudo -n chown ${flag[@]+"${flag[@]}"} "$own" "$path"; then return 0; fi
    fi
  fi
  local manual="sudo chown ${flag[*]:+${flag[*]} }$own $path"
  warn "could not set the owner of $path. Run:  $manual"
  PENDING+=("$manual")
  return 1
}

make_dir() { # make_dir PATH [UID:GID]
  local d="$1" own="${2:-}"
  if [ -d "$d" ]; then
    info "exists: $d"
  elif [ "$DRY_RUN" -eq 1 ]; then
    dry "mkdir -p $d"
  else
    mkdir -p "$d"
    info "created: $d"
  fi
  [ -z "$own" ] || chown_dir "$own" "$d" || true
}

if [ -d dashboard/config.yaml ]; then
  warn "dashboard/config.yaml is a directory (left over from an old bind-mount). The dashboard"
  warn "no longer mounts it; remove it with:  rm -rf dashboard/config.yaml"
fi

make_dir dashboard/data
if [ ! -e dashboard/data/config.yaml ]; then
  NEW_TOPIC="tinkerview-$(rand_str 12)"
  if [ "$DRY_RUN" -eq 1 ]; then
    dry "write dashboard/data/config.yaml (commented template + a random ntfy topic)"
  else
    {
      if [ -f dashboard/internal/config/template.yaml ]; then
        cat dashboard/internal/config/template.yaml
        printf '\n'
      else
        printf '# TinkerView dashboard config (edit, then restart the dashboard container).\n'
      fi
      printf '# Generated by install.sh: a random, unguessable ntfy topic for push notifications.\n'
      printf 'ntfy:\n  topic: "%s"\n' "$NEW_TOPIC"
    } >dashboard/data/config.yaml
    chmod 644 dashboard/data/config.yaml
    ok "wrote dashboard/data/config.yaml with ntfy topic $NEW_TOPIC"
  fi
else
  info "keeping existing dashboard/data/config.yaml"
fi
# Only the folder changes owner. config.yaml stays yours so you can edit it without sudo; the
# dashboard only reads it (it writes devices.yaml and its other state next to it).
chown_dir 65532:65532 dashboard/data || true

make_dir ntfy
make_dir mosquitto/data
make_dir mosquitto/log
make_dir "$STORAGE_PATH_VAL"
[ "$TAILSCALE" -eq 0 ] || make_dir tailscale/state

# ------------------------------------------------- 5. frigate host-preset block --
step "Frigate host preset (frigate/config.yml)"

CFG="frigate/config.yml"
PRESET_FILE="frigate/host-presets/$PRESET.yml"
[ -f "$PRESET_FILE" ] || die "missing $PRESET_FILE"

# Print the YAML block of a top-level key (detectors / model / ffmpeg) from a preset
# file: from "key:" until the next top-level key or a column-0 comment. Comments inside
# the block are kept; commented-out alternatives at column 0 are not.
extract_block() { # extract_block FILE KEY
  awk -v key="$2" '
    $0 ~ "^" key ":" { on = 1; print; next }
    on && /^[^ \t]/ { on = 0 }
    on { print }
  ' "$1" | awk '{ lines[NR] = $0 } END { n = NR; while (n > 0 && lines[n] ~ /^[ \t]*$/) n--; for (i = 1; i <= n; i++) print lines[i] }'
}

BLOCK_TMP="$(mktemp "${TMPDIR:-/tmp}/tinkerview-block.XXXXXX")"
CAND_TMP="$(mktemp "${TMPDIR:-/tmp}/tinkerview-cfg.XXXXXX")"
TMPFILES+=("$BLOCK_TMP" "$CAND_TMP")

build_block() {
  local label="$PRESET"
  [ "$USE_CORAL" -eq 1 ] && label="$PRESET + coral"
  {
    printf '%s %s >>> managed by install.sh. Re-running it replaces everything between\n' "$PRESET_BEGIN" "$label"
    printf '# these markers (a backup is kept). To take over manually, delete both marker lines.\n'
    if [ "$USE_CORAL" -eq 1 ]; then
      printf '# Sources: frigate/host-presets/coral.yml (detector) + frigate/host-presets/%s.yml (decode).\n' "$PRESET"
      if [ "$CORAL_KIND" = pci ]; then
        extract_block frigate/host-presets/coral.yml detectors | awk '{ sub(/device: usb/, "device: pci"); print }'
      else
        extract_block frigate/host-presets/coral.yml detectors
      fi
      if [ "$PRESET" != cpu ]; then
        extract_block "$PRESET_FILE" ffmpeg
      fi
    else
      printf '# Source: frigate/host-presets/%s.yml (see its header for model/image notes).\n' "$PRESET"
      for k in detectors model ffmpeg; do
        extract_block "$PRESET_FILE" "$k"
        # blank line between blocks is added below
      done
    fi
    printf '%s <<<\n' "$PRESET_END"
  } >"$BLOCK_TMP"
}

# Re-emit with blank lines between top-level blocks for readability.
tidy_block() {
  awk '
    /^[a-z]/ && NR > 2 && prev !~ /^[ \t]*$/ && prev !~ /^#/ { print "" }
    { print; prev = $0 }
  ' "$BLOCK_TMP" >"$CAND_TMP" && cat "$CAND_TMP" >"$BLOCK_TMP"
}

build_block
tidy_block

if [ ! -f "$CFG" ]; then
  warn "$CFG not found; skipping the preset step."
else
  has_marker=0
  has_legacy=0
  grep -q "^$PRESET_BEGIN" "$CFG" && has_marker=1
  grep -Eq '^# =+ HOST-SPECIFIC BLOCK' "$CFG" && has_legacy=1

  if [ "$has_marker" -eq 0 ] && [ "$has_legacy" -eq 0 ]; then
    warn "no HOST-SPECIFIC block or tinkerview markers found in $CFG — assuming you configured"
    warn "detectors/ffmpeg by hand. Reference: $PRESET_FILE"
  else
    if [ "$has_marker" -eq 1 ]; then
      start_re="^$PRESET_BEGIN"
      end_re="^$PRESET_END"
    else
      start_re='^# =+ HOST-SPECIFIC BLOCK'
      end_re='^# =+[ \t]*$'
    fi
    awk -v start_re="$start_re" -v end_re="$end_re" -v blockfile="$BLOCK_TMP" '
      state == 0 && $0 ~ start_re {
        while ((getline line < blockfile) > 0) print line
        close(blockfile)
        state = 1
        next
      }
      state == 1 {
        if ($0 ~ end_re) state = 2
        next
      }
      { print }
    ' "$CFG" >"$CAND_TMP"

    # Refuse to produce a duplicate top-level key (invalid YAML for Frigate).
    dup=""
    for k in detectors model ffmpeg; do
      n_new="$(grep -c "^$k:" "$BLOCK_TMP" || true)"
      [ "$n_new" -gt 0 ] || continue
      n_all="$(grep -c "^$k:" "$CAND_TMP" || true)"
      [ "$n_all" -le "$n_new" ] || dup="$dup $k"
    done

    yaml_ok=1
    yaml_checked=0
    if have python3 && python3 -c 'import yaml' >/dev/null 2>&1; then
      yaml_checked=1
      python3 -I -c 'import sys, yaml; yaml.safe_load(open(sys.argv[1]))' "$CAND_TMP" >/dev/null 2>&1 || yaml_ok=0
    fi

    if [ -n "$dup" ]; then
      warn "$CFG already defines top-level key(s)${dup} outside the host block; not changing it."
      warn "Merge $PRESET_FILE by hand."
    elif [ "$yaml_ok" -eq 0 ]; then
      warn "the merged config would not be valid YAML; leaving $CFG untouched."
      warn "Merge $PRESET_FILE by hand."
    elif cmp -s "$CFG" "$CAND_TMP"; then
      ok "$CFG already has the $PRESET preset — nothing to do"
    elif [ "$DRY_RUN" -eq 1 ]; then
      dry "write the '$PRESET' host-preset block into $CFG (backup first)"
      sed 's/^/        | /' "$BLOCK_TMP"
    elif [ ! -w "$CFG" ]; then
      warn "$CFG is not writable by $(id -un) (Frigate may have rewritten it as root)."
      warn "Re-run with sudo, or: sudo chown $(id -un) $CFG"
    else
      backup_file "$CFG"
      cat "$CAND_TMP" >"$CFG"
      ok "wrote the $PRESET host-preset block into $CFG"
      [ "$yaml_checked" -eq 1 ] && info "merged file parses as valid YAML"
    fi
  fi
fi

# Model files the preset needs but the Frigate image does not ship.
MODEL_MISSING=""
if [ -f "$PRESET_FILE" ] && [ "$USE_CORAL" -eq 0 ]; then
  mpath="$(extract_block "$PRESET_FILE" model | awk '/^[ \t]+path:[ \t]*\/config\// { print $2; exit }')"
  if [ -n "$mpath" ]; then
    local_path="frigate/${mpath#/config/}"
    if [ ! -e "$local_path" ]; then
      MODEL_MISSING="$local_path"
      warn "the $PRESET preset needs a detection model that is NOT bundled: $local_path"
      warn "Export it first (the command is in the header of $PRESET_FILE), or start with"
      warn "'--preset cpu' to confirm cameras work, then switch."
    fi
  fi
fi

# ------------------------------------------------------------ port conflicts ---
check_ports() {
  have ss || return 0
  if have "$RUNTIME" && "$RUNTIME" ps --format '{{.Names}}' 2>/dev/null | grep -qx frigate; then
    return 0 # a previous TinkerView run owns these ports
  fi
  local listening p busy=""
  listening="$(ss -ltn 2>/dev/null | awk 'NR > 1 { print $4 }' || true)"
  for p in 5000 5001 8971 1984 8554 8555 8088 8092 1883 2375; do
    if printf '%s\n' "$listening" | grep -Eq "[:.]$p\$"; then busy="$busy $p"; fi
  done
  if [ -n "$busy" ]; then
    warn "ports already in use on this host:$busy"
    warn "Frigate/ntfy/dashboard use host networking, so a busy port crash-loops them."
    warn "Find the owner with 'ss -lntp | grep :PORT', stop it, or re-run with --bridge."
  fi
}
step "Port check"
check_ports
info "done"

# ---------------------------------------------------------- 6. the command ------
step "Start command"

CMD_UP=("${COMPOSE[@]}" "${FILE_ARGS[@]}")
if [ "$IMAGE_MODE" = build ]; then
  UP_TAIL=(up -d --build)
else
  UP_TAIL=(up -d)
fi
PULL_CMD=("${COMPOSE[@]}" "${FILE_ARGS[@]}" pull)
FULL_CMD=("${CMD_UP[@]}" "${UP_TAIL[@]}")

if [ "$IMAGE_MODE" = prebuilt ]; then
  say "    $(quote_cmd "${PULL_CMD[@]}")"
fi
say "    $(quote_cmd "${FULL_CMD[@]}")"
info "(this folder's .env also records COMPOSE_FILE, so plain '${COMPOSE[*]} up -d' / 'logs' / 'ps' work here)"

DO_START=0
case "$START" in
  yes) DO_START=1 ;;
  no) DO_START=0 ;;
  *)
    BLOCKER=""
    [ -z "$MODEL_MISSING" ] || BLOCKER="the detection model is missing, so Frigate will not start cleanly"
    if [ "${#PENDING[@]}" -gt 0 ]; then
      BLOCKER="${BLOCKER:+$BLOCKER; }the dashboard data directory is not writable by the container yet"
    fi
    if [ -n "$BLOCKER" ]; then
      if [ "$INTERACTIVE" -eq 1 ]; then
        if ask_yn "Heads-up: $BLOCKER. Start anyway?" n; then DO_START=1; fi
      else
        info "not starting automatically: $BLOCKER (fix it, or use --start to force)."
      fi
    elif ask_yn "Start TinkerView now?" y; then
      DO_START=1
    fi
    ;;
esac

if [ "$DO_START" -eq 1 ]; then
  if [ "$DRY_RUN" -eq 1 ]; then
    [ "$IMAGE_MODE" = prebuilt ] && dry "run: $(quote_cmd "${PULL_CMD[@]}")"
    dry "run: $(quote_cmd "${FULL_CMD[@]}")"
  else
    if [ "$IMAGE_MODE" = prebuilt ]; then
      info "pulling images (the first time takes a few minutes)..."
      "${PULL_CMD[@]}" || die "image pull failed. Check your network, or re-run with --build to build the dashboard locally."
    fi
    info "starting the stack..."
    "${FULL_CMD[@]}" || die "compose failed. See the output above; 'docker compose ps' and 'logs' will help."
    ok "containers started"
  fi
fi

# ------------------------------------------------------------- summary ---------
host_ip=""
if have hostname; then
  host_ip="$(hostname -I 2>/dev/null | awk '{ print $1 }' || true)"
fi
if [ -z "$host_ip" ] && have ip; then
  host_ip="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "src") { print $(i + 1); exit } }' || true)"
fi
[ -n "$host_ip" ] || host_ip="<this-host>"

up=0
verified=1 # 0 when neither curl nor wget is there to check
if [ "$DO_START" -eq 1 ] && [ "$DRY_RUN" -eq 0 ]; then
  step "Waiting for the dashboard"
  n=0
  while [ "$n" -lt 30 ]; do
    if have curl && curl -fsS -m 2 "http://127.0.0.1:8088/healthz" >/dev/null 2>&1; then
      up=1
      break
    elif have wget && wget -q -T 2 -O /dev/null "http://127.0.0.1:8088/healthz" 2>/dev/null; then
      up=1
      break
    elif ! have curl && ! have wget; then
      verified=0
      break
    fi
    n=$((n + 1))
    sleep 2
  done
  if [ "$up" -eq 1 ]; then
    ok "dashboard is answering on :8088"
  elif have curl || have wget; then
    warn "the dashboard did not answer within 60s. Check:  ${COMPOSE[*]} logs dashboard"
  else
    info "(install curl to have this script verify the dashboard)"
  fi
fi

tok_q=""
[ -n "$GEN_TOKEN" ] && [ "$DRY_RUN" -eq 0 ] && tok_q="?token=$GEN_TOKEN"

if [ "$DO_START" -eq 1 ] && [ "$DRY_RUN" -eq 0 ]; then
  if [ "$up" -eq 1 ]; then
    STATE="running"
  elif [ "$verified" -eq 1 ]; then
    STATE="started, but the dashboard is not answering yet"
  else
    STATE="started"
  fi
else
  STATE="ready"
fi
step "TinkerView is $STATE"
cat <<EOF

    Dashboard        http://$host_ip:8088/$tok_q
    Frigate          http://$host_ip:5000        (authenticated UI: https://$host_ip:8971)
    go2rtc           http://$host_ip:1984        (per-camera stream health)
    ntfy             http://$host_ip:8092        (push notifications)
    MQTT (Mosquitto) port 1883           (Home Assistant: see README)

    Preset: $PRESET$([ "$USE_CORAL" -eq 1 ] && printf ' + coral')   Dashboard image: $IMAGE_MODE
    Compose: $(quote_cmd "${CMD_UP[@]}")
EOF
if [ -n "$GEN_TOKEN" ] && [ "$DRY_RUN" -eq 0 ]; then
  say ""
  say "    Dashboard token (stored in .env, open the URL above once per browser):"
  say "      $GEN_TOKEN"
elif [ "$WANT_TOKEN" = yes ] && [ "$DRY_RUN" -eq 1 ]; then
  say ""
  say "    A dashboard token would be generated and stored in .env on a real run."
fi
if [ -n "$NEW_TOPIC" ] && [ "$DRY_RUN" -eq 0 ]; then
  say ""
  say "    ntfy topic for your phone: $NEW_TOPIC  (in dashboard/data/config.yaml)"
elif [ -n "$NEW_TOPIC" ]; then
  say ""
  say "    A random ntfy topic would be written to dashboard/data/config.yaml on a real run."
fi
if [ "$(env_get DOCKERPROXY_POST)" = 1 ] || [ "$ALLOW_RESTART" = yes ]; then
  say ""
  say "    The container Restart button is enabled (DOCKERPROXY_POST=1). Read SECURITY.md first."
fi
cat <<EOF

  Next steps
    1. Edit frigate/config.yml: point the go2rtc streams at your cameras, then set
       'enabled: true' on that camera (the two examples ship disabled).
       Camera credentials go in .env (FRIGATE_RTSP_USER / FRIGATE_RTSP_PASSWORD).
       Apply:  ${COMPOSE[*]} restart frigate
    2. Open the dashboard and add your printers and cameras from the UI.
    3. Frigate prints a generated admin password in its log on first start:
       ${COMPOSE[*]} logs frigate | grep -i password
    4. Optional: ntfy tokens, Home Assistant MQTT, Tailscale. See README.md.
    Backups of anything this script changed: storage/install-backups/
EOF
if [ "${#PENDING[@]}" -gt 0 ]; then
  say ""
  say "    Still to do by hand (needs root):"
  for c in "${PENDING[@]}"; do say "      $c"; done
fi
if [ -n "$MODEL_MISSING" ]; then
  say ""
  say "    Remember: $MODEL_MISSING (detection model) is still missing. See $PRESET_FILE."
fi
if [ "$DRY_RUN" -eq 1 ]; then
  say ""
  say "${C_BOLD}Dry run complete: nothing was changed.${C_RESET}"
fi
