#!/usr/bin/env bash
# Start QuizTrace services (backend API + frontend dev server)
# Usage: ./scripts/start.sh              start both backend and frontend
#        ./scripts/start.sh backend     start only backend
#        ./scripts/start.sh frontend    start only frontend
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT/logs"
mkdir -p "$LOG_DIR"

TARGET="${1:-all}"

start_backend() {
  # Only a process listening on the service port counts as running. A plain
  # `lsof -ti:8080` also matches unrelated outbound connections whose remote
  # port happens to be 8080, which previously prevented the API from starting.
  if lsof -nP -tiTCP:8080 -sTCP:LISTEN >/dev/null 2>&1; then
    echo "[ok] backend already running on :8080"
    return
  fi
  echo "[start] building and launching backend..."
  (cd "$ROOT/backend" && go build -o /tmp/quiztrace-server ./cmd/server)
  # nohup + disown detach from terminal (macOS has no setsid)
  (cd "$ROOT/backend" && nohup /tmp/quiztrace-server >> "$LOG_DIR/backend.log" 2>&1 < /dev/null &)
  disown 2>/dev/null || true
  echo "       binary: /tmp/quiztrace-server, log: logs/backend.log"
}

start_frontend() {
  if lsof -nP -tiTCP:5173 -sTCP:LISTEN >/dev/null 2>&1; then
    echo "[ok] frontend already running on :5173"
    return
  fi
  echo "[start] launching frontend..."
  # call vite binary directly to avoid pnpm->vite process hierarchy
  (cd "$ROOT/frontend" && nohup ./node_modules/.bin/vite >> "$LOG_DIR/frontend.log" 2>&1 < /dev/null &)
  disown 2>/dev/null || true
  echo "       log: logs/frontend.log"
}

# 检测本机局域网 IP（用于打印访问链接）
lan_ip() {
  # macOS: 取默认路由对应网卡 IP；其他系统回退 ip route
  local ip
  ip="$(ipconfig getifaddr en0 2>/dev/null || true)"
  if [ -z "$ip" ]; then
    ip="$(ipconfig getifaddr en1 2>/dev/null || true)"
  fi
  if [ -z "$ip" ]; then
    ip="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}' || true)"
  fi
  if [ -z "$ip" ]; then
    ip="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
  fi
  echo "$ip"
}

print_links() {
  local ip
  ip="$(lan_ip)"
  echo ""
  echo "  访问地址:"
  echo "    前端页面 (本机):     http://localhost:5173"
  if [ -n "$ip" ]; then
    echo "    前端页面 (局域网):   http://$ip:5173"
    echo "    后端 API   (局域网): http://$ip:8080"
  else
    echo "    (未检测到局域网 IP，可访问 http://localhost:5173)"
  fi
}

case "$TARGET" in
  backend) start_backend ;;
  frontend) start_frontend ;;
  all)
    start_backend
    start_frontend
    ;;
  *)
    echo "Usage: $0 [backend|frontend]" >&2
    exit 1
    ;;
esac

echo ""
echo "waiting for services..."
sleep 5
if [ "$TARGET" != "frontend" ]; then
  curl -s -o /dev/null -w "backend health: %{http_code}\n" http://localhost:8080/health/ready || echo "backend not ready, check logs/backend.log"
fi
if [ "$TARGET" != "backend" ]; then
  curl -s -o /dev/null -w "frontend http:  %{http_code}\n" http://localhost:5173/ || echo "frontend not ready, check logs/frontend.log"
fi
echo "done. view logs: ./scripts/logs.sh"
print_links
