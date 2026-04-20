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
