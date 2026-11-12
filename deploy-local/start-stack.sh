#!/bin/bash
# WeKnora 标准版（Docker Compose）一键启动
# 1) 确保宿主机 Ollama 监听在 docker 网桥 172.17.0.1:11434（容器内 host.docker.internal 指向该地址）
# 2) 启动 WeKnora compose 全套服务
set -euo pipefail

WK_DIR="/home/claw/WeKnora"
OLLAMA_BIN="/home/claw/opt/ollama/bin/ollama"
OLLAMA_BIND="172.17.0.1:11434"
OLLAMA_LOG="/tmp/ollama-serve-std.log"

cd "$WK_DIR"

# --- Ollama ---
if curl -sf -m 3 "http://${OLLAMA_BIND}/api/tags" >/dev/null 2>&1; then
  echo "[ollama] already listening on ${OLLAMA_BIND}"
else
  echo "[ollama] starting on ${OLLAMA_BIND}"
  pkill -f "${OLLAMA_BIN} serve" 2>/dev/null || true
  sleep 2
  OLLAMA_HOST="${OLLAMA_BIND}" setsid nohup "${OLLAMA_BIN}" serve > "${OLLAMA_LOG}" 2>&1 &
  sleep 4
  curl -sf -m 5 "http://${OLLAMA_BIND}/api/tags" >/dev/null || { echo "[ollama] FAILED to start"; exit 1; }
fi

# --- Docker Compose ---
# 若当前 shell 未刷新 docker 组，用 sg docker 兜底
if docker info >/dev/null 2>&1; then
  docker compose up -d
  docker compose ps
else
  sg docker -c "cd ${WK_DIR} && docker compose up -d && docker compose ps"
fi