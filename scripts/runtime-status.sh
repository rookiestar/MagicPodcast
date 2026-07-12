#!/bin/bash
# Read-only status for MagicPodcast service supervision and dependencies.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH
PROJECT_DIR="${MAGICPODCAST_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
uid="$(id -u)"

print_job() {
  local label="$1"
  if launchctl print "gui/$uid/$label" >/dev/null 2>&1; then
    echo "$label=loaded"
    launchctl print "gui/$uid/$label" | grep -E 'state =|runs =|last exit code|path =|program =' | head -8 || true
  else
    echo "$label=not_loaded"
  fi
}

echo "project_root=$PROJECT_DIR"
print_job com.magicpodcast.supervisor
print_job com.magicpodcast.backup
print_job com.cloudflare.cloudflared

for port in 3000 8080 8088; do
  listener="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -n "$listener" ]; then
    echo "port_$port=listening"
    echo "$listener" | sed -n '1,4p'
  else
    echo "port_$port=not_listening"
  fi
done

live="$(curl --silent --show-error --max-time 5 http://127.0.0.1:8080/live 2>/dev/null || true)"
ready="$(curl --silent --show-error --max-time 5 http://127.0.0.1:8080/ready 2>/dev/null || true)"
echo "live=${live:-unavailable}"
echo "ready=${ready:-unavailable}"

if [ -f "$PROJECT_DIR/logs/supervisor.status" ]; then
  echo "supervisor_status:"
  sed -n '1,20p' "$PROJECT_DIR/logs/supervisor.status"
else
  echo "supervisor_status=missing"
fi
