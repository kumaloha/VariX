package provenance

import "github.com/kumaloha/VariX/varix/ingest/types"

// AppendEvidence applies the shared ingest provenance dedupe rule:
// identical Kind+Value+Weight tuples are recorded only once.
func AppendEvidence(prov *types.Provenance, evidence types.ProvenanceEvidence) *types.Provenance {
	if prov == nil {
		prov = &types.Provenance{}
	}
	for _, existing := range prov.Evidence {
		if existing.Kind == evidence.Kind && existing.Value == evidence.Value && existing.Weight == evidence.Weight {
			return prov
		}
	}
	prov.Evidence = append(prov.Evidence, evidence)
	return prov
}
