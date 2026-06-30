package base

import (
	"encoding/json"
	"fmt"
)

func DerefInt(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}

func DerefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func ConvertAny[T any](raw any) (*T, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &out, nil
}
