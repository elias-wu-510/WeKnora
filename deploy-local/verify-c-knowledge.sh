#!/usr/bin/env bash
# C-Knowledge 公网终验。root 应用 nginx 变更后，在任意能访问公网的一侧运行。
#   bash /home/claw/WeKnora/deploy-local/verify-c-knowledge.sh
set -u
BASE="https://llm-ai.c-smart.hk/c-knowledge"
fail=0
say() { printf '%-50s' "$1"; }
res() { if [ "$1" = "$2" ]; then echo "PASS ($1)"; else echo "FAIL (got $1, want $2)"; fail=1; fi; }

say "T1 首页 HTTPS 200";        res "$(curl -s -k -o /tmp/vk-home.html -w '%{http_code}' "$BASE/")" 200
say "T2 HTML 资源带前缀";       grep -q '/c-knowledge/assets/' /tmp/vk-home.html && echo PASS || { echo FAIL; fail=1; }
say "T3 无斜杠 308 跳转";       res "$(curl -s -k -o /dev/null -w '%{http_code}' "https://llm-ai.c-smart.hk/c-knowledge")" 308
say "T4 无 token API 401/403";  c=$(curl -s -k -o /dev/null -w '%{http_code}' "$BASE/api/v1/knowledge-bases"); case "$c" in 401|403) echo "PASS ($c)";; *) echo "FAIL ($c)"; fail=1;; esac
say "T5 深链 200 且带前缀";     curl -s -k -o /tmp/vk-deep.html -w '%{http_code}' "$BASE/platform/knowledge-bases" | grep -q 200 && grep -q '/c-knowledge/assets/' /tmp/vk-deep.html && echo PASS || { echo FAIL; fail=1; }
say "T6 回归 /senior-ai";       c=$(curl -s -k -o /dev/null -w '%{http_code}' "https://llm-ai.c-smart.hk/senior-ai"); case "$c" in 200|301|302|308) echo "PASS ($c)";; *) echo "FAIL ($c)"; fail=1;; esac
say "T7 回归 根站 /";           res "$(curl -s -k -o /dev/null -w '%{http_code}' "https://llm-ai.c-smart.hk/")" 200

if [ -f /home/claw/WeKnora/deploy-local/admin-credentials.txt ]; then
  PW=$(awk -F': ' '/^password:/{print $2}' /home/claw/WeKnora/deploy-local/admin-credentials.txt)
  TOKEN=$(curl -s -k -m 20 -X POST "$BASE/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"email\":\"admin@c-smart.hk\",\"password\":\"$PW\"}" \
    | python3 -c "import sys,json
try:
    d=json.load(sys.stdin)
    print(d.get('token') or d.get('data',{}).get('token') or '')
except Exception: print('')" 2>/dev/null)
  say "T8 经前缀登录拿到 token"; [ -n "$TOKEN" ] && echo PASS || { echo FAIL; fail=1; }
fi

echo
[ "$fail" = 0 ] && echo "ALL PASS → https://llm-ai.c-smart.hk/c-knowledge/ 可用" || echo "存在失败项，见上"
exit $fail