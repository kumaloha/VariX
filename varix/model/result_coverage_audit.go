package model

type CoverageAudit struct {
	MissingCategories []string `json:"missingCategories,omitempty"`
	MissingListItems  []string `json:"missingListItems,omitempty"`
	OmittedLedgerIDs  []string `json:"omittedLedgerIds,omitempty"`
}

func (a CoverageAudit) IsZero() bool {
	return len(a.MissingCategories) == 0 &&
		len(a.MissingListItems) == 0 &&
		len(a.OmittedLedgerIDs) == 0
}
