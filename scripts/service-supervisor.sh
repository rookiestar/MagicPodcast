#!/bin/bash
# Keep the verified MagicPodcast production pair healthy under macOS launchd.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${MAGICPODCAST_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
source "$SCRIPT_DIR/production-maintenance.sh"
LOG_FILE="${MAGICPODCAST_SUPERVISOR_LOG:-$PROJECT_DIR/logs/supervisor.log}"
STATUS_FILE="${MAGICPODCAST_SUPERVISOR_STATUS_FILE:-$PROJECT_DIR/logs/supervisor.status}"
CHECK_INTERVAL="${MAGICPODCAST_SUPERVISOR_INTERVAL:-15}"
MAX_BACKOFF="${MAGICPODCAST_SUPERVISOR_MAX_BACKOFF:-600}"
NO_BUILD="${MAGICPODCAST_SUPERVISOR_NO_BUILD:-true}"
MAX_CYCLES="${MAGICPODCAST_SUPERVISOR_MAX_CYCLES:-0}"
CURL_BIN="${MAGICPODCAST_CURL_BIN:-curl}"
START_SCRIPT="${MAGICPODCAST_START_SCRIPT:-$PROJECT_DIR/scripts/start.sh}"
STOP_SCRIPT="${MAGICPODCAST_STOP_SCRIPT:-$PROJECT_DIR/scripts/stop.sh}"
SUPERVISE_TUNNEL="${MAGICPODCAST_SUPERVISE_TUNNEL:-false}"

BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_DIR="$PROJECT_DIR/frontend"
CURRENT_RELEASE_FILE="$PROJECT_DIR/.magicpodcast-releases/current.env"

fail() {
  echo "supervisor failed: $1" >&2
  exit 1
}

if ! [[ "$CHECK_INTERVAL" =~ ^[0-9]+$ ]] || [ "$CHECK_INTERVAL" -lt 1 ]; then
  fail "MAGICPODCAST_SUPERVISOR_INTERVAL must be a positive integer"
fi
if ! [[ "$MAX_BACKOFF" =~ ^[0-9]+$ ]] || [ "$MAX_BACKOFF" -lt 1 ]; then
  fail "MAGICPODCAST_SUPERVISOR_MAX_BACKOFF must be a positive integer"
fi
if ! [[ "$MAX_CYCLES" =~ ^[0-9]+$ ]]; then
  fail "MAGICPODCAST_SUPERVISOR_MAX_CYCLES must be a non-negative integer"
fi

mkdir -p "$(dirname "$LOG_FILE")" "$(dirname "$STATUS_FILE")"

rotate_log() {
  local max_bytes=1048576
  local size=0
  if [ -f "$LOG_FILE" ]; then
    size="$(wc -c < "$LOG_FILE" | tr -d ' ')"
  fi
  if [ "$size" -ge "$max_bytes" ]; then
    rm -f "$LOG_FILE.2"
    [ -f "$LOG_FILE.1" ] && mv "$LOG_FILE.1" "$LOG_FILE.2"
    mv "$LOG_FILE" "$LOG_FILE.1"
  fi
}

rotate_log
exec >> "$LOG_FILE" 2>&1

log() {
  echo "$(date -u '+%Y-%m-%dT%H:%M:%SZ') $*"
}

manifest_value() {
  local key="$1"
  [ -f "$CURRENT_RELEASE_FILE" ] || return 0
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$CURRENT_RELEASE_FILE"
}

write_status() {
  local state="$1"
  local failure_count="$2"
  local backend="$3"
  local frontend="$4"
  local tunnel="$5"
  local maintenance_info="${6:-}"
  local maintenance_owner_pid=""
  local maintenance_started_at=""
  local maintenance_operation=""
  if [ "$state" = maintenance ]; then
    maintenance_owner_pid="$(printf '%s\n' "$maintenance_info" | awk -F= '$1 == "owner_pid" { print $2; exit }')"
    maintenance_started_at="$(printf '%s\n' "$maintenance_info" | awk -F= '$1 == "started_at" { print $2; exit }')"
    maintenance_operation="$(printf '%s\n' "$maintenance_info" | awk -F= '$1 == "operation" { print $2; exit }')"
  fi
  umask 077
  cat > "$STATUS_FILE" <<EOF
state=$state
failure_count=$failure_count
backend=$backend
frontend=$frontend
tunnel=$tunnel
updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
maintenance_owner_pid=$maintenance_owner_pid
maintenance_started_at=$maintenance_started_at
maintenance_operation=$maintenance_operation
EOF
}

probe_backend() {
  local health
  health="$("$CURL_BIN" --fail --silent --show-error --max-time 5 http://127.0.0.1:8080/ready 2>/dev/null)" || return 1
  printf '%s\n' "$health" | grep -Fq '"status":"ok"' || return 1
  printf '%s\n' "$health" | grep -Fq '"data_profile":"production"'
}

probe_frontend() {
  "$CURL_BIN" --fail --silent --show-error --max-time 5 http://127.0.0.1:3000 >/dev/null 2>&1
}

rotate_service_log() {
  local file="$1"
  local max_bytes=5242880
  local size=0
  [ -f "$file" ] || return 0
  size="$(wc -c < "$file" | tr -d ' ')"
  if [ "$size" -ge "$max_bytes" ]; then
    cp "$file" "$file.1"
    : > "$file"
  fi
}

rotate_service_logs() {
  rotate_service_log "$PROJECT_DIR/logs/backend.log"
  rotate_service_log "$PROJECT_DIR/logs/frontend.log"
}

tunnel_state() {
  if [ "$SUPERVISE_TUNNEL" != true ]; then
    echo disabled
  elif launchctl print "gui/$(id -u)/com.cloudflare.cloudflared" >/dev/null 2>&1; then
    echo loaded
  else
    echo unavailable
  fi
}

load_release_metadata() {
  local release_id frontend_build_id
  release_id="$(manifest_value release_id)"
  frontend_build_id="$(manifest_value frontend_build_id)"
  [ -n "$release_id" ] && export MAGICPODCAST_RELEASE_ID="$release_id"
  [ -n "$frontend_build_id" ] && export MAGICPODCAST_FRONTEND_BUILD_ID="$frontend_build_id"
}

restart_verified_pair() {
  # A supervisor cycle may have started its health probe just before a
  # publisher claimed the maintenance window. Re-check immediately before
  # invoking stop.sh so a stale recovery branch cannot interrupt a release.
  if production_maintenance_inspect >/dev/null 2>&1; then
    log "maintenance window claimed during health probe; skipping recovery"
    return 1
  fi

  load_release_metadata
  if [ "$NO_BUILD" = true ] && [ -x "$BACKEND_DIR/api" ] && [ -f "$FRONTEND_DIR/.next/BUILD_ID" ]; then
    "$STOP_SCRIPT"
    "$START_SCRIPT" --prod --no-build
  else
    "$START_SCRIPT" --prod
  fi
}

check_once() {
  local failure_count="$1"
  local backend="down"
  local frontend="down"

  if probe_backend; then backend="ready"; fi
  if probe_frontend; then frontend="ready"; fi

  if [ "$backend" = ready ] && [ "$frontend" = ready ]; then
    write_status healthy "$failure_count" "$backend" "$frontend" "$(tunnel_state)"
    return 0
  fi

  # The publisher can claim the lock while the two probes above are running.
  # Do not enter recovery after that point; the publisher owns the service
  # transition and will perform the paired health check itself.
  local maintenance_info=""
  if maintenance_info="$(production_maintenance_inspect 2>/dev/null)"; then
    write_status maintenance 0 maintenance maintenance "$(tunnel_state)" "$maintenance_info"
    log "maintenance window active after health probe $(printf '%s' "$maintenance_info" | tr '\n' ' ')"
    return 0
  fi

  log "health check failed backend=$backend frontend=$frontend; attempting controlled recovery"
  if restart_verified_pair; then
    if probe_backend && probe_frontend; then
      write_status recovered 0 ready ready "$(tunnel_state)"
      log "controlled recovery succeeded"
      return 0
    fi
  fi

  write_status degraded "$((failure_count + 1))" "$backend" "$frontend" "$(tunnel_state)"
  return 1
}

log "supervisor started interval=$CHECK_INTERVAL no_build=$NO_BUILD"
failure_count=0
cycles=0
while :; do
  cycles=$((cycles + 1))
  rotate_service_logs
  maintenance_info=""
  if maintenance_info="$(production_maintenance_inspect 2>/dev/null)"; then
    write_status maintenance 0 maintenance maintenance "$(tunnel_state)" "$maintenance_info"
    log "maintenance window active $(printf '%s' "$maintenance_info" | tr '\n' ' ')"
    failure_count=0
  elif check_once "$failure_count"; then
    failure_count=0
  else
    failure_count=$((failure_count + 1))
    backoff=$((CHECK_INTERVAL * (2 ** (failure_count - 1))))
    [ "$backoff" -le "$MAX_BACKOFF" ] || backoff="$MAX_BACKOFF"
    log "recovery failed failure_count=$failure_count backoff_seconds=$backoff"
    sleep "$backoff"
  fi

  [ "$MAX_CYCLES" -gt 0 ] && [ "$cycles" -ge "$MAX_CYCLES" ] && exit 0
  sleep "$CHECK_INTERVAL"
done
