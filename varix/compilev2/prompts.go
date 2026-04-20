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

const stage3RelationSystemPrompt = `You are a relation planner for financial-analysis graph nodes.

Given the full node set, decide which pairs form:
- causal_edges: true driver/transmission/target causal relations
- support_links: one node is evidence for another
- supplement_links: two nodes substantially express the same market meaning, but one should remain primary and the other should become supplementary
- explanation_links: one node explains or frames another

Rules:
- Only output links that are strongly justified.
- Do not force every node into a link.
- If two nodes say almost the same thing, prefer supplement_links instead of causal_edges.
- If one node merely proves another, prefer support_links instead of causal_edges.
- If one node interprets or frames another, prefer explanation_links.
- If both nodes are themselves market outcomes / result-like statements, do NOT classify them as support_links by default. Prefer supplement_links when one is a label, restatement, slogan, or narrative wrapper around the other.
- Labels or narrative wrappers such as trade names, sloganized framing, or "X narrative" / "X trade" wording should usually become the secondary side of a supplement relation rather than the primary retained result.

Return JSON only:
{
  "causal_edges":[{"from":"n1","to":"n2"}],
  "support_links":[{"from":"n3","to":"n4"}],
  "supplement_links":[{"primary":"n5","secondary":"n6"}],
  "explanation_links":[{"from":"n7","to":"n8"}]
}`

const stage3RelationUserPrompt = `Plan relations over the following node set. Each line is:
node_id | node_text | role=<role> | ontology=<ontology> | quote=<source_quote>

%s`

const stage2SystemPrompt = `You are a semantic equivalence judge for financial-analysis graph nodes.

Decide whether node A and node B express substantially the same market-relevant fact, state, or outcome.
When uncertain, return false. Over-merging is worse than under-merging.

Return JSON only:
{"equivalent": true|false, "reason":"..." }`

const stage2UserPrompt = `Node A: %s
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
