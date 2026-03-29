package ai

import "fmt"

const rolePrefix = "你是 MantisOps 运维分析助手，一个专业的 IT 基础设施智能运维分析系统。"

const baseReportInstructions = `请基于以下运维数据，生成一份专业的运维分析报告。

要求：
- 使用中文（简体）撰写
- 使用 Markdown 格式输出
- 数据不足时如实说明，不要编造数据
- 对异常情况给出可能的原因分析和处置建议
- 健康评分范围 0-100，综合考虑各项指标`

const dataDelimiterStart = "\n\n===== 运维数据开始 =====\n"
const dataDelimiterEnd = "\n===== 运维数据结束 =====\n"

func wrapData(data string) string {
	return dataDelimiterStart + data + dataDelimiterEnd
}

// ---------------------------------------------------------------------------
// Chapter definitions per report type
// ---------------------------------------------------------------------------

const dailyChapters = `请生成【日报】，包含以下章节：

# 一、整体概况
- 健康评分（0-100）
- 一句话总结当前基础设施整体状态

# 二、服务器状态
- 各服务器关键指标表格（CPU、内存、磁盘、负载）
- 标注异常服务器并说明异常原因

# 三、告警回顾
- 告警统计（总数、各级别分布）
- 告警时间线
- 如有反复出现的告警，分析可能的根因

# 四、端口与服务可用性
- 各监控端口/服务的可用性状态
- 不可用服务的持续时间和影响范围

# 五、容器状态
- 容器运行状态汇总
- 异常容器详情（重启、退出等）

# 六、AI 洞察与建议
- 异常模式识别（是否存在关联性异常）
- 性能优化建议
- 潜在风险预警`

const weeklyChapters = `请生成【周报】，包含日报的所有章节（一至六），并额外增加以下内容：

# 七、本周 vs 上周对比
- 关键指标对比（告警数、平均负载、磁盘使用增长等）
- 趋势变化说明

# 八、周度可用性 SLA
- 各服务/端口的周可用率
- 未达标项目分析

# 九、告警趋势
- 按天分布的告警数量变化
- 告警热点时段分析

# 十、容量变化分析
- 磁盘、内存等资源的周度变化趋势
- 资源使用率预警`

const monthlyChapters = `请生成【月报】，包含日报的所有章节（一至六），并额外增加以下内容：

# 七、月度趋势分析
- 各项关键指标的月度变化趋势
- 与上月对比分析

# 八、容量规划建议
- 磁盘增长预测（按当前增速预估满载时间）
- 内存和 CPU 使用趋势
- 扩容建议

# 九、告警模式分析
- 高频告警 Top 10
- 告警关联性分析
- 告警规则优化建议

# 十、云资源到期提醒
- 即将到期的云服务器、域名、SSL 证书等
- 续费或迁移建议

# 十一、优化建议
- 基础设施优化方向
- 成本优化建议
- 安全加固建议`

const quarterlyChapters = `请生成【季报】，包含日报的所有章节（一至六），并额外增加以下内容：

# 七、三个月趋势对比
- 各月关键指标对比表格
- 整体趋势判断（改善/恶化/稳定）

# 八、基础设施演进
- 本季度基础设施变更记录
- 新增/下线服务器和服务
- 架构变化说明

# 九、重大事件复盘
- 本季度重大故障/告警事件
- 事件时间线、影响范围、根因分析
- 改进措施及执行情况

# 十、下季度预测与规划
- 容量需求预测
- 预计需要的基础设施调整
- 风险预判和预防建议`

const yearlyChapters = `请生成【年报】，包含日报的所有章节（一至六），并额外增加以下内容：

# 七、全年回顾
- 12 个月关键指标趋势图表（用文字描述趋势）
- 各月健康评分变化
- 年度重大事件时间线

# 八、可靠性评分
- 年度整体可用率
- 各服务 SLA 达标情况
- MTTR（平均恢复时间）和 MTBF（平均故障间隔）分析

# 九、增长分析
- 基础设施规模变化（服务器数、服务数、容器数）
- 资源使用量增长率
- 业务增长与基础设施增长的匹配度

# 十、来年容量规划
- 基于全年趋势的资源需求预测
- 预算估算建议
- 技术债务清理计划
- 架构优化路线图`

// ---------------------------------------------------------------------------
// Prompt constructors
// ---------------------------------------------------------------------------

func buildPrompt(chapters, data string) string {
	return fmt.Sprintf("%s\n\n%s\n\n%s%s", rolePrefix, baseReportInstructions, chapters, wrapData(data))
}

func DailyReportPrompt(data string) string     { return buildPrompt(dailyChapters, data) }
func WeeklyReportPrompt(data string) string    { return buildPrompt(weeklyChapters, data) }
func MonthlyReportPrompt(data string) string   { return buildPrompt(monthlyChapters, data) }
func QuarterlyReportPrompt(data string) string { return buildPrompt(quarterlyChapters, data) }
func YearlyReportPrompt(data string) string    { return buildPrompt(yearlyChapters, data) }

// DefaultPromptTemplate returns the built-in prompt template (without data) for a report type.
func DefaultPromptTemplate(reportType string) string {
	chapters := dailyChapters
	switch reportType {
	case "weekly":
		chapters = weeklyChapters
	case "monthly":
		chapters = monthlyChapters
	case "quarterly":
		chapters = quarterlyChapters
	case "yearly":
		chapters = yearlyChapters
	}
	return fmt.Sprintf("%s\n\n%s\n\n%s", rolePrefix, baseReportInstructions, chapters)
}

// ReportPromptForType dispatches to the appropriate prompt function based on report type.
func ReportPromptForType(reportType string, data string) string {
	switch reportType {
	case "weekly":
		return WeeklyReportPrompt(data)
	case "monthly":
		return MonthlyReportPrompt(data)
	case "quarterly":
		return QuarterlyReportPrompt(data)
	case "yearly":
		return YearlyReportPrompt(data)
	default:
		return DailyReportPrompt(data)
	}
}

// ChatSystemPrompt returns the system prompt for chat conversations.
func ChatSystemPrompt(context string) string {
	return fmt.Sprintf(`%s

你的职责：
- 回答用户关于服务器、容器、告警、服务可用性等运维相关问题
- 基于提供的基础设施数据进行分析和建议
- 使用中文（简体）回答
- 使用 Markdown 格式输出，便于阅读
- 当数据不足以回答问题时，诚实告知用户，并说明需要哪些额外信息
- 不要编造不存在的数据或指标
- 对于危险操作（如删除、重启），提醒用户注意风险

以下是当前基础设施快照数据，请基于此数据回答用户问题：
%s`, rolePrefix, wrapData(context))
}

// GenerateTitlePrompt returns a prompt for generating a conversation title.
func GenerateTitlePrompt(userMsg, assistantMsg string) string {
	return fmt.Sprintf(`请根据以下对话内容生成一个简短的中文标题，用于标识这段对话的主题。

要求：
- 最多 20 个中文字符
- 简洁明了，概括对话核心内容
- 只输出标题文本，不要加引号或其他格式

用户消息：
%s

助手回复：
%s`, userMsg, assistantMsg)
}
