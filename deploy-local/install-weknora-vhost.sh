#!/usr/bin/env bash
# 安装 weknora.c-smart.hk 的 nginx vhost（需 root）。
#
# 做什么：把仓库里的模板装成 /etc/nginx/conf.d/weknora.c-smart.hk.conf，
#         校验配置语法，重载 nginx；任一步失败自动回滚。
# 不做什么：改 DNS / 建 WAF 条目——那两步在控制台做，见
#         /home/claw/reports/weknora-dedicated-subdomain-20261005.md
#
# 用法（需 root）：
#   sudo /home/claw/WeKnora/deploy-local/install-weknora-vhost.sh
#   sudo /home/claw/WeKnora/deploy-local/install-weknora-vhost.sh --dry-run
set -euo pipefail

SRC=/home/claw/WeKnora/deploy-local/nginx-weknora.conf
DST=/etc/nginx/conf.d/weknora.c-smart.hk.conf
CERT=/etc/nginx/ssl/c-smart.hk-2027-02-17.pem
KEY=/etc/nginx/ssl/c-smart.hk-2027-02-17.key
BACKUP="${DST}.bak.$(date +%Y%m%d%H%M%S)"
DRY_RUN=0
[[ "${1:-}" == "--dry-run" ]] && DRY_RUN=1

log() { printf '[install-weknora-vhost] %s\n' "$*"; }
die() { printf '[install-weknora-vhost] ERROR: %s\n' "$*" >&2; exit 1; }

[[ $DRY_RUN -eq 1 ]] || [[ $(id -u) -eq 0 ]] || die "需要 root（当前 uid=$(id -u)）。用 sudo 重跑。"
[[ -r "$SRC" ]] || die "模板不存在或不可读：$SRC"
if [[ ! -r "$CERT" || ! -r "$KEY" ]]; then
  if [[ $DRY_RUN -eq 1 ]]; then
    log "注意：证书以当前用户不可读（key 为 0600 root），root 执行时正常。"
  else
    die "证书不可读：$CERT / $KEY"
  fi
fi
command -v nginx >/dev/null || die "找不到 nginx"

log "模板：$SRC"
log "目标：$DST"
log "证书：$CERT （有效期至 2027-02-17）"

if [[ $DRY_RUN -eq 1 ]]; then
  log "--dry-run：只做存在性检查，不落盘、不 reload。"
  exit 0
fi

if [[ -e "$DST" ]]; then
  cp -a "$DST" "$BACKUP"
  log "已备份旧文件 → $BACKUP"
fi

install -m 0644 -o root -g root "$SRC" "$DST"
log "已安装新 vhost"

if ! nginx -t; then
  log "nginx -t 失败，回滚"
  if [[ -e "$BACKUP" ]]; then mv -f "$BACKUP" "$DST"; else rm -f "$DST"; fi
  nginx -t || true
  die "配置校验失败，已回滚；未重载 nginx"
fi

systemctl reload nginx
log "nginx 已重载"

code80=$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: weknora.c-smart.hk' http://127.0.0.1/ || echo '000')
code443=$(curl -sk -o /dev/null -w '%{http_code}' -H 'Host: weknora.c-smart.hk' https://127.0.0.1/ || echo '000')
log "本机自测：80 → $code80（期望 301）；443 → $code443（期望 200）"
log "完成。公网仍需 WAF 条目 + DNS CNAME 后才可达（见 runbook）。"