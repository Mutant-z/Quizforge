#!/usr/bin/env bash
# 查看 QuizTrace 服务日志
# 用法:
#   ./scripts/logs.sh                 查看后端+前端最近 50 行
#   ./scripts/logs.sh backend         只看后端
#   ./scripts/logs.sh frontend        只看前端
#   ./scripts/logs.sh -f              实时跟踪（Ctrl+C 退出）
#   ./scripts/logs.sh -f backend      实时跟踪后端
#   ./scripts/logs.sh clear           清空日志文件
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$ROOT/logs"
BACKEND_LOG="$LOG_DIR/backend.log"
FRONTEND_LOG="$LOG_DIR/frontend.log"

LINES="${QT_LOG_LINES:-50}"

if [ "${1:-}" = "clear" ]; then
  : > "$BACKEND_LOG" 2>/dev/null || true
  : > "$FRONTEND_LOG" 2>/dev/null || true
  echo "日志已清空"
  exit 0
fi

FOLLOW=""
TARGET=""
for arg in "$@"; do
  case "$arg" in
    -f) FOLLOW="-f" ;;
    backend) TARGET="backend" ;;
    frontend) TARGET="frontend" ;;
    *)
      echo "未知参数: $arg (支持: -f, backend, frontend, clear)" >&2
      exit 1
      ;;
  esac
done

if [ ! -f "$BACKEND_LOG" ] && [ ! -f "$FRONTEND_LOG" ]; then
  echo "暂无日志文件，请先运行 ./scripts/start.sh"
  exit 0
fi

show_one() {
  local file
  local name
  file="$1"
  name="$2"
  if [ ! -f "$file" ] || [ ! -s "$file" ]; then
    echo "== $name: 无日志 =="
    return
  fi
  echo "== $name ($file) =="
  if [ "$FOLLOW" = "-f" ]; then
    tail -f "$file"
  else
    tail -n "$LINES" "$file"
  fi
}

case "$TARGET" in
  backend)
    show_one "$BACKEND_LOG" "backend"
    ;;
  frontend)
    show_one "$FRONTEND_LOG" "frontend"
    ;;
  "")
    if [ "$FOLLOW" = "-f" ]; then
      tail -f "$BACKEND_LOG" "$FRONTEND_LOG"
    else
      show_one "$BACKEND_LOG" "backend"
      echo ""
      show_one "$FRONTEND_LOG" "frontend"
    fi
    ;;
  *)
    echo "用法错误" >&2
    exit 1
    ;;
esac
