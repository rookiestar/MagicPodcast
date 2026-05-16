#!/bin/bash
# Install the macOS launchd job for daily MagicPodcast backups.

set -euo pipefail

if [ "$(uname)" != "Darwin" ]; then
  echo "This installer currently supports macOS launchd only." >&2
  exit 1
fi

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LABEL="com.magicpodcast.backup"
PLIST_PATH="$HOME/Library/LaunchAgents/$LABEL.plist"
BACKUP_HOUR="${BACKUP_HOUR:-3}"
BACKUP_MINUTE="${BACKUP_MINUTE:-30}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

if ! [[ "$BACKUP_HOUR" =~ ^[0-9]+$ ]] || [ "$BACKUP_HOUR" -gt 23 ]; then
  echo "BACKUP_HOUR must be an integer between 0 and 23." >&2
  exit 1
fi

if ! [[ "$BACKUP_MINUTE" =~ ^[0-9]+$ ]] || [ "$BACKUP_MINUTE" -gt 59 ]; then
  echo "BACKUP_MINUTE must be an integer between 0 and 59." >&2
  exit 1
fi

if ! [[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] || [ "$RETENTION_DAYS" -lt 1 ]; then
  echo "RETENTION_DAYS must be a positive integer." >&2
  exit 1
fi

mkdir -p "$HOME/Library/LaunchAgents" "$PROJECT_DIR/logs"

cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>$PROJECT_DIR/scripts/daily-backup.sh</string>
  </array>
  <key>WorkingDirectory</key>
  <string>$PROJECT_DIR</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>COMPRESS</key>
    <string>true</string>
    <key>RETENTION_DAYS</key>
    <string>$RETENTION_DAYS</string>
  </dict>
  <key>StartCalendarInterval</key>
  <dict>
    <key>Hour</key>
    <integer>$BACKUP_HOUR</integer>
    <key>Minute</key>
    <integer>$BACKUP_MINUTE</integer>
  </dict>
  <key>StandardOutPath</key>
  <string>$PROJECT_DIR/logs/backup-agent.out.log</string>
  <key>StandardErrorPath</key>
  <string>$PROJECT_DIR/logs/backup-agent.err.log</string>
</dict>
</plist>
EOF

plutil -lint "$PLIST_PATH" >/dev/null

uid="$(id -u)"
launchctl bootout "gui/$uid" "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$uid" "$PLIST_PATH"
launchctl enable "gui/$uid/$LABEL" >/dev/null 2>&1 || true
launchctl print "gui/$uid/$LABEL" >/dev/null

echo "Installed $LABEL"
echo "  schedule: daily at $(printf '%02d:%02d' "$BACKUP_HOUR" "$BACKUP_MINUTE")"
echo "  retention: $RETENTION_DAYS days"
echo "  plist: $PLIST_PATH"
