package compile

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompileRedesignPromptBuildersCoverUnifiedThreeCallFlow(t *testing.T) {
	registry := newPromptRegistry("")
	bundle := Bundle{
		UnitID:     "web:redesign",
		Source:     "web",
		ExternalID: "redesign",
		Content:    "root body",
	}
	generated := UnifiedCompileOutput{
		Summary: "增长与回报预期继续压过政治风险定价，因此海外资金继续流入美国资产。",
		Drivers: []string{"美国增长与回报预期继续压过政治风险定价"},
		Targets: []string{"海外资金继续流入美国资产"},
		TransmissionPaths: []TransmissionPath{{
			Driver: "美国增长与回报预期继续压过政治风险定价",
			Target: "海外资金继续流入美国资产",
			Steps:  []string{"资本继续配置美国资产"},
		}},
		EvidenceNodes:    []string{"海外资金继续流入美国资产"},
		ExplanationNodes: []string{"市场仍按美国例外论框架理解风险"},
		Details:          HiddenDetails{Caveats: []string{"judge"}},
	}

	cases := []struct {
		name     string
		build    func() (string, string, error)
		system   []string
		userText []string
	}{
		{
			name: "unified-generator",
			build: func() (string, string, error) {
				system, err := registry.buildUnifiedGeneratorInstruction()
				if err != nil {
					return "", "", err
				}
				user, err := registry.buildUnifiedGeneratorPrompt(bundle)
				return system, user, err
			},
			system: []string{
				"unified compile generator",
				"single main market target",
				"target must be a market outcome",
				"the target may be a narrow pricing move or a broader trading / positioning state",
				"driver must be the shared source of all retained transmission paths",
				"prefer the market outcome most directly supported by the article's main evidence",
				"prefer the directly evidenced current market outcome unless the forecast is clearly the article's primary conclusion",
				"prefer article-native target wording over abstract paraphrase when the source wording already cleanly expresses the market outcome",
				"when a slogan-like trade label only comments on a more basic market result, prefer the underlying market result as the target",
				"`summary`, `drivers`, `targets`, `transmission_paths`, `evidence_nodes`, `explanation_nodes`, `details`, `topics`, `confidence`",
			},
			userText: []string{"Payload:", "Generate the full dominant-thesis package in one pass."},
		},
		{
			name: "unified-challenge",
			build: func() (string, string, error) {
				system, err := registry.buildUnifiedChallengeInstruction()
				if err != nil {
					return "", "", err
				}
				user, err := registry.buildUnifiedChallengePrompt(bundle, generated)
				return system, user, err
			},
			system: []string{
				"unified compile challenger",
				"boundary problems",
				"support / explanation items incorrectly placed on the main transmission spine",
				"items incorrectly promoted to target even though they are not market outcomes",
				"speculative side forecast",
				"directly evidenced current market outcome",
				"over-abstract target wording",
				"slogan-like trade label",
			},
			userText: []string{"Generated draft:", "\"transmission_paths\": [", "Return only corrections worth applying to the generated package."},
		},
		{
			name: "unified-judge",
			build: func() (string, string, error) {
				system, err := registry.buildUnifiedJudgeInstruction()
				if err != nil {
					return "", "", err
				}
				user, err := registry.buildUnifiedJudgePrompt(bundle, generated, UnifiedCompileOutput{Targets: []string{"没有形成 sell America 交易"}})
				return system, user, err
			},
			system: []string{
				"unified compile judge",
				"final adjudicator and final generator",
				"single main market target",
				"all retained transmission paths must converge to the retained target set",
				"prefer the market outcome most directly supported by the article's main evidence",
				"prefer the market outcome most directly supported by the article's main evidence over a speculative side forecast",
				"prefer article-native target wording over abstract paraphrase when possible",
				"prefer the underlying market result over a slogan-like trade label",
				"`summary`, `drivers`, `targets`, `transmission_paths`, `evidence_nodes`, `explanation_nodes`, `details`, `topics`, `confidence`",
			},
			userText: []string{"Challenge corrections:", "\"targets\": [", "Return the final complete compile package."},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			system, user, err := tc.build()
			if err != nil {
				t.Fatalf("build prompts: %v", err)
			}
			for _, want := range tc.system {
				if !strings.Contains(system, want) {
					t.Fatalf("system prompt missing %q in %q", want, system)
				}
			}
			for _, want := range tc.userText {
				if !strings.Contains(user, want) {
					t.Fatalf("user prompt missing %q in %q", want, user)
				}
			}
		})
	}
}

func TestParseOutputPreservesThreeStageFieldsAlongsideCompatibilityGraph(t *testing.T) {
	raw := `{
	  "summary":"增长预期压过政治风险定价，海外资金继续流入美国资产。",
	  "drivers":["  美国增长与回报预期继续压过政治风险定价  "],
	  "targets":["  海外资金继续流入美国资产  "],
	  "transmission_paths":[
	    {
	      "driver":"  美国增长与回报预期继续压过政治风险定价 ",
	      "target":" 海外资金继续流入美国资产 ",
	      "steps":[" 增长与回报预期继续压过政治风险定价 ", " 资本继续配置美国资产 "]
	    }
	  ],
	  "evidence_nodes":[" 海外资金继续流入美国资产 ", " 美元指数并未出现持续性崩跌 "],
	  "explanation_nodes":[" 市场仍按美国例外论框架理解风险 "],
	  "graph":{
	    "nodes":[
	      {"id":"n1","kind":"机制","text":"增长与回报预期继续压过政治风险定价","occurred_at":"2026-04-14T00:00:00Z"},
	      {"id":"n2","kind":"事实","text":"海外资金继续流入美国资产","occurred_at":"2026-04-14T00:00:00Z"}
	    ],
	    "edges":[
	      {"from":"n1","to":"n2","kind":"正向"}
	    ]
	  },
	  "details":{"caveats":["judge"]},
	  "topics":["macro"],
	  "confidence":"high"
	}`

	out, err := ParseOutput(raw)
	if err != nil {
		t.Fatalf("ParseOutput() error = %v", err)
	}

	if got, want := out.Drivers, []string{"美国增长与回报预期继续压过政治风险定价"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Drivers = %#v, want %#v", got, want)
	}
	if got, want := out.Targets, []string{"海外资金继续流入美国资产"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Targets = %#v, want %#v", got, want)
	}
	if len(out.TransmissionPaths) != 1 {
		t.Fatalf("TransmissionPaths = %#v, want 1 path", out.TransmissionPaths)
	}
	if got, want := out.TransmissionPaths[0].Driver, "美国增长与回报预期继续压过政治风险定价"; got != want {
		t.Fatalf("TransmissionPaths[0].Driver = %q, want %q", got, want)
	}
	if got, want := out.TransmissionPaths[0].Steps, []string{"增长与回报预期继续压过政治风险定价", "资本继续配置美国资产"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TransmissionPaths[0].Steps = %#v, want %#v", got, want)
	}
	if got, want := out.EvidenceNodes, []string{"海外资金继续流入美国资产", "美元指数并未出现持续性崩跌"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EvidenceNodes = %#v, want %#v", got, want)
	}
	if got, want := out.ExplanationNodes, []string{"市场仍按美国例外论框架理解风险"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExplanationNodes = %#v, want %#v", got, want)
	}
	if got, want := out.Graph.Nodes[0].Kind, NodeMechanism; got != want {
		t.Fatalf("Graph.Nodes[0].Kind = %q, want %q", got, want)
	}
	if got, want := out.Graph.Edges[0].Kind, EdgePositive; got != want {
		t.Fatalf("Graph.Edges[0].Kind = %q, want %q", got, want)
	}
	wantOccurredAt := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	if !out.Graph.Nodes[0].OccurredAt.Equal(wantOccurredAt) {
		t.Fatalf("Graph.Nodes[0].OccurredAt = %v, want %v", out.Graph.Nodes[0].OccurredAt, wantOccurredAt)
	}
}
