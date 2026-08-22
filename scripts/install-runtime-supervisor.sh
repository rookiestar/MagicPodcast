#!/bin/bash
# Install the repeatable macOS launchd supervisor for MagicPodcast services.

set -euo pipefail

if [ "$(uname)" != "Darwin" ]; then
  echo "This installer currently supports macOS launchd only." >&2
  exit 1
fi

PROJECT_DIR="${MAGICPODCAST_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
DEPLOY_LOCK_DIR="${MAGICPODCAST_DEPLOY_LOCK_DIR:-/tmp/magicpodcast-production-deploy.lock}"
LABEL="com.magicpodcast.supervisor"
LEGACY_LABEL="com.magicpodcast"
PLIST_PATH="$HOME/Library/LaunchAgents/$LABEL.plist"
DRY_RUN=false
REPLACE_LEGACY=false

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    --replace-legacy) REPLACE_LEGACY=true ;;
    --help|-h)
      echo "Usage: $0 [--dry-run] [--replace-legacy]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      exit 2
      ;;
  esac
done

if [ "$DRY_RUN" != true ] && launchctl print "gui/$(id -u)/$LEGACY_LABEL" >/dev/null 2>&1 && [ "$REPLACE_LEGACY" != true ]; then
  echo "Legacy $LEGACY_LABEL is loaded; rerun with --replace-legacy after review." >&2
  exit 1
fi

mkdir -p "$HOME/Library/LaunchAgents" "$PROJECT_DIR/logs"

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
    <string>/bin/bash</string>
    <string>$PROJECT_DIR/scripts/service-supervisor.sh</string>
  </array>
  <key>WorkingDirectory</key>
  <string>$PROJECT_DIR</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>MAGICPODCAST_PROJECT_DIR</key>
    <string>$PROJECT_DIR</string>
    <key>MAGICPODCAST_DEPLOY_LOCK_DIR</key>
    <string>$DEPLOY_LOCK_DIR</string>
    <key>MAGICPODCAST_SUPERVISOR_LOG</key>
    <string>$PROJECT_DIR/logs/supervisor.log</string>
    <key>MAGICPODCAST_SUPERVISOR_STATUS_FILE</key>
    <string>$PROJECT_DIR/logs/supervisor.status</string>
    <key>MAGICPODCAST_BACKEND_LOG</key>
    <string>$PROJECT_DIR/logs/backend.log</string>
    <key>MAGICPODCAST_FRONTEND_LOG</key>
    <string>$PROJECT_DIR/logs/frontend.log</string>
    <key>MAGICPODCAST_SUPERVISE_TUNNEL</key>
    <string>false</string>
    <key>MAGICPODCAST_SUPERVISOR_NO_BUILD</key>
    <string>true</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>30</integer>
  <key>ProcessType</key>
  <string>Interactive</string>
  <key>StandardOutPath</key>
  <string>/dev/null</string>
  <key>StandardErrorPath</key>
  <string>/dev/null</string>
</dict>
</plist>
EOF
}

if [ "$DRY_RUN" = true ]; then
  render_plist
  exit 0
fi

render_plist > "$PLIST_PATH"
plutil -lint "$PLIST_PATH" >/dev/null

uid="$(id -u)"
if [ "$REPLACE_LEGACY" = true ]; then
  launchctl bootout "gui/$uid/$LEGACY_LABEL" >/dev/null 2>&1 || true
  launchctl disable "gui/$uid/$LEGACY_LABEL" >/dev/null 2>&1 || true
fi
launchctl bootout "gui/$uid" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$uid" "$PLIST_PATH"
launchctl enable "gui/$uid/$LABEL"
launchctl kickstart -k "gui/$uid/$LABEL"
launchctl print "gui/$uid/$LABEL" >/dev/null

echo "Installed $LABEL"
echo "  plist: $PLIST_PATH"
echo "  log: $PROJECT_DIR/logs/supervisor.log"
echo "  status: $PROJECT_DIR/logs/supervisor.status"
