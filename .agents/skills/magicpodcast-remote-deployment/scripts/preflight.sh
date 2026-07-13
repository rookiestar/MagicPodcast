#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  preflight.sh --expected-host <hostname> --repo-root <absolute-path>

Runs read-only MagicPodcast deployment checks on the intended Mac mini.
EOF
}

expected_host=""
repo_root=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --expected-host)
      [ "$#" -ge 2 ] || { usage >&2; exit 2; }
      expected_host="$2"
      shift 2
      ;;
    --repo-root)
      [ "$#" -ge 2 ] || { usage >&2; exit 2; }
      repo_root="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ -n "$expected_host" ] || { printf 'Missing --expected-host\n' >&2; exit 2; }
[ -n "$repo_root" ] || { printf 'Missing --repo-root\n' >&2; exit 2; }
case "$repo_root" in
  /*) ;;
  *) printf 'Repository root must be an absolute path: %s\n' "$repo_root" >&2; exit 2 ;;
esac

short_host="$(hostname -s 2>/dev/null || hostname)"
full_host="$(hostname 2>/dev/null || true)"
computer_name="$(scutil --get ComputerName 2>/dev/null || true)"

if [ "$expected_host" != "$short_host" ] && \
   [ "$expected_host" != "$full_host" ] && \
   [ "$expected_host" != "$computer_name" ]; then
  printf 'Target mismatch: expected=%s short=%s full=%s computer_name=%s\n' \
    "$expected_host" "$short_host" "$full_host" "${computer_name:-unavailable}" >&2
  exit 3
fi

[ -d "$repo_root" ] || { printf 'Repository path does not exist: %s\n' "$repo_root" >&2; exit 4; }
actual_root="$(git -C "$repo_root" rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$actual_root" ] || { printf 'Not a Git checkout: %s\n' "$repo_root" >&2; exit 4; }

normalized_expected="$(cd "$repo_root" && pwd -P)"
normalized_actual="$(cd "$actual_root" && pwd -P)"
[ "$normalized_actual" = "$normalized_expected" ] || {
  printf 'Repository root mismatch: expected=%s actual=%s\n' "$normalized_expected" "$normalized_actual" >&2
  exit 4
}

origin_url="$(git -C "$repo_root" remote get-url origin 2>/dev/null || true)"
case "$origin_url" in
  *github.com/rookiestar/MagicPodcast|*github.com/rookiestar/MagicPodcast.git|*github.com:rookiestar/MagicPodcast|*github.com:rookiestar/MagicPodcast.git) ;;
  *) printf 'Unexpected or missing origin: %s\n' "${origin_url:-unavailable}" >&2; exit 4 ;;
esac

printf 'target_host=%s\n' "$short_host"
printf 'computer_name=%s\n' "${computer_name:-unavailable}"
printf 'architecture=%s\n' "$(uname -m)"
printf 'repo_root=%s\n' "$normalized_actual"
printf 'branch=%s\n' "$(git -C "$repo_root" branch --show-current 2>/dev/null || true)"
printf 'worktree_changes=%s\n' "$(git -C "$repo_root" status --short | wc -l | tr -d ' ')"

for tool in go node npm nginx cloudflared; do
  if command -v "$tool" >/dev/null 2>&1; then
    printf 'tool_%s=present path=%s\n' "$tool" "$(command -v "$tool")"
  else
    printf 'tool_%s=absent\n' "$tool"
  fi
done

unsafe_listener=false
for port in 3000 8080 8088; do
  listener_output="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -z "$listener_output" ]; then
    printf 'port_%s=not_listening\n' "$port"
    continue
  fi

  printf 'port_%s=listening\n' "$port"
  while IFS= read -r pid; do
    [ -n "$pid" ] || continue
    process_name="$(ps -p "$pid" -o comm= 2>/dev/null | sed 's/^[[:space:]]*//' || true)"
    process_user="$(ps -p "$pid" -o user= 2>/dev/null | sed 's/^[[:space:]]*//' || true)"
    process_cwd="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | awk '/^n/ { sub(/^n/, ""); print; exit }')"
    printf 'port_%s_process pid=%s user=%s name=%s cwd=%s\n' \
      "$port" "$pid" "${process_user:-unknown}" "${process_name:-unknown}" "${process_cwd:-unknown}"
  done < <(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | sort -u)

  if printf '%s\n' "$listener_output" | grep -Eq "TCP (\\*|0\\.0\\.0\\.0|\\[::\\]):$port \\(LISTEN\\)"; then
    printf 'port_%s_scope=unsafe_non_loopback\n' "$port" >&2
    unsafe_listener=true
  elif printf '%s\n' "$listener_output" | grep -Eq "TCP (127\\.0\\.0\\.1|\\[::1\\]):$port \\(LISTEN\\)"; then
    printf 'port_%s_scope=loopback\n' "$port"
  else
    printf 'port_%s_scope=unrecognized\n' "$port" >&2
    unsafe_listener=true
  fi
done

cloudflared_dir="$HOME/.cloudflared"
if [ -d "$cloudflared_dir" ]; then
  printf 'cloudflared_dir=present\n'
  for file in "$cloudflared_dir/config.yml" "$cloudflared_dir/config.yaml" "$cloudflared_dir/cert.pem"; do
    if [ -f "$file" ]; then
      stat -f 'cloudflared_file=%N mode=%Lp owner=%Su' "$file"
    fi
  done
else
  printf 'cloudflared_dir=absent\n'
fi

if launchctl print "gui/$(id -u)/com.cloudflare.cloudflared" >/dev/null 2>&1; then
  printf 'cloudflared_launch_agent=loaded\n'
else
  printf 'cloudflared_launch_agent=not_loaded\n'
fi

if [ "$unsafe_listener" = true ]; then
  printf 'preflight=failed reason=unsafe_listener\n' >&2
  exit 5
fi

printf 'preflight=passed\n'
