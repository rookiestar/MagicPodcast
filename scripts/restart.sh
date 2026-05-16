#!/bin/bash
# Restart MagicPodcast local services through the canonical stop/start scripts.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "Restarting MagicPodcast..."
"$PROJECT_DIR/scripts/stop.sh"
echo ""
"$PROJECT_DIR/scripts/start.sh" "$@"
