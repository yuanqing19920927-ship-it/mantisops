#!/bin/bash
# MantisOps - 定时同步到 GitHub
# 每天凌晨 2 点执行，通过 crontab 调度

set -e

PROJECT_DIR="/Users/piggy/Projects/opsboard"
LOG_FILE="/Users/piggy/Projects/opsboard/logs/sync-github.log"
PROXY="http://192.168.10.63:7890"

mkdir -p "$(dirname "$LOG_FILE")"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" >> "$LOG_FILE"
}

cd "$PROJECT_DIR"

# 检查是否有变更
if git diff --quiet HEAD && [ -z "$(git status --porcelain)" ]; then
  log "无变更，跳过同步"
  exit 0
fi

# 暂存所有变更
git add -A

# 生成提交信息
CHANGED=$(git diff --cached --stat | tail -1)
git commit -m "chore: auto-sync $(date '+%Y-%m-%d')

${CHANGED}" || { log "提交失败"; exit 1; }

# 通过代理推送
export https_proxy="$PROXY"
git push github main >> "$LOG_FILE" 2>&1

log "同步完成: ${CHANGED}"
