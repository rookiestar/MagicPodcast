#!/usr/bin/env bash
# Shared production maintenance window for the publisher and launchd supervisor.
#
# The directory itself is the maintenance marker. mkdir is used as the atomic
# claim; the metadata files make ownership and stale-lock recovery auditable.

MAGICPODCAST_MAINTENANCE_LOCK_DIR="${MAGICPODCAST_DEPLOY_LOCK_DIR:-/tmp/magicpodcast-production-deploy.lock}"
MAGICPODCAST_MAINTENANCE_STALE_AFTER="${MAGICPODCAST_DEPLOY_LOCK_STALE_AFTER:-300}"
MAGICPODCAST_MAINTENANCE_HEARTBEAT_INTERVAL="${MAGICPODCAST_DEPLOY_LOCK_HEARTBEAT_INTERVAL:-5}"
MAGICPODCAST_MAINTENANCE_OWNERSHIP=none
MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID=""

production_maintenance_validate_config() {
  [[ "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" = /* ]] || {
    printf 'production lock path must be absolute\n' >&2
    return 1
  }
  if ! [[ "$MAGICPODCAST_MAINTENANCE_STALE_AFTER" =~ ^[0-9]+$ ]] ||
    [ "$MAGICPODCAST_MAINTENANCE_STALE_AFTER" -lt 1 ]; then
    printf 'invalid production lock stale timeout: %s\n' \
      "$MAGICPODCAST_MAINTENANCE_STALE_AFTER" >&2
    return 1
  fi
  if ! [[ "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_INTERVAL" =~ ^[0-9]+$ ]] ||
    [ "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_INTERVAL" -lt 1 ]; then
    printf 'invalid production lock heartbeat interval: %s\n' \
      "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_INTERVAL" >&2
    return 1
  fi
}

production_maintenance_value() {
  local key="$1"
  local dir="${2:-$MAGICPODCAST_MAINTENANCE_LOCK_DIR}"
  local file
  case "$key" in
    owner_pid) file=owner.pid ;;
    owner_started_at) file=owner.started_at ;;
    started_at) file=started_at ;;
    started_epoch) file=started_epoch ;;
    heartbeat_epoch) file=heartbeat_epoch ;;
    operation) file=operation ;;
    state) file=state ;;
    *) return 2 ;;
  esac
  if [ -f "$dir/$file" ]; then
    tr -d '\r\n' < "$dir/$file"
  elif [ "$key" = owner_pid ] && [ -f "$dir/pid" ]; then
    # Read the legacy wrapper marker while an older deploy process is being
    # drained; this prevents a new supervisor from reclaiming it prematurely.
    tr -d '\r\n' < "$dir/pid"
  fi
}

production_maintenance_process_start() {
  local pid="$1"
  ps -p "$pid" -o lstart= 2>/dev/null |
    sed 's/^[[:space:]]*//;s/[[:space:]]*$//' |
    head -1
}

production_maintenance_path_epoch() {
  local dir="$1"
  stat -f '%m' "$dir" 2>/dev/null || stat -c '%Y' "$dir" 2>/dev/null
}

production_maintenance_owner_alive() {
  local dir="${1:-$MAGICPODCAST_MAINTENANCE_LOCK_DIR}"
  local owner_pid recorded_start current_start process_state
  owner_pid="$(production_maintenance_value owner_pid "$dir")"
  [[ "$owner_pid" =~ ^[1-9][0-9]*$ ]] || return 1
  kill -0 "$owner_pid" 2>/dev/null || return 1
  process_state="$(ps -p "$owner_pid" -o stat= 2>/dev/null | sed 's/^[[:space:]]*//')"
  [[ "$process_state" != Z* ]] || return 1

  recorded_start="$(production_maintenance_value owner_started_at "$dir")"
  [ -n "$recorded_start" ] || return 0
  current_start="$(production_maintenance_process_start "$owner_pid")"
  [ -n "$current_start" ] || return 0
  [ "$recorded_start" = "$current_start" ]
}

production_maintenance_age() {
  local dir="${1:-$MAGICPODCAST_MAINTENANCE_LOCK_DIR}"
  local last_epoch now
  last_epoch="$(production_maintenance_value heartbeat_epoch "$dir")"
  if ! [[ "$last_epoch" =~ ^[0-9]+$ ]]; then
    last_epoch="$(production_maintenance_value started_epoch "$dir")"
  fi
  if ! [[ "$last_epoch" =~ ^[0-9]+$ ]]; then
    last_epoch="$(production_maintenance_path_epoch "$dir")"
  fi
  [[ "$last_epoch" =~ ^[0-9]+$ ]] || return 1
  now="$(date +%s)"
  if [ "$now" -le "$last_epoch" ]; then
    printf '0\n'
  else
    printf '%s\n' "$((now - last_epoch))"
  fi
}

production_maintenance_is_stale() {
  local dir="${1:-$MAGICPODCAST_MAINTENANCE_LOCK_DIR}"
  local age lock_state
  lock_state="$(production_maintenance_value state "$dir")"
  # A dead owner in the database-write phase is ambiguous: the transaction
  # may already be committed. Only an explicit recovery operation may claim it.
  [ "$lock_state" != critical ] && [ "$lock_state" != recovery_required ] || return 1
  production_maintenance_owner_alive "$dir" && return 1
  age="$(production_maintenance_age "$dir")" || return 1
  [ "$age" -ge "$MAGICPODCAST_MAINTENANCE_STALE_AFTER" ]
}

production_maintenance_remove_dir() {
  local dir="$1"
  rm -f \
    "$dir/owner.pid" \
    "$dir/pid" \
    "$dir/owner.started_at" \
    "$dir/started_at" \
    "$dir/started_epoch" \
    "$dir/heartbeat_epoch" \
    "$dir/operation" \
    "$dir/state" || return 1
  rmdir "$dir"
}

production_maintenance_reclaim_stale() {
  local dir="${1:-$MAGICPODCAST_MAINTENANCE_LOCK_DIR}"
  local stale_dir="${dir}.stale.$$.$RANDOM"

  production_maintenance_is_stale "$dir" || return 1
  if ! mv "$dir" "$stale_dir" 2>/dev/null; then
    return 1
  fi

  # Re-check after the atomic rename. A PID may have been reused between the
  # first check and mv; never remove a lock that now belongs to a live process.
  if production_maintenance_owner_alive "$stale_dir"; then
    if [ ! -e "$dir" ]; then
      mv "$stale_dir" "$dir" 2>/dev/null || true
    fi
    return 1
  fi
  production_maintenance_remove_dir "$stale_dir"
}

production_maintenance_inspect() {
  production_maintenance_validate_config || return 1
  if [ ! -e "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ]; then
    return 1
  fi
  if [ ! -d "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ]; then
    printf 'state=active\nowner_pid=unknown\noperation=unknown\n'
    return 0
  fi

  local owner_pid started_at operation lock_state age
  owner_pid="$(production_maintenance_value owner_pid)"
  started_at="$(production_maintenance_value started_at)"
  operation="$(production_maintenance_value operation)"
  lock_state="$(production_maintenance_value state)"
  if [ "$lock_state" = recovery_required ]; then
    printf 'state=recovery_required\n'
    printf 'owner_pid=%s\n' "${owner_pid:-unknown}"
    printf 'started_at=%s\n' "${started_at:-unknown}"
    printf 'operation=%s\n' "${operation:-unknown}"
    return 0
  fi
  if [ "$lock_state" = critical ] && ! production_maintenance_owner_alive; then
    printf 'state=recovery_required\n'
    printf 'owner_pid=%s\n' "${owner_pid:-unknown}"
    printf 'started_at=%s\n' "${started_at:-unknown}"
    printf 'operation=%s\n' "${operation:-unknown}"
    return 0
  fi
  if production_maintenance_owner_alive; then
    printf 'state=active\n'
    printf 'owner_pid=%s\n' "${owner_pid:-unknown}"
    printf 'started_at=%s\n' "${started_at:-unknown}"
    printf 'operation=%s\n' "${operation:-unknown}"
    return 0
  fi

  age="$(production_maintenance_age 2>/dev/null || true)"
  if [ -z "$age" ] || [ "$age" -lt "$MAGICPODCAST_MAINTENANCE_STALE_AFTER" ]; then
    printf 'state=stale_pending\n'
    printf 'owner_pid=%s\n' "${owner_pid:-unknown}"
    printf 'started_at=%s\n' "${started_at:-unknown}"
    printf 'operation=%s\n' "${operation:-unknown}"
    return 0
  fi

  if production_maintenance_reclaim_stale; then
    printf 'state=stale_reclaimed\n' >&2
    return 1
  fi

  printf 'state=stale_reclaim_failed\n'
  printf 'owner_pid=%s\n' "${owner_pid:-unknown}"
  printf 'started_at=%s\n' "${started_at:-unknown}"
  printf 'operation=%s\n' "${operation:-unknown}"
  return 0
}

production_maintenance_heartbeat_loop() {
  local owner_pid="$1"
  local owner_start="$2"
  while [ -d "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ]; do
    local current_start current_pid
    current_pid="$(production_maintenance_value owner_pid)"
    [ "$current_pid" = "$owner_pid" ] || break
    current_start="$(production_maintenance_value owner_started_at)"
    [ -z "$owner_start" ] || [ "$current_start" = "$owner_start" ] || break
    production_maintenance_owner_alive || break
    printf '%s\n' "$(date +%s)" > \
      "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/heartbeat_epoch" || break
    sleep "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_INTERVAL" || break
  done
}

production_maintenance_begin() {
  local operation="${1:-deploy}"
  local owner_pid owner_start started_at started_epoch
  case "$operation" in
    deploy|rollback|migration|recovery) ;;
    *) printf 'invalid production maintenance operation: %s\n' "$operation" >&2; return 1 ;;
  esac
  production_maintenance_validate_config || return 1

  if [ "$operation" = recovery ] && [ -d "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ]; then
    local held_state claim_dir claim_age
    held_state="$(production_maintenance_value state 2>/dev/null || true)"
    if { [ "$held_state" = recovery_required ] || [ "$held_state" = critical ]; } &&
      ! production_maintenance_owner_alive; then
      claim_dir="$MAGICPODCAST_MAINTENANCE_LOCK_DIR/.recovery-claim"
      if [ -d "$claim_dir" ]; then
        claim_age="$(production_maintenance_age "$claim_dir" 2>/dev/null || true)"
        if production_maintenance_owner_alive "$claim_dir" || [ -z "$claim_age" ] ||
          [ "$claim_age" -lt "$MAGICPODCAST_MAINTENANCE_STALE_AFTER" ]; then
          printf 'another recovery claimant is active\n' >&2
          return 1
        fi
        production_maintenance_remove_dir "$claim_dir" 2>/dev/null || return 1
      fi
      if ! (umask 077 && mkdir "$claim_dir"); then
        printf 'failed to claim held recovery window\n' >&2
        return 1
      fi
      owner_pid="$$"
      owner_start="$(production_maintenance_process_start "$owner_pid")"
      printf '%s\n' "$owner_pid" > "$claim_dir/owner.pid"
      printf '%s\n' "$owner_start" > "$claim_dir/owner.started_at"
      if production_maintenance_owner_alive ||
        { [ "$(production_maintenance_value state 2>/dev/null || true)" != recovery_required ] &&
          [ "$(production_maintenance_value state 2>/dev/null || true)" != critical ]; }; then
        production_maintenance_remove_dir "$claim_dir" 2>/dev/null || true
        printf 'held recovery window changed while claiming\n' >&2
        return 1
      fi
      started_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
      started_epoch="$(date +%s)"
      printf '%s\n' "$owner_pid" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/owner.pid"
      printf '%s\n' "$owner_start" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/owner.started_at"
      printf '%s\n' "$started_at" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/started_at"
      printf '%s\n' "$started_epoch" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/started_epoch"
      printf '%s\n' "$started_epoch" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/heartbeat_epoch"
      printf 'recovery\n' > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/operation"
      # A recovery takeover inherits the ambiguous database state. Keep it
      # non-reclaimable until paired release readiness succeeds.
      printf 'critical\n' > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/state"
      production_maintenance_remove_dir "$claim_dir" 2>/dev/null || return 1
      MAGICPODCAST_MAINTENANCE_OWNER_PID="$owner_pid"
      MAGICPODCAST_MAINTENANCE_OWNER_START="$owner_start"
      export MAGICPODCAST_MAINTENANCE_OWNER_PID MAGICPODCAST_MAINTENANCE_OWNER_START
      MAGICPODCAST_MAINTENANCE_OWNERSHIP=owned
      production_maintenance_heartbeat_loop "$owner_pid" "$owner_start" &
      MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID="$!"
      return 0
    fi
  fi
  if [ -e "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ]; then
    production_maintenance_inspect >/dev/null 2>&1 || true
    [ ! -e "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ] || {
      printf 'another production maintenance window is active\n' >&2
      return 1
    }
  fi
  if ! (umask 077 && mkdir "$MAGICPODCAST_MAINTENANCE_LOCK_DIR"); then
    printf 'failed to claim production maintenance lock\n' >&2
    return 1
  fi

  owner_pid="$$"
  owner_start="$(production_maintenance_process_start "$owner_pid")"
  started_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  started_epoch="$(date +%s)"
  if ! {
    printf '%s\n' "$owner_pid" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/owner.pid"
    printf '%s\n' "$owner_start" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/owner.started_at"
    printf '%s\n' "$started_at" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/started_at"
    printf '%s\n' "$started_epoch" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/started_epoch"
    printf '%s\n' "$started_epoch" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/heartbeat_epoch"
    printf '%s\n' "$operation" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/operation"
    printf 'maintenance\n' > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/state"
  }; then
    production_maintenance_remove_dir "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" || true
    return 1
  fi

  MAGICPODCAST_MAINTENANCE_OWNER_PID="$owner_pid"
  MAGICPODCAST_MAINTENANCE_OWNER_START="$owner_start"
  export MAGICPODCAST_MAINTENANCE_OWNER_PID MAGICPODCAST_MAINTENANCE_OWNER_START
  MAGICPODCAST_MAINTENANCE_OWNERSHIP=owned
  production_maintenance_heartbeat_loop "$owner_pid" "$owner_start" &
  MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID="$!"
}

production_maintenance_adopt() {
  local expected_operation="${1:-deploy}"
  [ -n "${MAGICPODCAST_MAINTENANCE_OWNER_PID:-}" ] || {
    printf 'production maintenance owner context is missing\n' >&2
    return 1
  }
  [ -d "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" ] || {
    printf 'production maintenance lock is missing\n' >&2
    return 1
  }
  production_maintenance_context_matches || {
    printf 'production maintenance lock owner does not match inherited context\n' >&2
    return 1
  }
  [ "$(production_maintenance_value operation)" = "$expected_operation" ] || {
    printf 'production maintenance operation does not match inherited context\n' >&2
    return 1
  }
  MAGICPODCAST_MAINTENANCE_OWNERSHIP=adopted
}

production_maintenance_context_matches() {
  local expected_pid="${MAGICPODCAST_MAINTENANCE_OWNER_PID:-}"
  local expected_start="${MAGICPODCAST_MAINTENANCE_OWNER_START:-}"
  local actual_pid actual_start
  [ -n "$expected_pid" ] || return 1
  actual_pid="$(production_maintenance_value owner_pid)"
  actual_start="$(production_maintenance_value owner_started_at)"
  [ "$actual_pid" = "$expected_pid" ] || return 1
  [ -z "$expected_start" ] || [ "$actual_start" = "$expected_start" ] || return 1
  production_maintenance_owner_alive
}

production_maintenance_enter() {
  local operation="${1:-deploy}"
  if [ -n "${MAGICPODCAST_MAINTENANCE_OWNER_PID:-}" ]; then
    production_maintenance_adopt "$operation"
  else
    production_maintenance_begin "$operation"
  fi
}

production_maintenance_finish() {
  [ "$MAGICPODCAST_MAINTENANCE_OWNERSHIP" = owned ] || return 0
  local actual_pid actual_start expected_pid expected_start
  expected_pid="${MAGICPODCAST_MAINTENANCE_OWNER_PID:-}"
  expected_start="${MAGICPODCAST_MAINTENANCE_OWNER_START:-}"
  actual_pid="$(production_maintenance_value owner_pid)"
  actual_start="$(production_maintenance_value owner_started_at)"
  if [ "$actual_pid" != "$expected_pid" ] ||
    { [ -n "$expected_start" ] && [ "$actual_start" != "$expected_start" ]; }; then
    printf 'refusing to release a production lock owned by another process\n' >&2
    return 1
  fi

  if [[ "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID" =~ ^[1-9][0-9]*$ ]]; then
    kill "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID" 2>/dev/null || true
    wait "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID" 2>/dev/null || true
  fi
  production_maintenance_remove_dir "$MAGICPODCAST_MAINTENANCE_LOCK_DIR" || {
    printf 'failed to release production maintenance lock\n' >&2
    return 1
  }
  MAGICPODCAST_MAINTENANCE_OWNERSHIP=none
}

production_maintenance_mark_critical() {
  [ "$MAGICPODCAST_MAINTENANCE_OWNERSHIP" = owned ] || {
    printf 'only the maintenance owner may mark a critical window\n' >&2
    return 1
  }
  production_maintenance_context_matches || {
    printf 'refusing to mark a production lock owned by another process\n' >&2
    return 1
  }
  case "$(production_maintenance_value operation)" in
    migration|recovery) ;;
    *) printf 'only migration or recovery may enter database critical state\n' >&2; return 1 ;;
  esac
  printf 'critical\n' > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/state"
  printf '%s\n' "$(date +%s)" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/heartbeat_epoch"
}

production_maintenance_hold_for_recovery() {
  [ "$MAGICPODCAST_MAINTENANCE_OWNERSHIP" = owned ] || {
    printf 'only the maintenance owner may hold recovery state\n' >&2
    return 1
  }
  production_maintenance_context_matches || {
    printf 'refusing to hold a production lock owned by another process\n' >&2
    return 1
  }
  if [[ "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID" =~ ^[1-9][0-9]*$ ]]; then
    kill "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID" 2>/dev/null || true
    wait "$MAGICPODCAST_MAINTENANCE_HEARTBEAT_PID" 2>/dev/null || true
  fi
  printf 'recovery_required\n' > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/state"
  printf '%s\n' "$(date +%s)" > "$MAGICPODCAST_MAINTENANCE_LOCK_DIR/heartbeat_epoch"
  MAGICPODCAST_MAINTENANCE_OWNERSHIP=held
}
