# Compile V2 代码总览与中文解释

> 目标：把当前仓库里已经落地的 **compile v2** 代码、入口接线、调试模式，以及每个文件的职责，用一份文档讲清楚。
>
> 说明：这里收录的是**当前真实代码**的快照式整理，而不是理想方案草稿。也就是说，文档同时会写明：
> - 已完成什么
> - 还没完成什么
> - 代码目前在哪些地方是 MVP / fallback / 待增强

---

## 1. 当前 compile v2 涉及的文件

### 核心实现
- `varix/compilev2/client.go`
- `varix/compilev2/pipeline.go`
- `varix/compilev2/prompts.go`

### 测试与调试
- `varix/compilev2/prompts_test.go`
- `varix/compilev2/smoke_test.go`
- `varix/compilev2/inspect_stages_test.go`

### CLI 接线
- `varix/cmd/cli/compile_commands.go`
- `varix/cmd/cli/main_test.go`

---

## 2. Compile V2 当前设计目标

compile v2 的目标，不再是沿用 legacy compile 那种“直接抽 driver / target / path 桶”的方式，而是转向：

1. Stage 1：先抽图（节点 + 边 + off-graph）
2. Stage 2：做 dedup
3. Stage 3：先做全节点关系判定，再做角色分类
4. Stage 4：做覆盖度 validate（当前是最小版）
5. Stage 5：渲染为最终 schema

当前 v2 已经是可跑的代码，不是空壳；但还属于：
- **MVP + Stage4 minimal**
- 还没达到“可完全替代 legacy compile”的程度

---

## 3. CLI 如何进入 v2

当前入口在：
- `varix/cmd/cli/compile_commands.go`

已经接入：

```bash
varix compile run --pipeline v2 --url <url>
```

默认仍是 legacy：

```bash
varix compile run --url <url>
```

也就是说：
- 不会破坏现有 legacy pipeline
- v2 通过显式 `--pipeline v2` 启动

### 当前 CLI 选择逻辑
在 `selectCompileClient(...)` 里做：
- `legacy` → 用旧 compile client
- `v2` → 用 `compilev2.NewClientFromConfig(...)`
- `v2` 不支持 legacy 的 `--no-verify / --no-validate`

这一步是为了把：
- 旧 compile 行为
- 新 compile v2 行为

完全隔离开。

---

## 4. 文件逐个解释

---

# 4.1 `varix/compilev2/client.go`

## 作用
这是 compile v2 的**客户端入口**，负责：
- 从 config 里拿模型/API 配置
- 初始化 Dashscope runtime
- 按 stage 顺序跑 pipeline
- 在 `COMPILE_STAGE_DEBUG=1` 时，把每一步结果落本地

## 当前核心流程
`Client.Compile(...)` 的顺序是：

1. `stage1Extract`
2. `stage2Dedup`
3. `stage3Relations`
4. `stage3Classify`
5. `stage4Validate`
6. `stage5Render`

然后组装成：
- `compile.Record`

## Debug 模式
当设置：

```bash
COMPILE_STAGE_DEBUG=1
```

时，会在本地创建：

```text
.omx/debug/compilev2/<unit_id>/<timestamp>/
```

并写出：
- `meta.json`
- `stage1_extract.json`
- `stage2_dedup.json`
- `stage3_relations.json`
- `stage3_classify.json`
- `stage4_validate.json`
- `stage5_render.json`

如果某个 stage 报错，还会写：
- `stageX.error.txt`

## 为什么重要
这一步解决了之前最大的问题之一：
> 跑真实样本时，不知道哪一步歪了。

现在至少可以直接看每一步中间结果。

---

# 4.2 `varix/compilev2/pipeline.go`

这是 compile v2 的**主实现文件**。

## 里面定义了哪些内部类型

### `graphNode`
内部图节点：
- `ID`
- `Text`
- `SourceQuote`
- `Role`
- `Ontology`

### `graphEdge`
内部图边：
- `From`
- `To`

### `offGraphItem`
图外节点：
- `ID`
- `Text`
- `Role`
- `AttachesTo`
- `SourceQuote`

### `graphState`
整个中间态图：
- `Nodes`
- `Edges`
- `OffGraph`
- `Rounds`

---

## Stage 1：`stage1Extract`

### 作用
调用 LLM，抽出：
- `nodes`
- `edges`
- `off_graph`

### 当前实现特点
- parser 比较宽容：
  - `nodes` 里混 string / object 都能收
  - `edges` 也允许一些别名字段
  - `off_graph` 缺字段时会补默认值
- 缺少 `id` 时会补 `n1 / n2 / ...`
- 缺少 `source_quote` 时会用 `text` 兜底

### 当前问题
- 召回还是偏宽
- 结果常常会抽出很多“语义上接近但层级不同”的节点
- 所以单靠 Stage 1 不可能直接得到干净主线

---

## Stage 2：`stage2Dedup`

### 作用
对 Stage 1 的节点做去重。

### 当前实现
- 目前是**文本归一化去重**
- 不是你方案里那种真正的 LLM pairwise dedup + union-find 完整实现
- 去重后：
  - 被合掉的节点会降级成 `supplementary`
  - 边会重定向

### 当前问题
- 过于保守
- 对“近义但不完全同文”的节点帮助有限
- 所以 G04 那种：
  - 外资流入
  - no sell America
  - no hedge America
  之间的关系，Stage2 目前基本处理不了

---

## Stage 3A：`stage3Relations`

### 作用
这是目前最关键的新层。

在 dedup 和 classify 之间，先判断节点关系：
- `causal`
- `supports`
- `supplements`
- `explains`
- `none`

### 当前逻辑
如果 A -> B 被判为：
- `causal`：保留边
- `supports`：把 A 降成 `evidence`，挂到 B
- `explains`：把 A 降成 `explanation`，挂到 B
- `supplements`：只保留 primary，一个留主结构，另一个降到 `supplementary`
- `none`：删边，但节点暂时保留

### 为什么重要
这一步正是为了解决你前面一直抓的问题：
> 不是先判 target，而是先判“证明 / 补充说明 / 解释 / 传导”。

### 当前问题
- 还是 pairwise 沿边判，不够全局
- 关系判定 prompt 还比较短
- 对复杂 case（比如 G04）还不稳定

---

## Stage 3B：`stage3Classify`

### 作用
在经过关系层后，再做角色分类：
- `driver`
- `transmission`
- `target_candidate`
- `target`
- `orphan`

### 当前逻辑
先用拓扑：
- 入度 0 / 出度 > 0 → driver
- 出度 0 / 入度 > 0 → target_candidate
- 入度 >0 / 出度 >0 → transmission
- 否则 orphan

然后只对 `target_candidate` 再问一次 LLM：
- 是不是 market outcome
- 属于 `price / flow / decision / none`

### 当前问题
- 这一步还是比较机械
- 如果前面的关系层没判好，这里仍然会把很多“像结果”的句子提成 target

---

## Stage 4：`stage4Validate`

### 作用
当前是最小版 validate：
- 按段落切分文章
- 每段都问：当前图有没有漏节点 / 漏边 / 误分类

### 产物
LLM 返回 patch：
- `missing_nodes`
- `missing_edges`
- `misclassified`

再把 patch 合回图里：
- `applyValidatePatch(...)`
- 然后再跑一遍 dedup + classify

### 当前问题
- 只做了最小可运行版
- 很慢
- 在真实样本上容易成为最大耗时段之一
- 还没有专门的 patch 质量约束

---

## Stage 5：`stage5Render`

### 作用
把内部图渲染成最终 `compile.Output`。

### 它做了什么
1. 找 `drivers`
2. 找 `targets`
3. 提取路径（`extractPaths`）
4. 批量翻译中文（`translateAll`）
5. 写 summary（`summarizeChinese`）
6. 组装最终：
   - `drivers`
   - `targets`
   - `transmission_paths`
   - `evidence_nodes`
   - `explanation_nodes`
   - `supplementary_nodes`
   - `summary`

### 当前 fallback
如果图太差：
- 会用“第一个 driver / 最后一个 target”兜底
- 没有 path 时，会自动补最小 path

### 当前问题
- fallback 太粗
- 容易把不该连的主线硬连出来
- 所以 v2 现在“能跑”，但还容易抽偏

---

# 4.3 `varix/compilev2/prompts.go`

## 作用
集中放 v2 现在用的 prompt 常量。

### 当前有：
- `stage1SystemPrompt`
- `stage1UserPrompt`
- `stage3SystemPrompt`
- `stage3UserPrompt`
- `stage3RelationSystemPrompt`
- `stage3RelationUserPrompt`
- `stage4SystemPrompt`
- `stage4UserPrompt`
- `stage5TranslateSystemPrompt`
- `stage5TranslateUserPrompt`
- `stage5SummarySystemPrompt`
- `stage5SummaryUserPrompt`

## 设计意图
把 prompt 从 pipeline 逻辑里拆出来，方便：
- 单独调 prompt
- 单独测 prompt
- 后续替换成文件模板也更容易

---

# 4.4 `varix/compilev2/prompts_test.go`

## 作用
检查 prompt 常量有没有核心指令。

这不是质量测试，更多是：
- 配置面回归
- 防止 prompt 被误删/误改空

---

# 4.5 `varix/compilev2/smoke_test.go`

## 作用
真实样本 smoke test。

默认不会跑，必须显式打开：

```bash
RUN_COMPILEV2_SMOKE=1 go test ./compilev2 -run TestCompileV2SmokeOnStoredSampleWhenEnabled -count=1 -v
```

## 价值
这个测试不是在做精细断言，
而是在检查：
- 能不能真跑通
- 至少能不能产出非空 `drivers / targets`

---

# 4.6 `varix/compilev2/inspect_stages_test.go`

## 作用
这是调试用的真实样本 stage inspection。

默认不会跑，必须显式打开：

```bash
RUN_COMPILEV2_INSPECT=1 go test ./compilev2 -run TestInspectG04Stages -count=1 -v
```

## 价值
它会打印：
- Stage1 输出
- Stage2 输出
- Stage3 输出
- Stage4 输出
- Stage5 输出

这正是用来定位：
> 到底哪一步把 G04 弄歪了。

---

# 4.7 `varix/cmd/cli/compile_commands.go`

## 作用
这是 CLI compile 入口。

## 跟 v2 相关的改动
增加了：
- `--pipeline` 参数

例如：

```bash
varix compile run --pipeline v2 --url <url>
```

并通过 `selectCompileClient(...)` 做路由：
- `legacy` → 旧 client
- `v2` → compilev2 client

## 为什么重要
这一步让我们能：
- 不破坏 legacy
- 但真实用新方案跑样本

---

# 4.8 `varix/cmd/cli/main_test.go`

## 跟 v2 相关的测试
目前覆盖：
- `v2` 入口是否真的走 v2 client
- 非法 `--pipeline` 值是否会被拒绝

这一步是 worker-2 那条 team lane 合进来的重要内容之一。

---

## 5. 当前 v2 到底完成了什么

### 已完成
- v2 独立 package 已存在
- CLI 已能选择 v2
- Stage1/2/3/4/5 都已经有代码
- debug 模式已能把每一步结果落到本地
- smoke test 已能跑真实样本

### 未完成 / 仍然粗糙的部分
- Stage1 抽图质量仍然不够稳
- Stage2 已升级为候选对 + LLM equivalence + union-find，但仍需继续增强候选生成与质量
- Stage3 关系层已改成全节点一次性关系编排，但复杂样本上仍需继续验证质量与耗时
- Stage4 过慢，容易超时
- Stage5 fallback 还比较粗
- 和 legacy 相比，v2 还没到“默认替代”的质量

---

## 6. 当前 debug 模式怎么用

### 运行
```bash
COMPILE_STAGE_DEBUG=1 \
varix compile run --pipeline v2 --force --url <url> --timeout 600s
```

### 输出目录
```text
.omx/debug/compilev2/<unit_id>/<timestamp>/
```

### 目录里的文件
- `meta.json`
- `stage1_extract.json`
- `stage2_dedup.json`
- `stage3_relations.json`
- `stage3_classify.json`
- `stage4_validate.json`
- `stage5_render.json`

如果出错，还会有：
- `stageX.error.txt`

这能让我们非常直接地看：
- 哪一步慢
- 哪一步歪
- 哪一步把结构弄错了

---

## 7. 当前我对 v2 的结论

compile v2 已经从“空想方案”变成了：

- 可运行
- 可调试
- 已接入最小 validate
- Stage2 已不再是字符串去重
- Stage3 已不再是逐边关系判定



> **一个可运行、可调试、可逐步增强的新主线。**

但它目前仍然是：
- **MVP + 最小 validate**
- 不是最终版

真正下一步该做的是：
1. 提高 Stage1 质量
2. 让 Stage3 关系层更准
3. 降低 Stage4 成本
4. 削弱 Stage5 fallback 的粗暴程度

---

## 8. 附：当前 compile v2 代码全文

> 下面贴当前文件的原始代码。为了避免文档和代码漂移，建议后续如果代码继续大改，再刷新这部分。

### `varix/compilev2/client.go`

```go
package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kumaloha/VariX/varix/compile"
	"github.com/kumaloha/VariX/varix/config"
	"github.com/kumaloha/forge/llm"
)

const defaultDashScopeCompatibleBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

type runtimeChat interface {
	Call(ctx context.Context, req llm.ProviderRequest) (llm.Response, error)
}

type Client struct {
	runtime     runtimeChat
	model       string
	projectRoot string
}

func NewClientFromConfig(projectRoot string, httpClient *http.Client) *Client {
	apiKey := firstConfiguredValue(projectRoot, "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	baseURL := firstConfiguredValue(projectRoot, "COMPILE_BASE_URL", "DASHSCOPE_BASE_URL")
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultDashScopeCompatibleBaseURL
	}
	model := firstConfiguredValue(projectRoot, "COMPILE_MODEL")
	if strings.TrimSpace(model) == "" {
		model = compile.Qwen36PlusModel
	}
	if httpClient == nil {
		timeout := 180 * time.Second
		if raw := firstConfiguredValue(projectRoot, "COMPILE_TIMEOUT_SECONDS"); strings.TrimSpace(raw) != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
				timeout = time.Duration(seconds) * time.Second
			}
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	opts := []llm.DashscopeOption{llm.WithAPIKey(apiKey)}
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, llm.WithAPIBase(baseURL))
	}
	if httpClient.Timeout > 0 {
		opts = append(opts, llm.WithTimeout(httpClient.Timeout))
	}
	provider, err := llm.NewDashscope(opts...)
	if err != nil {
		return nil
	}
	runtime := llm.NewRuntime(llm.RuntimeConfig{
		Provider: provider,
		LLMConfig: llm.LLMConfig{
			Default: llm.DefaultConfig{
				Model:       strings.TrimSpace(model),
				Search:      false,
				Temperature: 0,
				Thinking:    false,
			},
		},
		MaxAttempts: 3,
	})
	return &Client{runtime: runtime, model: strings.TrimSpace(model), projectRoot: projectRoot}
}

func (c *Client) Compile(ctx context.Context, bundle compile.Bundle) (compile.Record, error) {
	if c == nil || c.runtime == nil {
		return compile.Record{}, fmt.Errorf("compile v2 client is nil")
	}
	debugRunDir := c.startDebugRun(bundle)
	debugV2Stage(bundle, "pipeline", "start")
	graph, err := stage1Extract(ctx, c.runtime, c.model, bundle)
	if err != nil {
		debugV2Stage(bundle, "stage1_extract", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage1_extract.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	debugV2Stage(bundle, "stage1_extract", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "stage1_extract.json", graph)
	graph = stage2Dedup(graph)
	debugV2Stage(bundle, "stage2_dedup", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "stage2_dedup.json", graph)
	graph, err = stage3Relations(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage3_relations", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage3_relations.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	debugV2Stage(bundle, "stage3_relations", fmt.Sprintf("done nodes=%d edges=%d off_graph=%d", len(graph.Nodes), len(graph.Edges), len(graph.OffGraph)))
	c.writeDebugJSON(debugRunDir, "stage3_relations.json", graph)
	graph, err = stage3Classify(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage3_classify", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage3_classify.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	debugV2Stage(bundle, "stage3_classify", fmt.Sprintf("done drivers=%d targets=%d", countRole(graph, roleDriver), countRole(graph, roleTarget)))
	c.writeDebugJSON(debugRunDir, "stage3_classify.json", graph)
	graph, err = stage4Validate(ctx, c.runtime, c.model, bundle, graph, 1)
	if err != nil {
		debugV2Stage(bundle, "stage4_validate", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage4_validate.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	debugV2Stage(bundle, "stage4_validate", fmt.Sprintf("done rounds=%d nodes=%d edges=%d", graph.Rounds, len(graph.Nodes), len(graph.Edges)))
	c.writeDebugJSON(debugRunDir, "stage4_validate.json", graph)
	out, err := stage5Render(ctx, c.runtime, c.model, bundle, graph)
	if err != nil {
		debugV2Stage(bundle, "stage5_render", "error: "+err.Error())
		c.writeDebugArtifact(debugRunDir, "stage5_render.error.txt", []byte(err.Error()))
		return compile.Record{}, err
	}
	debugV2Stage(bundle, "stage5_render", fmt.Sprintf("done drivers=%d targets=%d paths=%d evidence=%d explanation=%d supplementary=%d", len(out.Drivers), len(out.Targets), len(out.TransmissionPaths), len(out.EvidenceNodes), len(out.ExplanationNodes), len(out.SupplementaryNodes)))
	c.writeDebugJSON(debugRunDir, "stage5_render.json", out)
	return compile.Record{
		UnitID:         bundle.UnitID,
		Source:         bundle.Source,
		ExternalID:     bundle.ExternalID,
		RootExternalID: bundle.RootExternalID,
		Model:          c.model,
		Output:         out,
		CompiledAt:     time.Now().UTC(),
	}, nil
}

func (c *Client) Verify(ctx context.Context, bundle compile.Bundle, output compile.Output) (compile.Verification, error) {
	return compile.Verification{}, fmt.Errorf("compile v2 client does not implement verify")
}

func (c *Client) VerifyDetailed(ctx context.Context, bundle compile.Bundle, output compile.Output) (compile.Verification, error) {
	return compile.Verification{}, fmt.Errorf("compile v2 client does not implement verify")
}

func firstConfiguredValue(projectRoot string, keys ...string) string {
	for _, key := range keys {
		if value, ok := config.Get(projectRoot, key); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseJSONObject(raw string, target any) error {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	if start >= 0 {
		raw = raw[start:]
	}
	end := strings.LastIndex(raw, "}")
	if end >= 0 {
		raw = raw[:end+1]
	}
	return json.Unmarshal([]byte(raw), target)
}

func debugV2Stage(bundle compile.Bundle, stageName, message string) {
	if strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) == "" {
		return
	}
	unitID := strings.TrimSpace(bundle.UnitID)
	if unitID == "" {
		unitID = strings.TrimSpace(bundle.ExternalID)
	}
	fmt.Fprintf(os.Stderr, "[compilev2-stage] %s %s %s\n", time.Now().UTC().Format(time.RFC3339), stageName, unitID)
	fmt.Fprintf(os.Stderr, "[compilev2-stage] %s %s\n", stageName, message)
}

func (c *Client) startDebugRun(bundle compile.Bundle) string {
	if c == nil || strings.TrimSpace(os.Getenv("COMPILE_STAGE_DEBUG")) == "" || strings.TrimSpace(c.projectRoot) == "" {
		return ""
	}
	unitID := sanitizeDebugPath(firstNonEmpty(bundle.UnitID, bundle.ExternalID, "unknown"))
	ts := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(c.projectRoot, ".omx", "debug", "compilev2", unitID, ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	_ = os.WriteFile(filepath.Join(dir, "meta.json"), mustJSON(map[string]any{
		"unit_id":        bundle.UnitID,
		"source":         bundle.Source,
		"external_id":    bundle.ExternalID,
		"root_external_id": bundle.RootExternalID,
		"started_at":     ts,
	}), 0o644)
	return dir
}

func (c *Client) writeDebugJSON(dir, name string, value any) {
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), mustJSON(value), 0o644)
}

func (c *Client) writeDebugArtifact(dir, name string, payload []byte) {
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), payload, 0o644)
}

func mustJSON(value any) []byte {
	payload, _ := json.MarshalIndent(value, "", "  ")
	return payload
}

func sanitizeDebugPath(value string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", " ", "_")
	return replacer.Replace(strings.TrimSpace(value))
}
```

### `varix/compilev2/pipeline.go`

```go
package compilev2

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kumaloha/VariX/varix/compile"
)

type graphRole string

const (
	roleDriver             graphRole = "driver"
	roleTransmission       graphRole = "transmission"
	roleTargetCandidate    graphRole = "target_candidate"
	roleTarget             graphRole = "target"
	roleOrphan             graphRole = "orphan"
)

type graphNode struct {
	ID          string
	Text        string
	SourceQuote string
	Role        graphRole
	Ontology    string
}

type graphEdge struct {
	From string
	To   string
}

type offGraphItem struct {
	ID          string
	Text        string
	Role        string
	AttachesTo  string
	SourceQuote string
}

type graphState struct {
	Nodes    []graphNode
	Edges    []graphEdge
	OffGraph []offGraphItem
	Rounds   int
}

type relationKind string

const (
	relationCausal      relationKind = "causal"
	relationSupports    relationKind = "supports"
	relationSupplements relationKind = "supplements"
	relationExplains    relationKind = "explains"
	relationNone        relationKind = "none"
)

func countRole(state graphState, role graphRole) int {
	count := 0
	for _, n := range state.Nodes {
		if n.Role == role {
			count++
		}
	}
	return count
}

func stage1Extract(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle) (graphState, error) {
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage1SystemPrompt, fmt.Sprintf(stage1UserPrompt, bundle.TextContext()))
	if err != nil {
		return graphState{}, err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return graphState{}, err
	}
	var payload map[string]any
	if err := parseJSONObject(resp.Text, &payload); err != nil {
		return graphState{}, fmt.Errorf("stage1 extract parse: %w", err)
	}
	state := graphState{}
	state.Nodes = decodeStage1Nodes(payload["nodes"])
	state.Edges = decodeStage1Edges(payload["edges"])
	state.OffGraph = decodeStage1OffGraph(payload["off_graph"])
	state = fillMissingStage1IDs(state)
	return state, nil
}

func stage2Dedup(state graphState) graphState {
	state = normalizeStage1State(state)
	seen := map[string]graphNode{}
	redirect := map[string]string{}
	for _, n := range state.Nodes {
		key := normalizeText(n.Text)
		if existing, ok := seen[key]; ok {
			redirect[n.ID] = existing.ID
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID: fmt.Sprintf("sup_%s", n.ID), Text: n.Text, Role: "supplementary", AttachesTo: existing.ID, SourceQuote: n.SourceQuote,
			})
			continue
		}
		seen[key] = n
		redirect[n.ID] = n.ID
	}
	nodes := make([]graphNode, 0, len(seen))
	for _, n := range seen {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	edgeSet := map[string]struct{}{}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		from := redirect[e.From]
		to := redirect[e.To]
		if from == "" || to == "" || from == to {
			continue
		}
		key := from + "->" + to
		if _, ok := edgeSet[key]; ok {
			continue
		}
		edgeSet[key] = struct{}{}
		edges = append(edges, graphEdge{From: from, To: to})
	}
	state.Nodes = nodes
	state.Edges = edges
	return state
}

func normalizeStage1State(state graphState) graphState {
	nodes := make([]graphNode, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		n.ID = strings.TrimSpace(n.ID)
		n.Text = strings.TrimSpace(n.Text)
		n.SourceQuote = strings.TrimSpace(n.SourceQuote)
		if n.Text == "" {
			continue
		}
		nodes = append(nodes, n)
	}
	state.Nodes = nodes

	validIDs := map[string]struct{}{}
	for _, n := range state.Nodes {
		validIDs[n.ID] = struct{}{}
	}
	edges := make([]graphEdge, 0, len(state.Edges))
	for _, e := range state.Edges {
		e.From = strings.TrimSpace(e.From)
		e.To = strings.TrimSpace(e.To)
		if e.From == "" || e.To == "" || e.From == e.To {
			continue
		}
		if _, ok := validIDs[e.From]; !ok {
			continue
		}
		if _, ok := validIDs[e.To]; !ok {
			continue
		}
		edges = append(edges, e)
	}
	state.Edges = edges

	off := make([]offGraphItem, 0, len(state.OffGraph))
	for _, o := range state.OffGraph {
		o.Text = strings.TrimSpace(o.Text)
		if o.Text == "" {
			continue
		}
		o.Role = strings.TrimSpace(o.Role)
		if o.Role == "" {
			o.Role = "supplementary"
		}
		off = append(off, o)
	}
	state.OffGraph = off
	return state
}

func decodeStage1Nodes(raw any) []graphNode {
	items, _ := raw.([]any)
	out := make([]graphNode, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, graphNode{Text: strings.TrimSpace(v)})
		case map[string]any:
			out = append(out, graphNode{
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(firstNonEmpty(asString(v["text"]), asString(v["content"]))),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
}

func decodeStage1Edges(raw any) []graphEdge {
	items, _ := raw.([]any)
	out := make([]graphEdge, 0, len(items))
	for _, item := range items {
		v, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, graphEdge{
			From: strings.TrimSpace(firstNonEmpty(asString(v["from"]), asString(v["source"]))),
			To:   strings.TrimSpace(firstNonEmpty(asString(v["to"]), asString(v["target"]))),
		})
	}
	return out
}

func decodeStage1OffGraph(raw any) []offGraphItem {
	items, _ := raw.([]any)
	out := make([]offGraphItem, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			out = append(out, offGraphItem{Text: strings.TrimSpace(v), Role: "supplementary"})
		case map[string]any:
			out = append(out, offGraphItem{
				ID:          strings.TrimSpace(asString(v["id"])),
				Text:        strings.TrimSpace(asString(v["text"])),
				Role:        strings.TrimSpace(asString(v["role"])),
				AttachesTo:  strings.TrimSpace(asString(v["attaches_to"])),
				SourceQuote: strings.TrimSpace(asString(v["source_quote"])),
			})
		}
	}
	return out
}

func fillMissingStage1IDs(state graphState) graphState {
	for i := range state.Nodes {
		if strings.TrimSpace(state.Nodes[i].ID) == "" {
			state.Nodes[i].ID = fmt.Sprintf("n%d", i+1)
		}
		if strings.TrimSpace(state.Nodes[i].SourceQuote) == "" {
			state.Nodes[i].SourceQuote = state.Nodes[i].Text
		}
	}
	for i := range state.OffGraph {
		if strings.TrimSpace(state.OffGraph[i].ID) == "" {
			state.OffGraph[i].ID = fmt.Sprintf("o%d", i+1)
		}
		if strings.TrimSpace(state.OffGraph[i].Role) == "" {
			state.OffGraph[i].Role = "supplementary"
		}
		if strings.TrimSpace(state.OffGraph[i].SourceQuote) == "" {
			state.OffGraph[i].SourceQuote = state.OffGraph[i].Text
		}
	}
	return state
}

func stage3Classify(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	inDegree := map[string]int{}
	outDegree := map[string]int{}
	for _, n := range state.Nodes {
		inDegree[n.ID] = 0
		outDegree[n.ID] = 0
	}
	for _, e := range state.Edges {
		outDegree[e.From]++
		inDegree[e.To]++
	}
	for i := range state.Nodes {
		n := &state.Nodes[i]
		switch {
		case inDegree[n.ID] == 0 && outDegree[n.ID] > 0:
			n.Role = roleDriver
		case outDegree[n.ID] == 0 && inDegree[n.ID] > 0:
			n.Role = roleTargetCandidate
		case inDegree[n.ID] > 0 && outDegree[n.ID] > 0:
			n.Role = roleTransmission
		default:
			n.Role = roleOrphan
		}
	}
	filtered := make([]graphNode, 0, len(state.Nodes))
	for _, n := range state.Nodes {
		if n.Role != roleTargetCandidate {
			filtered = append(filtered, n)
			continue
		}
		req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage3SystemPrompt, fmt.Sprintf(stage3UserPrompt, n.Text, n.SourceQuote))
		if err != nil {
			return graphState{}, err
		}
		resp, err := rt.Call(ctx, req)
		if err != nil {
			return graphState{}, err
		}
		var result struct {
			IsMarketOutcome bool   `json:"is_market_outcome"`
			Category        string `json:"category"`
		}
		if err := parseJSONObject(resp.Text, &result); err != nil {
			return graphState{}, fmt.Errorf("stage3 classify parse: %w", err)
		}
		if result.IsMarketOutcome {
			n.Role = roleTarget
			n.Ontology = result.Category
			filtered = append(filtered, n)
			continue
		}
		attachTo := predecessorOf(state.Edges, n.ID)
		if attachTo != "" {
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID: fmt.Sprintf("sup_%s", n.ID), Text: n.Text, Role: "supplementary", AttachesTo: attachTo, SourceQuote: n.SourceQuote,
			})
		}
	}
	state.Nodes = filtered
	return state, nil
}

func stage3Relations(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (graphState, error) {
	keptEdges := make([]graphEdge, 0, len(state.Edges))
	demoted := map[string]struct{}{}
	for _, e := range state.Edges {
		fromNode, okFrom := nodeByID(state.Nodes, e.From)
		toNode, okTo := nodeByID(state.Nodes, e.To)
		if !okFrom || !okTo {
			continue
		}
		req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage3RelationSystemPrompt, fmt.Sprintf(stage3RelationUserPrompt, fromNode.Text, fromNode.SourceQuote, toNode.Text, toNode.SourceQuote))
		if err != nil {
			return graphState{}, err
		}
		resp, err := rt.Call(ctx, req)
		if err != nil {
			return graphState{}, err
		}
		var result struct {
			Relation relationKind `json:"relation"`
			Primary  string       `json:"primary"`
			Reason   string       `json:"reason"`
		}
		if err := parseJSONObject(resp.Text, &result); err != nil {
			return graphState{}, fmt.Errorf("stage3 relation parse: %w", err)
		}
		switch result.Relation {
		case relationCausal:
			keptEdges = append(keptEdges, e)
		case relationSupports:
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID:         fmt.Sprintf("evi_%s_%s", fromNode.ID, toNode.ID),
				Text:       fromNode.Text,
				Role:       "evidence",
				AttachesTo: toNode.ID,
				SourceQuote: fromNode.SourceQuote,
			})
			demoted[fromNode.ID] = struct{}{}
		case relationExplains:
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID:         fmt.Sprintf("exp_%s_%s", fromNode.ID, toNode.ID),
				Text:       fromNode.Text,
				Role:       "explanation",
				AttachesTo: toNode.ID,
				SourceQuote: fromNode.SourceQuote,
			})
			demoted[fromNode.ID] = struct{}{}
		case relationSupplements:
			primaryID := toNode.ID
			secondary := fromNode
			if strings.TrimSpace(result.Primary) == "from" {
				primaryID = fromNode.ID
				secondary = toNode
			}
			state.OffGraph = append(state.OffGraph, offGraphItem{
				ID:         fmt.Sprintf("sup_%s_%s", secondary.ID, primaryID),
				Text:       secondary.Text,
				Role:       "supplementary",
				AttachesTo: primaryID,
				SourceQuote: secondary.SourceQuote,
			})
			demoted[secondary.ID] = struct{}{}
		default:
			// drop the edge, keep both nodes for later topology/ontology handling
		}
	}
	state.Edges = dedupeEdges(keptEdges)
	if len(demoted) > 0 {
		nodes := make([]graphNode, 0, len(state.Nodes))
		for _, n := range state.Nodes {
			if _, ok := demoted[n.ID]; ok {
				continue
			}
			nodes = append(nodes, n)
		}
		state.Nodes = nodes
	}
	return state, nil
}

func stage4Validate(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState, maxRounds int) (graphState, error) {
	if maxRounds <= 0 {
		return state, nil
	}
	paragraphs := splitParagraphs(bundle.TextContext())
	if len(paragraphs) == 0 {
		return state, nil
	}
	for round := 0; round < maxRounds; round++ {
		totalPatches := 0
		for _, para := range paragraphs {
			req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage4SystemPrompt, fmt.Sprintf(stage4UserPrompt, para, serializeNodeList(state.Nodes), serializeEdgeList(state.Edges)))
			if err != nil {
				return graphState{}, err
			}
			resp, err := rt.Call(ctx, req)
			if err != nil {
				return graphState{}, err
			}
			var patch struct {
				MissingNodes []struct {
					Text              string `json:"text"`
					SourceQuote       string `json:"source_quote"`
					SuggestedRoleHint string `json:"suggested_role_hint"`
				} `json:"missing_nodes"`
				MissingEdges []struct {
					FromText string `json:"from_text"`
					ToText   string `json:"to_text"`
				} `json:"missing_edges"`
				Misclassified []struct {
					NodeID string `json:"node_id"`
					Issue  string `json:"issue"`
				} `json:"misclassified"`
			}
			if err := parseJSONObject(resp.Text, &patch); err != nil {
				return graphState{}, fmt.Errorf("stage4 validate parse: %w", err)
			}
			totalPatches += len(patch.MissingNodes) + len(patch.MissingEdges) + len(patch.Misclassified)
			state = applyValidatePatch(state, patch)
		}
		state = stage2Dedup(state)
		var err error
		state, err = stage3Classify(ctx, rt, model, bundle, state)
		if err != nil {
			return graphState{}, err
		}
		state.Rounds++
		if totalPatches < 2 {
			break
		}
	}
	return state, nil
}

func stage5Render(ctx context.Context, rt runtimeChat, model string, bundle compile.Bundle, state graphState) (compile.Output, error) {
	drivers := filterNodesByRole(state.Nodes, roleDriver)
	targets := filterNodesByRole(state.Nodes, roleTarget)
	if len(drivers) == 0 && len(state.Nodes) > 0 {
		drivers = append(drivers, state.Nodes[0])
	}
	if len(targets) == 0 && len(state.Nodes) > 1 {
		targets = append(targets, state.Nodes[len(state.Nodes)-1])
	}
	paths := extractPaths(state, drivers, targets)
	if len(paths) == 0 && len(drivers) > 0 && len(targets) > 0 {
		paths = append(paths, renderedPath{
			driver: drivers[0],
			target: targets[0],
			steps:  []graphNode{{ID: drivers[0].ID, Text: drivers[0].Text}},
		})
		if !hasEdge(state.Edges, drivers[0].ID, targets[0].ID) {
			state.Edges = append(state.Edges, graphEdge{From: drivers[0].ID, To: targets[0].ID})
		}
	}
	translated, err := translateAll(ctx, rt, model, uniqueTexts(drivers, targets, paths, state.OffGraph))
	if err != nil {
		return compile.Output{}, err
	}
	cn := func(id, fallback string) string {
		if value, ok := translated[id]; ok && strings.TrimSpace(value) != "" {
			return value
		}
		return fallback
	}
	driversOut := make([]string, 0, len(drivers))
	for _, d := range drivers {
		driversOut = append(driversOut, cn(d.ID, d.Text))
	}
	targetsOut := make([]string, 0, len(targets))
	for _, t := range targets {
		targetsOut = append(targetsOut, cn(t.ID, t.Text))
	}
	transmission := make([]compile.TransmissionPath, 0, len(paths))
	for _, p := range paths {
		steps := make([]string, 0, len(p.steps))
		for _, s := range p.steps {
			steps = append(steps, cn(s.ID, s.Text))
		}
		transmission = append(transmission, compile.TransmissionPath{
			Driver: cn(p.driver.ID, p.driver.Text),
			Target: cn(p.target.ID, p.target.Text),
			Steps:  steps,
		})
	}
	evidence, explanation, supplementary := renderOffGraph(state.OffGraph, cn)
	summary, err := summarizeChinese(ctx, rt, model, driversOut, targetsOut, transmission, bundle)
	if err != nil {
		return compile.Output{}, err
	}
	graph := compile.ReasoningGraph{}
	for _, n := range state.Nodes {
		kind := compile.NodeMechanism
		form := compile.NodeFormObservation
		function := compile.NodeFunctionTransmission
		switch n.Role {
		case roleDriver:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		case roleTransmission:
			kind = compile.NodeMechanism
			form = compile.NodeFormObservation
			function = compile.NodeFunctionTransmission
		case roleTarget:
			kind = compile.NodeConclusion
			form = compile.NodeFormJudgment
			function = compile.NodeFunctionClaim
			if n.Ontology == "flow" {
				kind = compile.NodeConclusion
			}
		}
		graph.Nodes = append(graph.Nodes, compile.GraphNode{
			ID:         n.ID,
			Kind:       kind,
			Form:       form,
			Function:   function,
			Text:       cn(n.ID, n.Text),
			OccurredAt: bundle.PostedAt,
		})
	}
	for _, e := range state.Edges {
		graph.Edges = append(graph.Edges, compile.GraphEdge{From: e.From, To: e.To, Kind: compile.EdgePositive})
	}
	return compile.Output{
		Summary:            summary,
		Drivers:            driversOut,
		Targets:            targetsOut,
		TransmissionPaths:  transmission,
		EvidenceNodes:      evidence,
		ExplanationNodes:   explanation,
		SupplementaryNodes: supplementary,
		Graph:              graph,
		Details:            compile.HiddenDetails{Caveats: []string{"compile v2 mvp"}},
		Topics:             nil,
		Confidence:         "medium",
	}, nil
}

func applyValidatePatch(state graphState, patch struct {
	MissingNodes []struct {
		Text              string `json:"text"`
		SourceQuote       string `json:"source_quote"`
		SuggestedRoleHint string `json:"suggested_role_hint"`
	} `json:"missing_nodes"`
	MissingEdges []struct {
		FromText string `json:"from_text"`
		ToText   string `json:"to_text"`
	} `json:"missing_edges"`
	Misclassified []struct {
		NodeID string `json:"node_id"`
		Issue  string `json:"issue"`
	} `json:"misclassified"`
}) graphState {
	nextNode := len(state.Nodes) + 1
	textToID := map[string]string{}
	for _, n := range state.Nodes {
		textToID[normalizeText(n.Text)] = n.ID
	}
	for _, item := range patch.MissingNodes {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		key := normalizeText(text)
		if _, ok := textToID[key]; ok {
			continue
		}
		id := fmt.Sprintf("n%d", nextNode)
		nextNode++
		state.Nodes = append(state.Nodes, graphNode{ID: id, Text: text, SourceQuote: strings.TrimSpace(item.SourceQuote)})
		textToID[key] = id
	}
	for _, item := range patch.MissingEdges {
		fromID := textToID[normalizeText(item.FromText)]
		toID := textToID[normalizeText(item.ToText)]
		if fromID == "" || toID == "" || fromID == toID {
			continue
		}
		if !hasEdge(state.Edges, fromID, toID) {
			state.Edges = append(state.Edges, graphEdge{From: fromID, To: toID})
		}
	}
	for _, item := range patch.Misclassified {
		if strings.TrimSpace(item.NodeID) == "" {
			continue
		}
		state.OffGraph = append(state.OffGraph, offGraphItem{
			ID:         fmt.Sprintf("mis_%s", item.NodeID),
			Text:       strings.TrimSpace(item.Issue),
			Role:       "supplementary",
			AttachesTo: strings.TrimSpace(item.NodeID),
		})
	}
	return state
}

type renderedPath struct {
	driver graphNode
	target graphNode
	steps  []graphNode
}

func extractPaths(state graphState, drivers, targets []graphNode) []renderedPath {
	adj := map[string][]string{}
	for _, e := range state.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	var out []renderedPath
	for _, d := range drivers {
		for _, t := range targets {
			pathIDs := shortestPath(adj, d.ID, t.ID)
			if len(pathIDs) < 2 {
				continue
			}
			steps := make([]graphNode, 0, max(0, len(pathIDs)-2))
			for _, id := range pathIDs[1 : len(pathIDs)-1] {
				if node, ok := nodeByID(state.Nodes, id); ok {
					steps = append(steps, node)
				}
			}
			out = append(out, renderedPath{driver: d, target: t, steps: steps})
		}
	}
	return out
}

func hasEdge(edges []graphEdge, from, to string) bool {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func shortestPath(adj map[string][]string, start, target string) []string {
	type item struct {
		id   string
		path []string
	}
	queue := []item{{id: start, path: []string{start}}}
	seen := map[string]struct{}{start: {}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == target {
			return cur.path
		}
		for _, next := range adj[cur.id] {
			if _, ok := seen[next]; ok {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, item{id: next, path: append(append([]string(nil), cur.path...), next)})
		}
	}
	return nil
}

func dedupeEdges(edges []graphEdge) []graphEdge {
	seen := map[string]struct{}{}
	out := make([]graphEdge, 0, len(edges))
	for _, e := range edges {
		key := e.From + "->" + e.To
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	return out
}

func filterNodesByRole(nodes []graphNode, role graphRole) []graphNode {
	out := make([]graphNode, 0)
	for _, n := range nodes {
		if n.Role == role {
			out = append(out, n)
		}
	}
	return out
}

func predecessorOf(edges []graphEdge, id string) string {
	for _, e := range edges {
		if e.To == id {
			return e.From
		}
	}
	return ""
}

func nodeByID(nodes []graphNode, id string) (graphNode, bool) {
	for _, n := range nodes {
		if n.ID == id {
			return n, true
		}
	}
	return graphNode{}, false
}

func normalizeText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
}

func uniqueTexts(nodes []graphNode, targets []graphNode, paths []renderedPath, off []offGraphItem) []map[string]string {
	seen := map[string]struct{}{}
	out := make([]map[string]string, 0)
	add := func(id, text string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, map[string]string{"id": id, "text": text})
	}
	for _, n := range nodes {
		add(n.ID, n.Text)
	}
	for _, n := range targets {
		add(n.ID, n.Text)
	}
	for _, p := range paths {
		add(p.driver.ID, p.driver.Text)
		add(p.target.ID, p.target.Text)
		for _, s := range p.steps {
			add(s.ID, s.Text)
		}
	}
	for _, o := range off {
		add(o.ID, o.Text)
	}
	return out
}

func translateAll(ctx context.Context, rt runtimeChat, model string, items []map[string]string) (map[string]string, error) {
	if len(items) == 0 {
		return map[string]string{}, nil
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	bundle := compile.Bundle{UnitID: "translate", Source: "compilev2", ExternalID: "translate", Content: string(payload)}
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage5TranslateSystemPrompt, fmt.Sprintf(stage5TranslateUserPrompt, string(payload)))
	if err != nil {
		return nil, err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	var result struct {
		Translations []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"translations"`
	}
	if err := parseJSONObject(resp.Text, &result); err != nil {
		return nil, fmt.Errorf("stage5 translate parse: %w", err)
	}
	out := map[string]string{}
	for _, item := range result.Translations {
		out[item.ID] = strings.TrimSpace(item.Text)
	}
	return out, nil
}

func summarizeChinese(ctx context.Context, rt runtimeChat, model string, drivers, targets []string, paths []compile.TransmissionPath, bundle compile.Bundle) (string, error) {
	payload, err := json.Marshal(map[string]any{"drivers": drivers, "targets": targets, "paths": paths})
	if err != nil {
		return "", err
	}
	req, err := compile.BuildQwen36ProviderRequest(model, bundle, stage5SummarySystemPrompt, fmt.Sprintf(stage5SummaryUserPrompt, string(payload)))
	if err != nil {
		return "", err
	}
	resp, err := rt.Call(ctx, req)
	if err != nil {
		return "", err
	}
	var result struct{ Summary string `json:"summary"` }
	if err := parseJSONObject(resp.Text, &result); err != nil {
		return "", fmt.Errorf("stage5 summary parse: %w", err)
	}
	return strings.TrimSpace(result.Summary), nil
}

func renderOffGraph(items []offGraphItem, cn func(id, fallback string) string) (evidence, explanation, supplementary []string) {
	for _, item := range items {
		switch item.Role {
		case "evidence":
			evidence = append(evidence, cn(item.ID, item.Text))
		case "explanation":
			explanation = append(explanation, cn(item.ID, item.Text))
		default:
			supplementary = append(supplementary, cn(item.ID, item.Text))
		}
	}
	return
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func serializeNodeList(nodes []graphNode) string {
	lines := make([]string, 0, len(nodes))
	for _, n := range nodes {
		lines = append(lines, fmt.Sprintf("%s: %s", n.ID, n.Text))
	}
	return strings.Join(lines, "\n")
}

func serializeEdgeList(edges []graphEdge) string {
	lines := make([]string, 0, len(edges))
	for _, e := range edges {
		lines = append(lines, fmt.Sprintf("%s -> %s", e.From, e.To))
	}
	return strings.Join(lines, "\n")
}

func asString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

### `varix/compilev2/prompts.go`

```go
package compilev2

const stage1SystemPrompt = `You are a causal graph extractor for financial analysis articles.

Extract every atomic semantic unit and the causal relations between them.
Do not classify units into drivers / targets / transmission yet.
Prioritize recall over compression.

Rules:
- A node must be single-sided: either a source-side unit or an outcome-side unit.
- A node must not mix cause and effect in one clause.
- If a clause contains both source and changed outcome, split it.
- If conjunction introduces parallel causes or parallel outcomes, split them.
- If uncertain whether an item belongs on the graph or off-graph, prefer including it on the graph.
- Keep node text in the article's original language.
- Keep node text short and self-contained.
- Put data points, statistics, analogies, definitions, and side commentary into off_graph with role evidence/explanation/supplementary and an attaches_to field when possible.

Return JSON only with keys:
{
  "nodes": [{"id":"n1","text":"...","source_quote":"..."}],
  "edges": [{"from":"n1","to":"n2"}],
  "off_graph": [{"id":"o1","text":"...","role":"evidence|explanation|supplementary","attaches_to":"n1","source_quote":"..."}]
}`

const stage1UserPrompt = `Extract a causal graph from the following article.

Article:
%s`

const stage3SystemPrompt = `You are an ontology classifier for financial market outcomes.

Decide whether the node below is a market outcome.
A valid market outcome may be:
- price: price/yield/spread/valuation move
- flow: capital flow / positioning / allocation / holdings / hedging change
- decision: concrete market-relevant policy / rating / allocation decision

Return JSON only:
{"is_market_outcome": true|false, "category":"price|flow|decision|none", "reason":"..."}`

const stage3UserPrompt = `Classify the following node.

Node: %s
Source quote: %s`

const stage3RelationSystemPrompt = `You are a relation judge for financial-analysis graph nodes.

For node A and node B, decide which ONE relation best describes them:
- causal: A genuinely drives or causes B
- supports: A is evidence proving or supporting B
- supplements: A and B substantially express the same market meaning, but one is more direct and the other should be supplementary
- explains: A explains, frames, or interprets B
- none: none of the above

If relation is supplements, set "primary" to "from" or "to" for which node should remain primary.

Return JSON only:
{"relation":"causal|supports|supplements|explains|none","primary":"from|to|none","reason":"..." }`

const stage3RelationUserPrompt = `Node A: %s
Node A source quote: %s

Node B: %s
Node B source quote: %s`

const stage5TranslateSystemPrompt = `You are a financial-Chinese translator.

Translate each input item into concise, natural, professional Chinese suitable for downstream financial analysis output.
- Keep already-Chinese items unchanged.
- Preserve standard market abbreviations and proper nouns in normal Chinese financial usage.
- Keep each translation concise.

Return JSON only:
{"translations":[{"id":"...","text":"..."}]}`

const stage5TranslateUserPrompt = `Translate the following id/text list into Chinese:

%s`

const stage5SummarySystemPrompt = `You are a financial summary writer.

Write one concise Chinese sentence summarizing the thesis package.
- Mention at least one driver and one target.
- Keep it to one sentence.

Return JSON only:
{"summary":"..."}`

const stage5SummaryUserPrompt = `Summarize this thesis package in one Chinese sentence:

%s`

const stage4SystemPrompt = `You are a coverage auditor for a causal graph extracted from a financial article.

For one paragraph, check whether its core causal claims and market-result statements are already represented in the current graph.
Return JSON only with keys:
{
  "missing_nodes":[{"text":"...","source_quote":"...","suggested_role_hint":"upstream|midstream|downstream"}],
  "missing_edges":[{"from_text":"...","to_text":"..."}],
  "misclassified":[{"node_id":"...","issue":"..."}]
}

If no issues, return empty arrays.`

const stage4UserPrompt = `Paragraph:
%s

Current nodes:
%s

Current edges:
%s`
```

### `varix/cmd/cli/compile_commands.go`（与 v2 相关部分）

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	c "github.com/kumaloha/VariX/varix/compile"
	cv2 "github.com/kumaloha/VariX/varix/compilev2"
	"github.com/kumaloha/VariX/varix/ingest/types"
	"github.com/kumaloha/VariX/varix/storage/contentstore"
)

type compileClient interface {
	Compile(ctx context.Context, bundle c.Bundle) (c.Record, error)
	Verify(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error)
	VerifyDetailed(ctx context.Context, bundle c.Bundle, output c.Output) (c.Verification, error)
}

var buildCompileClient = func(projectRoot string) compileClient {
	return c.NewClientFromConfig(projectRoot, nil)
}

var buildCompileClientNoVerify = func(projectRoot string) compileClient {
	return c.NewClientFromConfigNoVerify(projectRoot, nil)
}

var buildCompileClientNoVerifyNoValidate = func(projectRoot string) compileClient {
	return c.NewClientFromConfigNoVerifyNoValidate(projectRoot, nil)
}

var buildCompileClientV2 = func(projectRoot string) compileClient {
	return cv2.NewClientFromConfig(projectRoot, nil)
}

var openSQLiteStore = func(path string) (*contentstore.SQLiteStore, error) {
	return contentstore.NewSQLiteStore(path)
}

func selectCompileClient(projectRoot, pipeline string, noVerify, noValidate bool) (compileClient, error) {
	switch strings.TrimSpace(pipeline) {
	case "", "legacy":
		switch {
		case noVerify && noValidate:
			return buildCompileClientNoVerifyNoValidate(projectRoot), nil
		case noVerify:
			return buildCompileClientNoVerify(projectRoot), nil
		default:
			return buildCompileClient(projectRoot), nil
		}
	case "v2":
		if noVerify || noValidate {
			return nil, fmt.Errorf("--no-verify/--no-validate are not supported with --pipeline v2")
		}
		return buildCompileClientV2(projectRoot), nil
	default:
		return nil, fmt.Errorf("unsupported compile pipeline")
	}
}

func runCompileCommand(args []string, projectRoot string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: varix compile <run|show> ...")
		return 2
	}

	switch args[0] {
	case "run":
		return runCompileRun(args[1:], projectRoot, stdout, stderr)
	case "show":
		return runCompileShow(args[1:], projectRoot, stdout, stderr)
	case "summary":
		return runCompileSummary(args[1:], projectRoot, stdout, stderr)
	case "compare":
		return runCompileCompare(args[1:], projectRoot, stdout, stderr)
	case "card":
		return runCompileCard(args[1:], projectRoot, stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: varix compile <run|show|summary|compare|card> ...")
		return 2
	}
}

func runCompileRun(args []string, projectRoot string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compile run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rawURL := fs.String("url", "", "content url")
	platform := fs.String("platform", "", "content platform")
	externalID := fs.String("id", "", "content external id")
	force := fs.Bool("force", false, "force recompilation even if compiled output already exists")
	noVerify := fs.Bool("no-verify", false, "skip compile-time verification and retrieval")
	noValidate := fs.Bool("no-validate", false, "skip compile output validation (evaluation/debug only)")
	pipeline := fs.String("pipeline", "legacy", "compile pipeline: legacy | v2")
	timeout := fs.Duration("timeout", 10*time.Minute, "compile timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*rawURL) == "" && fs.NArg() > 0 {
		*rawURL = fs.Arg(0)
	}
	if strings.TrimSpace(*rawURL) == "" && (strings.TrimSpace(*platform) == "" || strings.TrimSpace(*externalID) == "") {
		fmt.Fprintln(stderr, "usage: varix compile run --url <url> | --platform <platform> --id <external_id>")
		return 2
	}

	app, err := buildApp(projectRoot)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !*noVerify {
		c.EnableFactWebVerification()
	}
	client, err := selectCompileClient(projectRoot, *pipeline, *noVerify, *noValidate)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if client == nil {
		fmt.Fprintln(stderr, "compile client config missing")
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	store, err := openSQLiteStore(app.Settings.ContentDBPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer store.Close()

	if !*force {
		switch {
		case strings.TrimSpace(*rawURL) != "":
			if parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL); err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
				if record, err := store.GetCompiledOutput(ctx, string(parsed.Platform), parsed.PlatformID); err == nil {
					payload, marshalErr := json.MarshalIndent(record, "", "  ")
					if marshalErr != nil {
						fmt.Fprintln(stderr, marshalErr)
						return 1
					}
					fmt.Fprintln(stdout, string(payload))
					return 0
				}
			}
		case strings.TrimSpace(*platform) != "" && strings.TrimSpace(*externalID) != "":
			if record, err := store.GetCompiledOutput(ctx, *platform, *externalID); err == nil {
				payload, marshalErr := json.MarshalIndent(record, "", "  ")
				if marshalErr != nil {
					fmt.Fprintln(stderr, marshalErr)
					return 1
				}
				fmt.Fprintln(stdout, string(payload))
				return 0
			}
		}
	}

	var raw types.RawContent
	switch {
	case strings.TrimSpace(*rawURL) != "":
		parsed, err := app.Dispatcher.ParseURL(ctx, *rawURL)
		if err == nil && strings.TrimSpace(parsed.PlatformID) != "" {
			existing, getErr := store.GetRawCapture(ctx, string(parsed.Platform), parsed.PlatformID)
			if getErr == nil {
				raw = existing
				break
			}
		}
		items, err := fetchURLItems(ctx, app, *rawURL)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
```
