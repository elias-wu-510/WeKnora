#!/bin/bash
# WeKnora 标准版（Docker Compose）停止（保留数据卷）
set -euo pipefail
WK_DIR="/home/claw/WeKnora"
cd "$WK_DIR"
if docker info >/dev/null 2>&1; then
  docker compose stop
else
  sg docker -c "cd ${WK_DIR} && docker compose stop"
fi