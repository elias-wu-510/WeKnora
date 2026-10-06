#!/usr/bin/env bash
# 由 root / ubuntu 执行：把 /c-knowledge/ 前缀反代插入 llm-ai.c-smart.hk 的 server 块。
# 幂等；失败自动回滚；只 reload，不 restart。
#
# 用法：sudo /home/claw/WeKnora/deploy-local/install-c-knowledge-nginx.sh
set -euo pipefail

[ "$(id -u)" -eq 0 ] || { echo "ERROR: 需要 root（sudo）执行" >&2; exit 1; }

CONF=/etc/nginx/conf.d/llm-ai.c-smart.hk.conf
SRC=/home/claw/WeKnora/deploy-local/nginx-c-knowledge.locations.conf

[ -f "$CONF" ] || { echo "ERROR: $CONF 不存在" >&2; exit 1; }
[ -f "$SRC" ]  || { echo "ERROR: $SRC 不存在" >&2; exit 1; }

# 前置：C-Knowledge 前端容器必须已在 8082 监听
if ! curl -sf -m 5 -o /dev/null http://127.0.0.1:8082/; then
  echo "ERROR: 127.0.0.1:8082 不可达；请先启动 C-Knowledge 前端容器" >&2
  exit 1
fi

BAK="${CONF}.bak-$(date +%Y%m%d-%H%M%S)-c-knowledge"
cp "$CONF" "$BAK"
echo "backup: $BAK"

python3 - "$CONF" "$SRC" <<'PY'
import sys
conf, src = sys.argv[1], sys.argv[2]
text = open(conf, encoding='utf-8').read()
if '/c-knowledge/' in text:
    print('already configured, skip'); raise SystemExit(0)
block = open(src, encoding='utf-8').read().rstrip('\n')
anchor = "location / {\nproxy_pass http://10.250.0.16:5000;"
assert text.count(anchor) == 1, 'anchor not found or not unique'
open(conf, 'w', encoding='utf-8').write(text.replace(anchor, block + "\n\n" + anchor))
print('config written')
PY

if ! nginx -t; then
  echo "nginx -t failed, rolling back" >&2
  cp "$BAK" "$CONF"
  nginx -t || true
  exit 1
fi

systemctl reload nginx
echo "reloaded nginx"

echo "--- verify ---"
echo -n "https /c-knowledge/        : "; curl -s -o /dev/null -w "%{http_code}\n" https://llm-ai.c-smart.hk/c-knowledge/
echo -n "http  /c-knowledge (301)   : "; curl -s -o /dev/null -w "%{http_code}\n" http://llm-ai.c-smart.hk/c-knowledge
echo -n "assets in HTML             : "; curl -s https://llm-ai.c-smart.hk/c-knowledge/ | grep -o '/c-knowledge/assets/' | head -1 || true
echo -n "api without token (401/403): "; curl -s -o /dev/null -w "%{http_code}\n" https://llm-ai.c-smart.hk/c-knowledge/api/v1/knowledge-bases
echo -n "regression /senior-ai/     : "; curl -s -o /dev/null -w "%{http_code}\n" https://llm-ai.c-smart.hk/senior-ai