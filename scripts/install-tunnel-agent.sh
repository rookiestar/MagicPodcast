#!/bin/bash
# Render or install the named Cloudflare Tunnel launchd agent.

set -euo pipefail

if [ "$(uname)" != "Darwin" ]; then
  echo "This installer currently supports macOS launchd only." >&2
  exit 1
fi

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
export PATH

LABEL="com.cloudflare.cloudflared"
PLIST_PATH="$HOME/Library/LaunchAgents/$LABEL.plist"
# Do not fall back to Cloudflare's generic config.yml: this Mac may contain
# another personal tunnel. MagicPodcast must always use its named-tunnel file.
CONFIG_PATH="${MAGICPODCAST_CLOUDFLARED_CONFIG:-$HOME/.cloudflared/magicpodcast-prod.yml}"
CLOUDFLARED_BIN="${MAGICPODCAST_CLOUDFLARED_BIN:-/opt/homebrew/bin/cloudflared}"
DRY_RUN=false
ENABLE=false

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --enable) ENABLE=true ;;
    --help|-h)
      echo "Usage: $0 --dry-run | --enable"
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      exit 2
      ;;
  esac
done

if [ "$DRY_RUN" != true ] && [ "$ENABLE" != true ]; then
  echo "Refusing to mutate launchd without --enable; use --dry-run to inspect." >&2
  exit 2
fi
[ -x "$CLOUDFLARED_BIN" ] || { echo "cloudflared not found: $CLOUDFLARED_BIN" >&2; exit 1; }
[ -f "$CONFIG_PATH" ] || { echo "Tunnel config not found: $CONFIG_PATH" >&2; exit 1; }

uid="$(id -u)"
render_plist() {
  cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$CLOUDFLARED_BIN</string>
    <string>tunnel</string>
    <string>--config</string>
    <string>$CONFIG_PATH</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>30</integer>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>StandardOutPath</key>
  <string>$HOME/Library/Logs/cloudflared.log</string>
  <key>StandardErrorPath</key>
  <string>$HOME/Library/Logs/cloudflared.log</string>
</dict>
</plist>
EOF
}

if [ "$DRY_RUN" = true ]; then
  render_plist
  exit 0
fi

mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs"
render_plist > "$PLIST_PATH"
plutil -lint "$PLIST_PATH" >/dev/null
launchctl bootout "gui/$uid" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$uid" "$PLIST_PATH"
launchctl enable "gui/$uid/$LABEL"
launchctl kickstart -k "gui/$uid/$LABEL"
launchctl print "gui/$uid/$LABEL" >/dev/null

echo "Installed $LABEL using the existing named Tunnel config"
