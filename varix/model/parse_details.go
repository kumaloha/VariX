package model

import (
	"encoding/json"
	"fmt"
)

func parseHiddenDetails(raw json.RawMessage) (HiddenDetails, error) {
	var details HiddenDetails
	if len(raw) == 0 || string(raw) == "null" {
		return details, nil
	}

	var object HiddenDetails
	if err := json.Unmarshal(raw, &object); err == nil {
		return object, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		details.Caveats = list
		return details, nil
	}

	var objects []map[string]any
	if err := json.Unmarshal(raw, &objects); err == nil {
		details.Items = objects
		return details, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		details.Caveats = []string{text}
		return details, nil
	}

	return HiddenDetails{}, fmt.Errorf("parse compile details: unsupported shape")
}
