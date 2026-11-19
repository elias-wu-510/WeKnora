#!/usr/bin/env bash
# claw 用户执行（需要 docker 组；未刷新组时会自动用 sg docker 兜底）：
#   1) 以 Vite base=/c-knowledge/ 构建前端 dist 到 frontend/dist-c-knowledge
#   2) 构建镜像 weknora-ui:c-knowledge
#   3) 启动/更新并行容器 WeKnora-frontend-ck，监听 127.0.0.1:8082（不影响现有 8081）
set -euo pipefail

WK=/home/claw/WeKnora
cd "$WK/frontend"

run() {
  if docker info >/dev/null 2>&1; then "$@"
  else sg docker -c "$(printf '%q ' "$@")"
  fi
}

# 1) 构建带前缀的 dist（已存在则跳过；强制重建用 FORCE=1）
if [ "${FORCE:-0}" = "1" ] || [ ! -f dist-c-knowledge/index.html ]; then
  echo "[build] vite build --base=/c-knowledge/ --outDir dist-c-knowledge"
  VITE_IS_DOCKER=true NODE_OPTIONS="--max-old-space-size=4096" \
    npm run build -- --base=/c-knowledge/ --outDir dist-c-knowledge
fi
grep -q '/c-knowledge/assets/' dist-c-knowledge/index.html \
  || { echo "ERROR: dist-c-knowledge/index.html 未带 /c-knowledge/ 前缀" >&2; exit 1; }

# 2) 构建镜像
echo "[image] docker build -t weknora-ui:c-knowledge"
run docker build -f Dockerfile.c-knowledge -t weknora-ui:c-knowledge .

# 3) 重建容器
run docker rm -f WeKnora-frontend-ck >/dev/null 2>&1 || true
run docker run -d \
  --name WeKnora-frontend-ck \
  --restart unless-stopped \
  --network weknora_WeKnora-network \
  -p 127.0.0.1:8082:80 \
  -e MAX_FILE_SIZE_MB=50 \
  -e MAX_SKILL_BUNDLE_SIZE_MB=256 \
  -e APP_HOST=app \
  -e APP_PORT=8080 \
  -e APP_SCHEME=http \
  weknora-ui:c-knowledge

sleep 3
echo "[verify] http://127.0.0.1:8082/ 资源前缀:"
run docker exec WeKnora-frontend-ck sh -c \
  "grep -o '/c-knowledge/assets/[^\"]*' /usr/share/nginx/html/index.html | head -3"
echo "OK: C-Knowledge 前端已在 127.0.0.1:8082 运行"