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
OFFSITE_DIR="${MAGICPODCAST_OFFSITE_DIR:-}"
AGE_RECIPIENT_FILE="${MAGICPODCAST_AGE_RECIPIENT_FILE:-}"
OFFSITE_KEEP="${MAGICPODCAST_OFFSITE_KEEP:-14}"
OFFSITE_MAX_AGE_HOURS="${MAGICPODCAST_OFFSITE_MAX_AGE_HOURS:-26}"

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

if [ -n "$OFFSITE_DIR" ] || [ -n "$AGE_RECIPIENT_FILE" ]; then
  if [ -z "$OFFSITE_DIR" ] || [ -z "$AGE_RECIPIENT_FILE" ]; then
    echo "MAGICPODCAST_OFFSITE_DIR and MAGICPODCAST_AGE_RECIPIENT_FILE must be set together." >&2
    exit 1
  fi
  if ! [[ "$OFFSITE_KEEP" =~ ^[0-9]+$ ]] || [ "$OFFSITE_KEEP" -lt 1 ]; then
    echo "MAGICPODCAST_OFFSITE_KEEP must be a positive integer." >&2
    exit 1
  fi
  if ! [[ "$OFFSITE_MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || [ "$OFFSITE_MAX_AGE_HOURS" -lt 1 ]; then
    echo "MAGICPODCAST_OFFSITE_MAX_AGE_HOURS must be a positive integer." >&2
    exit 1
  fi
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
    <key>MAGICPODCAST_OFFSITE_DIR</key>
    <string>$OFFSITE_DIR</string>
    <key>MAGICPODCAST_AGE_RECIPIENT_FILE</key>
    <string>$AGE_RECIPIENT_FILE</string>
    <key>MAGICPODCAST_OFFSITE_KEEP</key>
    <string>$OFFSITE_KEEP</string>
    <key>MAGICPODCAST_OFFSITE_MAX_AGE_HOURS</key>
    <string>$OFFSITE_MAX_AGE_HOURS</string>
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
