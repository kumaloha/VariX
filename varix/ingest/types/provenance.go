package types

type BaseRelation string
type EditorialLayer string
type Confidence string
type SourceLookupStatus string
type SourceMatchKind string
type Fidelity string

const (
	BaseRelationOriginal       BaseRelation = "original"
	BaseRelationRepost         BaseRelation = "repost"
	BaseRelationQuote          BaseRelation = "quote"
	BaseRelationExcerpt        BaseRelation = "excerpt"
	BaseRelationTranslation    BaseRelation = "translation"
	BaseRelationSummary        BaseRelation = "summary"
	BaseRelationCompilation    BaseRelation = "compilation"
	BaseRelationInterviewRecut BaseRelation = "interview_recut"
	BaseRelationUnknown        BaseRelation = "unknown"
)

const (
	EditorialLayerNone       EditorialLayer = "none"
	EditorialLayerCommentary EditorialLayer = "commentary"
	EditorialLayerAnalysis   EditorialLayer = "analysis"
	EditorialLayerReaction   EditorialLayer = "reaction"
	EditorialLayerFraming    EditorialLayer = "framing"
	EditorialLayerUnknown    EditorialLayer = "unknown"
)

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

const (
	SourceLookupStatusNotNeeded SourceLookupStatus = "not_needed"
	SourceLookupStatusPending   SourceLookupStatus = "pending"
	SourceLookupStatusFound     SourceLookupStatus = "found"
	SourceLookupStatusNotFound  SourceLookupStatus = "not_found"
	SourceLookupStatusFailed    SourceLookupStatus = "failed"
)

const (
	SourceMatchSameSource    SourceMatchKind = "same_source"
	SourceMatchLikelyDerived SourceMatchKind = "likely_derived"
	SourceMatchUnrelated     SourceMatchKind = "unrelated"
)

const (
	FidelityUnknown        Fidelity = "unknown"
	FidelityPartial        Fidelity = "partial"
	FidelityLikelyFaithful Fidelity = "likely_faithful"
	FidelityLikelyAdapted  Fidelity = "likely_adapted"
)

type Provenance struct {
	BaseRelation      BaseRelation         `json:"base_relation,omitempty"`
	EditorialLayer    EditorialLayer       `json:"editorial_layer,omitempty"`
	Confidence        Confidence           `json:"confidence,omitempty"`
	NeedsSourceLookup bool                 `json:"needs_source_lookup,omitempty"`
	ClaimedSpeakers   []string             `json:"claimed_speakers,omitempty"`
	SourceCandidates  []SourceCandidate    `json:"source_candidates,omitempty"`
	Evidence          []ProvenanceEvidence `json:"evidence,omitempty"`
	SourceLookup      SourceLookupState    `json:"source_lookup,omitempty"`
	Fidelity          Fidelity             `json:"fidelity,omitempty"`
}

type SourceCandidate struct {
	URL        string `json:"url,omitempty"`
	Host       string `json:"host,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

type ProvenanceEvidence struct {
	Kind   string `json:"kind,omitempty"`
	Value  string `json:"value,omitempty"`
	Weight string `json:"weight,omitempty"`
}

type SourceLookupState struct {
	Status             SourceLookupStatus `json:"status,omitempty"`
	CanonicalSourceURL string             `json:"canonical_source_url,omitempty"`
	ResolvedBy         string             `json:"resolved_by,omitempty"`
	MatchKind          SourceMatchKind    `json:"match_kind,omitempty"`
}
