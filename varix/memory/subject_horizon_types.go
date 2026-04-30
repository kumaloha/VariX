package memory

type SubjectHorizonMemory struct {
	UserID             string                   `json:"user_id"`
	Subject            string                   `json:"subject"`
	CanonicalSubject   string                   `json:"canonical_subject,omitempty"`
	Horizon            string                   `json:"horizon"`
	RefreshPolicy      string                   `json:"refresh_policy"`
	WindowStart        string                   `json:"window_start"`
	WindowEnd          string                   `json:"window_end"`
	GeneratedAt        string                   `json:"generated_at"`
	LastRefreshedAt    string                   `json:"last_refreshed_at"`
	NextRefreshAt      string                   `json:"next_refresh_at"`
	CacheStatus        string                   `json:"cache_status"`
	InputHash          string                   `json:"input_hash"`
	SampleCount        int                      `json:"sample_count"`
	SourceCount        int                      `json:"source_count"`
	DominantPattern    string                   `json:"dominant_pattern,omitempty"`
	TrendDirection     string                   `json:"trend_direction,omitempty"`
	VolatilityState    string                   `json:"volatility_state,omitempty"`
	Abstraction        string                   `json:"abstraction,omitempty"`
	KeyChanges         []SubjectHorizonChange   `json:"key_changes,omitempty"`
	DriverClusters     []SubjectHorizonDriver   `json:"driver_clusters,omitempty"`
	Contradictions     []SubjectHorizonConflict `json:"contradictions,omitempty"`
	EvidenceSourceRefs []string                 `json:"evidence_source_refs,omitempty"`
}

type SubjectHorizonChange struct {
	When             string                `json:"when,omitempty"`
	Subject          string                `json:"subject"`
	ChangeText       string                `json:"change_text"`
	RelationToPrior  SubjectChangeRelation `json:"relation_to_prior"`
	SourcePlatform   string                `json:"source_platform"`
	SourceExternalID string                `json:"source_external_id"`
	NodeID           string                `json:"node_id"`
}

type SubjectHorizonDriver struct {
	Subject       string   `json:"subject"`
	Changes       []string `json:"changes"`
	Count         int      `json:"count"`
	RelationPaths []string `json:"relation_paths,omitempty"`
	SourceRefs    []string `json:"source_refs,omitempty"`
}

type SubjectHorizonConflict struct {
	PreviousChange string `json:"previous_change"`
	CurrentChange  string `json:"current_change"`
	At             string `json:"at,omitempty"`
}
