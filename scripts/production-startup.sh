#!/bin/bash
# launchd entrypoint for MagicPodcast production startup.

set -euo pipefail

PROJECT_DIR="${MAGICPODCAST_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
LOG_FILE="${MAGICPODCAST_STARTUP_LOG:-/tmp/magicpodcast-production.log}"
STARTUP_DELAY="${MAGICPODCAST_STARTUP_DELAY:-3}"

mkdir -p "$(dirname "$LOG_FILE")"
exec >> "$LOG_FILE" 2>&1

log() {
  echo "$(date '+%Y-%m-%d %H:%M:%S') - $1"
}

is_backend_healthy() {
  curl -fsS http://localhost:8080/health >/dev/null 2>&1
}

is_frontend_healthy() {
  curl -fsS http://localhost:3000 >/dev/null 2>&1
}

port_is_listening() {
  local port="$1"
  lsof -ti :"$port" -sTCP:LISTEN >/dev/null 2>&1
}

log "=== MagicPodcast launchd startup begin ==="

cd "$PROJECT_DIR"

sleep "$STARTUP_DELAY"

if is_backend_healthy && is_frontend_healthy; then
  log "Services already healthy; startup skipped."
  log "=== MagicPodcast launchd startup complete ==="
  exit 0
fi

if port_is_listening 8080 || port_is_listening 3000; then
  log "Existing listener detected but health check is incomplete; restarting production services."
  "$PROJECT_DIR/scripts/restart.sh" --prod
else
  log "No existing listeners detected; starting production services."
  "$PROJECT_DIR/scripts/start.sh" --prod
fi

log "=== MagicPodcast launchd startup complete ==="
