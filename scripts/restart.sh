#!/bin/bash
# Restart MagicPodcast through the canonical release flow in production.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

for arg in "$@"; do
  case "$arg" in
    --prod|--production)
      exec "$PROJECT_DIR/scripts/release.sh" "$@"
      ;;
  esac
done

echo "Restarting MagicPodcast..."
"$PROJECT_DIR/scripts/stop.sh"
echo ""
"$PROJECT_DIR/scripts/start.sh" "$@"
