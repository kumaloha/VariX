package compile

const goldMatchThreshold = 0.38

type goldConcept struct {
	Token   string
	Weight  int
	Aliases []string
}

var goldFinanceConcepts = []goldConcept{
	{Token: "ai", Weight: 2, Aliases: []string{"ai", "artificial intelligence", "人工智能"}},
	{Token: "asset", Weight: 1, Aliases: []string{"asset", "assets", "资产"}},
	{Token: "basel", Weight: 2, Aliases: []string{"basel", "巴塞尔"}},
	{Token: "bitcoin", Weight: 2, Aliases: []string{"bitcoin", "btc", "比特币"}},
	{Token: "bond", Weight: 2, Aliases: []string{"bond", "bonds", "主权债", "国债", "债券"}},
	{Token: "capital", Weight: 1, Aliases: []string{"capital", "fund", "funds", "money", "资金", "资本"}},
	{Token: "cash", Weight: 2, Aliases: []string{"cash", "现金"}},
	{Token: "central_bank", Weight: 2, Aliases: []string{"central bank", "fed", "federal reserve", "央行", "美联储"}},
	{Token: "credit", Weight: 2, Aliases: []string{"credit", "信贷", "信用"}},
	{Token: "debt", Weight: 2, Aliases: []string{"debt", "债务"}},
	{Token: "dollar", Weight: 2, Aliases: []string{"dollar", "usd", "美元"}},
	{Token: "flow", Weight: 2, Aliases: []string{"flow", "flows", "inflow", "inflows", "outflow", "outflows", "reallocate", "reallocation", "rotation", "流入", "流出", "回流", "配置", "转向"}},
	{Token: "foreign", Weight: 2, Aliases: []string{"foreign", "overseas", "global", "海外", "全球"}},
	{Token: "gold", Weight: 2, Aliases: []string{"gold", "黄金"}},
	{Token: "growth", Weight: 2, Aliases: []string{"growth", "增长"}},
	{Token: "gsib", Weight: 2, Aliases: []string{"gsib", "g-sib"}},
	{Token: "hard_asset", Weight: 2, Aliases: []string{"hard asset", "hard assets", "tangible asset", "tangible assets", "real asset", "real assets", "实物资产", "硬资产"}},
	{Token: "inflation", Weight: 2, Aliases: []string{"inflation", "通胀"}},
	{Token: "infrastructure", Weight: 2, Aliases: []string{"infrastructure", "capex", "基建", "资本开支"}},
	{Token: "interest_rate", Weight: 2, Aliases: []string{"interest rate", "interest rates", "rates", "yield", "yields", "利率", "收益率"}},
	{Token: "iran_war", Weight: 2, Aliases: []string{"iran conflict", "iran war", "伊朗战争", "伊朗战事"}},
	{Token: "liquidity", Weight: 2, Aliases: []string{"liquidity", "流动性"}},
	{Token: "loan", Weight: 2, Aliases: []string{"loan", "loans", "lending", "放贷", "贷款"}},
	{Token: "long_rate", Weight: 2, Aliases: []string{"long-term rate", "long-term rates", "long term rate", "long term rates", "长端利率"}},
	{Token: "narrative", Weight: 2, Aliases: []string{"narrative", "exceptionalism", "叙事", "例外论"}},
	{Token: "oil", Weight: 2, Aliases: []string{"oil", "petrodollar", "石油", "油价", "石油美元"}},
	{Token: "political_risk", Weight: 2, Aliases: []string{"political risk", "politicized", "政治风险", "政治化"}},
	{Token: "private_credit", Weight: 3, Aliases: []string{"private credit", "private-credit", "私募信贷"}},
	{Token: "real_yield", Weight: 3, Aliases: []string{"real yield", "real yields", "real return", "real returns", "实际收益率", "真实收益率", "负真实收益率"}},
	{Token: "redemption", Weight: 2, Aliases: []string{"redemption", "redemptions", "赎回"}},
	{Token: "regulation", Weight: 2, Aliases: []string{"regulation", "regulatory", "监管"}},
	{Token: "reserve", Weight: 2, Aliases: []string{"reserve", "reserves", "准备金"}},
	{Token: "risk", Weight: 1, Aliases: []string{"risk", "risks", "风险"}},
	{Token: "safe_haven", Weight: 2, Aliases: []string{"safe haven", "safe-haven", "避险"}},
	{Token: "supply_chain", Weight: 2, Aliases: []string{"supply chain", "供应链", "产业链"}},
	{Token: "us", Weight: 1, Aliases: []string{"u.s.", "us", "usa", "america", "american", "美国"}},
}

type GoldCandidate struct {
	SampleID string `json:"sample_id"`
	Output   Output `json:"output"`
}

type GoldScorecard struct {
	DatasetVersion string            `json:"dataset_version"`
	SampleCount    int               `json:"sample_count"`
	OverallScore   float64           `json:"overall_score"`
	Rollup         GoldScoreRollup   `json:"rollup"`
	Samples        []GoldSampleScore `json:"samples"`
}

type GoldScoreRollup struct {
	SummaryScore    float64 `json:"summary_score"`
	DriversScore    float64 `json:"drivers_score"`
	TargetsScore    float64 `json:"targets_score"`
	StructureScore  float64 `json:"structure_score"`
	ReviewItemCount int     `json:"review_item_count"`
}

type GoldSampleScore struct {
	ID             string           `json:"id"`
	Title          string           `json:"title,omitempty"`
	OverallScore   float64          `json:"overall_score"`
	SummaryScore   float64          `json:"summary_score"`
	Drivers        GoldListScore    `json:"drivers"`
	Targets        GoldListScore    `json:"targets"`
	StructureScore float64          `json:"structure_score"`
	ReviewItems    []GoldReviewItem `json:"review_items,omitempty"`
}

type GoldListScore struct {
	Score          float64     `json:"score"`
	Precision      float64     `json:"precision"`
	Recall         float64     `json:"recall"`
	F1             float64     `json:"f1"`
	GoldCount      int         `json:"gold_count"`
	CandidateCount int         `json:"candidate_count"`
	MatchedCount   int         `json:"matched_count"`
	MissingCount   int         `json:"missing_count"`
	ExtraCount     int         `json:"extra_count"`
	Matches        []GoldMatch `json:"matches,omitempty"`
	MissingGold    []string    `json:"missing_gold,omitempty"`
	ExtraCandidate []string    `json:"extra_candidate,omitempty"`
}

type GoldMatch struct {
	Gold       string  `json:"gold"`
	Candidate  string  `json:"candidate"`
	Similarity float64 `json:"similarity"`
}

type GoldReviewItem struct {
	Severity  string  `json:"severity"`
	Field     string  `json:"field"`
	Kind      string  `json:"kind"`
	Score     float64 `json:"score,omitempty"`
	Gold      string  `json:"gold,omitempty"`
	Candidate string  `json:"candidate,omitempty"`
	Message   string  `json:"message"`
}
