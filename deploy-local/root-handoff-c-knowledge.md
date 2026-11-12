# 交给 root/ubuntu agent 的一句话任务（C-Knowledge 路径前缀上线）

## 复制给 root agent 的 prompt

```
你是 root/ubuntu 运维 agent。请执行以下脚本，为 WeKnora 增加 https://llm-ai.c-smart.hk/c-knowledge/ 入口：

  sudo /home/claw/WeKnora/deploy-local/install-c-knowledge-nginx.sh

脚本行为（已预演通过，无需你另写配置）：
1. 前置检查 127.0.0.1:8082 可达（C-Knowledge 前端容器 WeKnora-frontend-ck 已在运行）；
2. 备份 /etc/nginx/conf.d/llm-ai.c-smart.hk.conf 为 .bak-<时间戳>-c-knowledge；
3. 在 catch-all `location /` 之前插入 /c-knowledge/ 反代到 127.0.0.1:8082（末尾 / 剥前缀）；
4. nginx -t；失败自动回滚备份并退出；
5. systemctl reload nginx（不 restart）；
6. 打印自检：/c-knowledge/ 状态码、HTML 前缀、API 状态码、/senior-ai 回归。

请把完整输出贴回。失败时不要手工改配置，直接用脚本的回滚。
```

## 应用后终验（claw 侧执行）

```bash
bash /home/claw/WeKnora/deploy-local/verify-c-knowledge.sh
```