package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

func ChangedFields(before, after any) (any, any, bool, error) {
	normalizedBefore, err := normalizeJSON(before)
	if err != nil {
		return nil, nil, false, fmt.Errorf("normalize before state: %w", err)
	}
	normalizedAfter, err := normalizeJSON(after)
	if err != nil {
		return nil, nil, false, fmt.Errorf("normalize after state: %w", err)
	}
	beforeChanges, afterChanges, changed := changedValue(normalizedBefore, normalizedAfter)
	return beforeChanges, afterChanges, changed, nil
}

func normalizeJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func changedValue(before, after any) (any, any, bool) {
	beforeMap, beforeIsMap := before.(map[string]any)
	afterMap, afterIsMap := after.(map[string]any)
	if beforeIsMap && afterIsMap {
		beforeChanges := map[string]any{}
		afterChanges := map[string]any{}
		keys := make(map[string]struct{}, len(beforeMap)+len(afterMap))
		for key := range beforeMap {
			keys[key] = struct{}{}
		}
		for key := range afterMap {
			keys[key] = struct{}{}
		}
		for key := range keys {
			beforeValue, beforeExists := beforeMap[key]
			afterValue, afterExists := afterMap[key]
			var beforeChild, afterChild any
			var changed bool
			if beforeExists != afterExists {
				beforeChild, afterChild, changed = beforeValue, afterValue, true
				if !beforeExists {
					beforeChild = map[string]any{"_missing": true}
				}
				if !afterExists {
					afterChild = map[string]any{"_missing": true}
				}
			} else {
				beforeChild, afterChild, changed = changedValue(beforeValue, afterValue)
			}
			if changed {
				beforeChanges[key] = beforeChild
				afterChanges[key] = afterChild
			}
		}
		if len(beforeChanges) == 0 {
			return nil, nil, false
		}
		return beforeChanges, afterChanges, true
	}
	if reflect.DeepEqual(before, after) {
		return nil, nil, false
	}
	return before, after, true
}
