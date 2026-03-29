# GitHub 推送规则

> 本文档规范 MantisOps 项目推送到 GitHub 时的安全和质量要求。

---

## 一、禁止推送的内容

### 1. 生产环境数据

- **截图**：禁止包含生产环境的截图（含内网 IP、服务器名、用户名、业务数据）
- **数据库文件**：`.db`、`.db-shm`、`.db-wal`
- **日志文件**：`logs/` 目录下的所有文件
- **配置文件**：含真实密码、token、API Key 的 `server.yaml`

### 2. 凭据和密钥

- SSH 密码、私钥
- JWT Secret、PSK Token
- 阿里云 AK/SK
- `encryption_key`
- 任何 `.env`、`.pem`、`.key`、`.crt` 文件

### 3. 内网信息

- 内网 IP 地址（`192.168.x.x`、`172.x.x.x`、`10.x.x.x`）不得出现在代码或文档中
- 服务器主机名、业务系统名称
- 内部域名

---

## 二、截图规范

如需在 README 或文档中放截图：

1. 使用 **演示环境或本地开发环境** 的数据
2. 或使用设计工具制作 **脱敏后的示意图**
3. 确保截图中不包含：真实 IP、真实用户名、真实服务器名、真实业务数据

---

## 三、推送前检查清单

每次 `git push` 前，确认以下事项：

- [ ] `git diff --cached` 中无生产环境 IP、密码、token
- [ ] 无生产环境截图或日志文件
- [ ] 配置文件 (`server.yaml`) 中敏感字段为空值或占位符
- [ ] 前端代码中无硬编码的内网地址（使用 `localhost` 或配置变量替代）
- [ ] `.gitignore` 已覆盖所有敏感文件类型

---

## 四、已有 `.gitignore` 规则

```
# 敏感文件已纳入 .gitignore
*.db / *.db-shm / *.db-wal    # 数据库
.env / .env.*                   # 环境变量
*.pem / *.key / *.crt           # TLS 证书
docs/deploy-*.md                # 含凭据的部署文档
logs/                           # 日志
.claude/                        # Claude 配置
```

---

## 五、事故处理

如果敏感数据已推送到 GitHub：

1. **立即**从最新代码中删除文件并推送
2. 使用 `git filter-repo` 或 `bfg` 从 git 历史中彻底清除
3. 强制推送清理后的历史（`git push --force`）
4. 如涉及密码/token，**立即轮换**受影响的凭据
