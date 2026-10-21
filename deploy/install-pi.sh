#!/usr/bin/env bash
# Install or upgrade the gateway on a Raspberry Pi.
# Run on the Pi from the repo root after copying bin/arm64 (or bin/armv7) across:
#   sudo ./deploy/install-pi.sh bin/arm64/gpstrack-gateway
set -euo pipefail

BIN="${1:?usage: sudo $0 path/to/gpstrack-gateway}"

id gpstrack >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin gpstrack
install -d -m 755 /opt/gpstrack
install -m 755 "$BIN" /opt/gpstrack/gpstrack-gateway
install -d -m 750 -o gpstrack -g gpstrack /var/lib/gpstrack
install -d -m 750 /etc/gpstrack

if [ ! -f /etc/gpstrack/gpstrack.env ]; then
  install -m 600 deploy/gpstrack.env.example /etc/gpstrack/gpstrack.env
  sed -i "s/^GPSTRACK_TOKEN=.*/GPSTRACK_TOKEN=$(openssl rand -hex 16)/" /etc/gpstrack/gpstrack.env
  echo "Created /etc/gpstrack/gpstrack.env with a new token:"
  grep GPSTRACK_TOKEN /etc/gpstrack/gpstrack.env
fi

install -m 644 deploy/gpstrack-gateway.service /etc/systemd/system/gpstrack-gateway.service
systemctl daemon-reload
systemctl enable --now gpstrack-gateway
systemctl restart gpstrack-gateway
systemctl --no-pager status gpstrack-gateway | head -5
