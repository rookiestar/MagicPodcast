#!/bin/bash
# Read-only health check for the owner relay primary path and Cloudflare standby.

set -u

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH

PRIMARY_HEALTH_URL="${MAGICPODCAST_PRIMARY_HEALTH_URL:-http://127.0.0.1:18089/health}"
FALLBACK_HEALTH_URL="${MAGICPODCAST_FALLBACK_HEALTH_URL:-https://rookiestar.cn/health}"
TIMEOUT_SECONDS="${MAGICPODCAST_ACCESS_PATH_TIMEOUT:-10}"

case "${1:-}" in
  "")
    ;;
  --help|-h)
    cat <<'EOF'
Usage: ./scripts/access-path-status.sh

Checks:
  primary   Owner relay health endpoint (default: loopback relay)
  fallback  Unauthenticated Cloudflare Access gate

Environment:
  MAGICPODCAST_PRIMARY_HEALTH_URL
  MAGICPODCAST_FALLBACK_HEALTH_URL
  MAGICPODCAST_ACCESS_PATH_TIMEOUT
EOF
    exit 0
    ;;
  *)
    echo "Unknown argument: $1" >&2
    exit 2
    ;;
esac

case "$TIMEOUT_SECONDS" in
  ""|*[!0-9]*)
    echo "MAGICPODCAST_ACCESS_PATH_TIMEOUT must be a positive integer." >&2
    exit 2
    ;;
esac
if [ "$TIMEOUT_SECONDS" -le 0 ]; then
  echo "MAGICPODCAST_ACCESS_PATH_TIMEOUT must be a positive integer." >&2
  exit 2
fi

temporary_root="${TMPDIR:-/tmp}"
primary_body="$(mktemp "$temporary_root/magicpodcast-primary-body.XXXXXX")"
primary_error="$(mktemp "$temporary_root/magicpodcast-primary-error.XXXXXX")"
fallback_headers="$(mktemp "$temporary_root/magicpodcast-fallback-headers.XXXXXX")"
fallback_error="$(mktemp "$temporary_root/magicpodcast-fallback-error.XXXXXX")"
cleanup() {
  rm -f -- "$primary_body" "$primary_error" "$fallback_headers" "$fallback_error"
}
trap cleanup EXIT

primary_code="$(
  curl --silent --show-error \
    --connect-timeout "$TIMEOUT_SECONDS" \
    --max-time "$TIMEOUT_SECONDS" \
    --output "$primary_body" \
    --write-out '%{http_code}' \
    "$PRIMARY_HEALTH_URL" 2>"$primary_error" || true
)"
if [ "$primary_code" = "200" ] && grep -q '"status":"ok"' "$primary_body"; then
  primary_status="healthy"
else
  primary_status="unhealthy"
fi

primary_release_id="$(
  sed -n 's/.*"release_id":"\([^"]*\)".*/\1/p' "$primary_body" | head -1
)"
primary_build_mode="$(
  sed -n 's/.*"build_mode":"\([^"]*\)".*/\1/p' "$primary_body" | head -1
)"

fallback_code="$(
  curl --silent --show-error \
    --connect-timeout "$TIMEOUT_SECONDS" \
    --max-time "$TIMEOUT_SECONDS" \
    --max-redirs 0 \
    --dump-header "$fallback_headers" \
    --output /dev/null \
    --write-out '%{http_code}' \
    "$FALLBACK_HEALTH_URL" 2>"$fallback_error" || true
)"
if [ "$fallback_code" = "302" ] &&
  grep -qi '^location: .*\/cdn-cgi\/access\/login' "$fallback_headers"; then
  fallback_status="standby_ready"
  fallback_access_gate="present"
else
  fallback_status="unavailable"
  fallback_access_gate="missing"
fi

echo "policy=relay_primary_cloudflare_standby"
echo "primary_status=$primary_status"
echo "primary_http_code=${primary_code:-000}"
echo "primary_release_id=${primary_release_id:-unknown}"
echo "primary_build_mode=${primary_build_mode:-unknown}"
echo "fallback_status=$fallback_status"
echo "fallback_http_code=${fallback_code:-000}"
echo "fallback_access_gate=$fallback_access_gate"

if [ "$primary_status" != "healthy" ] || [ "$fallback_status" != "standby_ready" ]; then
  exit 1
fi
