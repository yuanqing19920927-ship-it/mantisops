# MantisOps — Claude 项目指令

## GitHub 推送安全

**严格遵守 [GitHub 推送规则](docs/github-push-rules.md)**，核心要点：

- **禁止推送生产环境数据**：截图、数据库、日志、含真实密码的配置
- **禁止推送内网 IP**：代码和文档中不得出现 `192.168.x.x` 等内网地址
- **截图仅用脱敏/演示数据**：不使用生产环境截图
- 推送前检查 `git diff --cached` 确认无敏感信息

## 部署流程

- 生产服务器：192.168.10.71（通过 sshpass 连接）
- 编译：`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build`
- 前端：`npm run build`
- 部署路径：`/opt/opsboard/`
- 服务管理：`systemctl restart opsboard-server / opsboard-agent`
- 部署后验证：检查服务状态 + API 端点 + Agent 注册

## 代码规范

- Go 编译检查：修改后端代码后运行 `go build ./cmd/server/`
- TypeScript 检查：修改前端代码后运行 `npx tsc --noEmit`
- 配置文件 `server.yaml` 仓库中仅保留空值模板，真实配置只在服务器上
