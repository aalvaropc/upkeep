#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: deploy/deploy.sh USER@HOST [HOSTNAME]" >&2
  echo "       deploy/deploy.sh --rollback USER@HOST" >&2
  exit 2
}

[ $# -ge 1 ] || usage

remote_run() {
  ssh -o StrictHostKeyChecking=accept-new "$target" "$@"
}

if [ "$1" = "--rollback" ]; then
  [ $# -eq 2 ] || usage
  target=$2
  remote_run 'set -euo pipefail
    cd /opt/upkeep/releases
    current=$(basename "$(readlink -f /opt/upkeep/current)")
    previous=$(ls -1t | grep -vx "$current" | head -1)
    [ -n "$previous" ] || { echo "there is no earlier release to go back to" >&2; exit 1; }
    sudo ln -sfn "/opt/upkeep/releases/$previous" /opt/upkeep/current
    sudo systemctl restart upkeep
    echo "rolled back from $current to $previous"'
  exit 0
fi

target=$1
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)

branch=$(git -C "$root" rev-parse --abbrev-ref HEAD)
[ "$branch" = "main" ] || { echo "deploy only from main, not from $branch" >&2; exit 1; }
[ -z "$(git -C "$root" status --porcelain)" ] || { echo "the working tree has changes; commit or stash them" >&2; exit 1; }

host=${2:-}
if [ -z "$host" ]; then
  address=${target#*@}
  host="${address//./-}.sslip.io"
fi

release=$(git -C "$root" rev-parse --short=12 HEAD)
echo "building $release for linux/amd64"
make -C "$root" web
(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$root/bin/upkeep-linux-amd64" ./cmd/upkeep)

echo "shipping to $target"
scp -o StrictHostKeyChecking=accept-new "$root/bin/upkeep-linux-amd64" "$target:/tmp/upkeep-$release"
scp "$here/upkeep.service" "$target:/tmp/upkeep.service"
scp "$here/Caddyfile" "$target:/tmp/Caddyfile"

remote_run "set -euo pipefail
  sudo install -d -o root -g root /opt/upkeep/releases/$release
  sudo install -m 0755 /tmp/upkeep-$release /opt/upkeep/releases/$release/upkeep
  rm -f /tmp/upkeep-$release
  sudo install -m 0644 /tmp/upkeep.service /etc/systemd/system/upkeep.service
  sudo install -m 0644 /tmp/Caddyfile /etc/caddy/Caddyfile
  sudo mkdir -p /etc/systemd/system/caddy.service.d
  printf '[Service]\nEnvironment=UPKEEP_HOST=$host\n' | sudo tee /etc/systemd/system/caddy.service.d/host.conf >/dev/null
  rm -f /tmp/upkeep.service /tmp/Caddyfile
  previous=\$(readlink -f /opt/upkeep/current || true)
  sudo ln -sfn /opt/upkeep/releases/$release /opt/upkeep/current
  sudo systemctl daemon-reload
  sudo systemctl enable upkeep caddy >/dev/null
  sudo systemctl restart upkeep
  sudo systemctl reload-or-restart caddy
  for i in \$(seq 1 60); do
    if curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
      echo healthy
      ls -1dt /opt/upkeep/releases/* | tail -n +4 | xargs -r sudo rm -rf
      exit 0
    fi
    sleep 2
  done
  echo 'the new release did not become healthy; going back' >&2
  if [ -n \"\$previous\" ]; then
    sudo ln -sfn \"\$previous\" /opt/upkeep/current
    sudo systemctl restart upkeep
  fi
  exit 1"

echo "released $release at https://$host"
