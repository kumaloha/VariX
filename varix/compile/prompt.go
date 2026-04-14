package compile

import (
	"encoding/json"
	"fmt"
	"strings"
)

type GraphRequirements struct {
	MinNodes int
	MinEdges int
}

func InferGraphRequirements(bundle Bundle) GraphRequirements {
	length := bundle.ApproxTextLength()
	switch {
	case length >= 8000:
		return GraphRequirements{MinNodes: 6, MinEdges: 5}
	case length >= 2500:
		return GraphRequirements{MinNodes: 4, MinEdges: 3}
	default:
		return GraphRequirements{MinNodes: 2, MinEdges: 1}
	}
}

func BuildInstruction(req GraphRequirements) string {
	return strings.TrimSpace(fmt.Sprintf(`
你是一个财经分析编译器。你的任务是把输入内容整理成：
1. 一句话总结
2. 推理逻辑图
3. 隐藏详情

要求：
- 只返回 JSON，不要 markdown，不要解释
- summary、graph、details 都必须出现
- graph 至少包含 %d 个节点、%d 条边
- 时间字段必须按 node kind 区分：
  - 事实 / 隐含条件：优先提供 "occurred_at"
  - 预测：提供 "prediction_start_at"，如果文本明确给出截止时间再提供 "prediction_due_at"
  - 对“未来三个月 / 未来一年 / 今后两年 / 12个月内 / 下季度 / 本季度 / 明年”这类明确相对期限，换算成具体的 "prediction_due_at"
  - 对“未来几年 / 今后一段时间”这类模糊期限，不要硬编截止时间
  - 显式条件 / 结论：没有可靠时间就不要硬编
- 仅当你非常确定一个 node 存在明确的有效窗口时，才额外提供 "valid_from" 和 "valid_to"
- 节点 kind 只允许：事实、显式条件、隐含条件、结论、预测
- 边 kind 只允许：正向、负向、推出、预设
- 节点分类定义必须严格遵守：
  - 事实：已经发生、或文本中被当作已成立/已观察到的现实情况
  - 显式条件：作者明确说出的 if/若/一旦/假如/如果 条件，本身可能尚未发生
  - 隐含条件：作者没明说，但没有它结论就不成立的默认前提
  - 结论：由事实和条件推出来的当前判断
  - 预测：关于未来是否会发生什么的判断
- 不要把“作者明确提出的条件”误标成事实
- 不要把“结论”误标成事实
- 如果一句话同时包含「若/如果/一旦...」条件和未来结果（如“将/会/可能/大概率/引发/导致”），优先拆成两个节点：
  - 一个显式条件节点（前提）
  - 一个预测节点（未来结果）
- 不要把这种“条件 + 未来结果”的混合整句直接全标成显式条件
- 例如：若中东地缘冲突升级叠加流动性收紧，私募信贷极大概率爆发挤兑，并可能引发华尔街系统性金融危机
  必须拆成：
  - 显式条件：若中东地缘冲突升级叠加流动性收紧
  - 预测：私募信贷极大概率爆发挤兑，并可能引发华尔街系统性金融危机
- 优先遵守这个因果骨架：
  - 事实 + 隐含条件 => 结论
  - 事实 + 显式条件 + 结论 => 预测
- 优先召回，不要过度保守
- summary 必须让人比直接读原文更容易一眼看懂
- details 用于折叠展示，可以保留 quote/reference/attachment 的补充信息
- 如果不确定，也必须给出你认为最合理的最小图，而不是返回空 graph
- 长文必须显式拆出多个事实、显式条件、隐含条件和结论，不要只给三两个概括节点
`, req.MinNodes, req.MinEdges))
}

func BuildPrompt(bundle Bundle) string {
	payload := map[string]any{
		"unit_id":          bundle.UnitID,
		"source":           bundle.Source,
		"external_id":      bundle.ExternalID,
		"root_external_id": bundle.RootExternalID,
		"content":          bundle.Content,
		"quotes":           bundle.Quotes,
		"references":       bundle.References,
		"thread_segments":  bundle.ThreadSegments,
		"attachments":      bundle.Attachments,
		"text_context":     bundle.TextContext(),
	}
	encoded, _ := json.MarshalIndent(payload, "", "  ")
	return fmt.Sprintf("请基于以下内容单元生成 compile 结果 JSON。\n\n返回格式示例：\n{\n  \"summary\": \"一句话总结\",\n  \"graph\": {\n    \"nodes\": [\n      {\"id\":\"n1\",\"kind\":\"事实\",\"text\":\"某个已发生的现实情况\",\"occurred_at\":\"1974-01-01T00:00:00Z\"},\n      {\"id\":\"n2\",\"kind\":\"显式条件\",\"text\":\"如果/若/一旦 某事发生\"},\n      {\"id\":\"n3\",\"kind\":\"隐含条件\",\"text\":\"作者没明说但推理必需的前提\",\"occurred_at\":\"2026-04-14T00:00:00Z\"},\n      {\"id\":\"n4\",\"kind\":\"结论\",\"text\":\"当前判断\"},\n      {\"id\":\"n5\",\"kind\":\"预测\",\"text\":\"未来会发生什么\",\"prediction_start_at\":\"2026-04-14T00:00:00Z\",\"prediction_due_at\":\"2026-07-14T00:00:00Z\"}\n    ],\n    \"edges\": [\n      {\"from\":\"n1\",\"to\":\"n3\",\"kind\":\"正向\"},\n      {\"from\":\"n3\",\"to\":\"n4\",\"kind\":\"推出\"},\n      {\"from\":\"n2\",\"to\":\"n5\",\"kind\":\"预设\"},\n      {\"from\":\"n4\",\"to\":\"n5\",\"kind\":\"推出\"}\n    ]\n  },\n  \"details\": {\n    \"caveats\": [\"...\"],\n    \"quote_highlights\": [\"...\"],\n    \"reference_highlights\": [\"...\"]\n  },\n  \"topics\": [\"...\"],\n  \"confidence\": \"low|medium|high\"\n}\n\n内容单元如下：\n\n%s", string(encoded))
}
