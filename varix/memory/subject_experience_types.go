package memory

type SubjectExperienceMemory struct {
	UserID             string                     `json:"user_id"`
	Subject            string                     `json:"subject"`
	CanonicalSubject   string                     `json:"canonical_subject,omitempty"`
	Horizons           []string                   `json:"horizons"`
	GeneratedAt        string                     `json:"generated_at"`
	CacheStatus        string                     `json:"cache_status"`
	InputHash          string                     `json:"input_hash"`
	LessonCount        int                        `json:"lesson_count"`
	Abstraction        string                     `json:"abstraction,omitempty"`
	HorizonSummaries   []SubjectExperienceHorizon `json:"horizon_summaries,omitempty"`
	AttributionSummary SubjectAttributionSummary  `json:"attribution_summary,omitempty"`
	Lessons            []SubjectExperienceLesson  `json:"lessons,omitempty"`
	EvidenceSourceRefs []string                   `json:"evidence_source_refs,omitempty"`
}

type SubjectExperienceHorizon struct {
	Horizon         string   `json:"horizon"`
	SampleCount     int      `json:"sample_count"`
	TrendDirection  string   `json:"trend_direction,omitempty"`
	VolatilityState string   `json:"volatility_state,omitempty"`
	TopDrivers      []string `json:"top_drivers,omitempty"`
}

type SubjectExperienceLesson struct {
	ID                 string   `json:"id"`
	Kind               string   `json:"kind"`
	Statement          string   `json:"statement"`
	Trigger            string   `json:"trigger,omitempty"`
	Mechanism          string   `json:"mechanism,omitempty"`
	Implication        string   `json:"implication,omitempty"`
	Boundary           string   `json:"boundary,omitempty"`
	TransferRule       string   `json:"transfer_rule,omitempty"`
	TimeScaleMeaning   string   `json:"time_scale_meaning,omitempty"`
	Confidence         float64  `json:"confidence"`
	SupportCount       int      `json:"support_count"`
	Horizons           []string `json:"horizons,omitempty"`
	DriverSubjects     []string `json:"driver_subjects,omitempty"`
	EvidenceSourceRefs []string `json:"evidence_source_refs,omitempty"`
	CounterEvidence    []string `json:"counter_evidence,omitempty"`
}

type SubjectAttributionSummary struct {
	ChangeCount        int                         `json:"change_count"`
	FactorCount        int                         `json:"factor_count"`
	PrimaryFactor      SubjectPrimaryFactor        `json:"primary_factor,omitempty"`
	ChangeAttributions []SubjectChangeAttribution  `json:"change_attributions,omitempty"`
	FactorRelations    []SubjectFactorRelationship `json:"factor_relations,omitempty"`
}

type SubjectPrimaryFactor struct {
	Subject     string   `json:"subject,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	Support     int      `json:"support"`
	SourceCount int      `json:"source_count"`
	Horizons    []string `json:"horizons,omitempty"`
}

type SubjectChangeAttribution struct {
	When             string   `json:"when,omitempty"`
	ChangeText       string   `json:"change_text"`
	Factors          []string `json:"factors,omitempty"`
	SourceExternalID string   `json:"source_external_id,omitempty"`
}

type SubjectFactorRelationship struct {
	Factors     []string `json:"factors,omitempty"`
	Left        string   `json:"left,omitempty"`
	Right       string   `json:"right,omitempty"`
	Relation    string   `json:"relation"`
	SourceCount int      `json:"source_count"`
	SourceRefs  []string `json:"source_refs,omitempty"`
}
