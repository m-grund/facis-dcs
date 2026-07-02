package base

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
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

func Unique(lists ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, list := range lists {
		for _, s := range list {
			if _, ok := seen[s]; !ok {
				seen[s] = struct{}{}
				result = append(result, s)
			}
		}
	}
	return result
}

func GetEnvOrDefault[T string | bool](key string, def T) T {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return def
	}

	var result any
	switch any(def).(type) {
	case string:
		result = raw
	case bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return def // or: log the invalid value
		}
		result = b
	}
	return result.(T)
}
