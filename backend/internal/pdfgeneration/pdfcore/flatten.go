package pdfcore

import (
	"encoding/json"
	"fmt"
)

// inlinePlaceholderRenderText makes a document renderable by pdf-core, whose
// schema knows only documentStructure: a clause references a placeholder by a
// bare {"@id"} node, but the label to show (and the filled value, if any) live
// on the typed placeholder in the top-level dcs:contractData registry. This
// copies dcs:label and dcs:value onto each in-content reference so pdf-core
// resolves the visible text without the registry.
func inlinePlaceholderRenderText(payload []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return payload, nil
	}
	registry := placeholderRegistry(doc)
	if len(registry) == 0 {
		return payload, nil
	}
	structure, ok := doc["dcs:documentStructure"].(map[string]any)
	if !ok {
		return payload, nil
	}
	blocks, _, ok := structureLists(structure)
	if !ok {
		return payload, nil
	}
	changed := false
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		content, ok := listValue(block["dcs:content"])
		if !ok {
			continue
		}
		for _, rawSegment := range content {
			segment, ok := rawSegment.(map[string]any)
			if !ok {
				continue
			}
			id := stringValue(segment["@id"])
			node, known := registry[id]
			if id == "" || !known {
				continue
			}
			if label := node["dcs:label"]; label != nil {
				segment["dcs:label"] = label
			}
			if value, present := node["dcs:value"]; present {
				segment["dcs:value"] = value
			}
			changed = true
		}
	}
	if !changed {
		return payload, nil
	}
	enriched, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("re-encode placeholder-inlined document: %w", err)
	}
	return enriched, nil
}

// placeholderRegistry indexes the document's placeholders by @id from the
// top-level dcs:contractData.
func placeholderRegistry(doc map[string]any) map[string]map[string]any {
	registry := map[string]map[string]any{}
	items, _ := listValue(doc["dcs:contractData"])
	for _, raw := range items {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if id := stringValue(node["@id"]); id != "" {
			registry[id] = node
		}
	}
	return registry
}

func structureLists(structure map[string]any) (blocks []any, layout []any, ok bool) {
	blocks, bok := listValue(structure["dcs:blocks"])
	layout, lok := listValue(structure["dcs:layout"])
	return blocks, layout, bok && lok
}

// listValue returns the @list of a JSON-LD list container, or a bare array.
func listValue(value any) ([]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		list, ok := typed["@list"].([]any)
		return list, ok
	case []any:
		return typed, true
	}
	return nil, false
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
