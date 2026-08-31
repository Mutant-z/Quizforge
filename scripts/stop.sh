#!/usr/bin/env bash
# Stop QuizTrace services (backend API + frontend dev server)
# Usage: ./scripts/stop.sh              stop both backend and frontend
#        ./scripts/stop.sh backend     stop only backend
#        ./scripts/stop.sh frontend    stop only frontend
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET="${1:-all}"

stop_port() {
  local port="$1"
  local name="$2"
  local pids
  # Restrict the lookup to listeners; matching all connections can terminate
  # unrelated applications that merely connect to this port.
  pids="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    kill $pids 2>/dev/null || true
    sleep 3
    local remaining
    remaining="$(lsof -nP -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
    if [ -n "$remaining" ]; then
      echo "[warn] $name (:$port) still alive, force killing..."
      kill -9 $remaining 2>/dev/null || true
    fi
    echo "[stop] $name stopped (:$port, pid: $pids)"
  else
    echo "[info] $name (:$port) not running"
  fi
}

case "$TARGET" in
  backend) stop_port 8080 "backend" ;;
  frontend) stop_port 5173 "frontend" ;;
  all)
    stop_port 8080 "backend"
    stop_port 5173 "frontend"
    ;;
  *)
    echo "Usage: $0 [backend|frontend]" >&2
    exit 1
    ;;
esac
