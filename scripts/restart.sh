#!/usr/bin/env bash
# Restart QuizTrace services
# Usage: ./scripts/restart.sh              restart both backend and frontend
#        ./scripts/restart.sh backend     restart only backend
#        ./scripts/restart.sh frontend    restart only frontend
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${1:-all}"

echo "[restart] restarting: $TARGET"
"$ROOT/scripts/stop.sh" "$TARGET"
"$ROOT/scripts/start.sh" "$TARGET"
echo "[restart] done"
