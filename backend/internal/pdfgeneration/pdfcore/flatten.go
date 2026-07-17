package pdfcore

import (
	"encoding/json"
	"fmt"
)

// flattenComposedStructure inlines composed sub-templates into a document's
// structure so pdf-core — whose schema only knows dcs:Section/dcs:Clause/
// dcs:TextBlock blocks — can render it. A dcs:ApprovedTemplate block only
// references a sub-template by DID; the referenced content lives in the
// document's dcs:metadata.dcs:subTemplates snapshot. Each ApprovedTemplate
// block is turned into a dcs:Section whose children are the sub-template's
// (id-remapped) blocks, mirroring the merge the editor UI does for display.
//
// A document with no dcs:ApprovedTemplate blocks is returned byte-for-byte
// unchanged, so simple documents keep their exact pdf-core payload (and
// FileID hash).
func flattenComposedStructure(payload []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		// Not an object we can flatten (pdf-core will report its own error).
		return payload, nil
	}

	structure, ok := doc["dcs:documentStructure"].(map[string]any)
	if !ok {
		return payload, nil
	}
	blocks, layout, ok := structureLists(structure)
	if !ok {
		return payload, nil
	}
	approved := approvedTemplateBlocks(blocks)
	if len(approved) == 0 {
		return payload, nil
	}

	snapshots := subTemplateSnapshots(doc)
	layoutByID := indexByID(layout)

	for _, block := range approved {
		blockID, _ := block["@id"].(string)
		templateDID, _ := block["dcs:templateDid"].(string)
		version := numberValue(block["dcs:version"])
		sub := findSnapshotTemplate(snapshots, templateDID, version)
		if sub == nil {
			return nil, fmt.Errorf("composed block %q references sub-template %s v%d not present in dcs:subTemplates", blockID, templateDID, version)
		}
		subStructure, ok := sub["dcs:documentStructure"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("sub-template %s has no dcs:documentStructure", templateDID)
		}
		subBlocks, subLayout, ok := structureLists(subStructure)
		if !ok {
			return nil, fmt.Errorf("sub-template %s has a malformed document structure", templateDID)
		}
		subRoot := rootLayoutNode(subLayout)
		if subRoot == nil {
			return nil, fmt.Errorf("sub-template %s has no root layout node", templateDID)
		}

		// Namespace every sub id under this block so two references to the
		// same sub-template never collide.
		remap := func(id string) string { return blockID + "::" + id }

		// The ApprovedTemplate block becomes a plain Section container.
		block["@type"] = "dcs:Section"
		delete(block, "dcs:templateDid")
		delete(block, "dcs:version")
		delete(block, "dcs:documentNumber")

		// Inline the sub-template's blocks (id-remapped).
		for _, raw := range subBlocks {
			sb, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			clone := cloneMap(sb)
			clone["@id"] = remap(stringValue(sb["@id"]))
			blocks = append(blocks, clone)
		}

		// Inline the sub-template's non-root layout nodes, and give this
		// block's layout node the sub-root's children.
		for _, raw := range subLayout {
			node, ok := raw.(map[string]any)
			if !ok || boolValue(node["dcs:isRoot"]) {
				continue
			}
			layout = append(layout, map[string]any{
				"@id":          remap(stringValue(node["@id"])),
				"@type":        "dcs:LayoutNode",
				"dcs:children": remappedChildren(node, remap),
			})
		}
		host := layoutByID[blockID]
		if host == nil {
			host = map[string]any{"@id": blockID, "@type": "dcs:LayoutNode", "dcs:children": jsonList(nil)}
			layout = append(layout, host)
			layoutByID[blockID] = host
		}
		host["dcs:children"] = remappedChildren(subRoot, remap)
	}

	structure["dcs:blocks"] = jsonList(blocks)
	structure["dcs:layout"] = jsonList(layout)

	flattened, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("re-encode flattened document: %w", err)
	}
	return flattened, nil
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

func jsonList(items []any) map[string]any {
	if items == nil {
		items = []any{}
	}
	return map[string]any{"@list": items}
}

func approvedTemplateBlocks(blocks []any) []map[string]any {
	var out []map[string]any
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if ok && block["@type"] == "dcs:ApprovedTemplate" {
			out = append(out, block)
		}
	}
	return out
}

func subTemplateSnapshots(doc map[string]any) []any {
	metadata, ok := doc["dcs:metadata"].(map[string]any)
	if !ok {
		return nil
	}
	snapshots, _ := listValue(metadata["dcs:subTemplates"])
	return snapshots
}

func findSnapshotTemplate(snapshots []any, templateDID string, version int) map[string]any {
	for _, raw := range snapshots {
		snapshot, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if stringValue(snapshot["@id"]) != templateDID {
			continue
		}
		if v := numberValue(snapshot["dcs:version"]); v != 0 && v != version {
			continue
		}
		if template, ok := snapshot["dcs:template"].(map[string]any); ok {
			return template
		}
	}
	return nil
}

func indexByID(nodes []any) map[string]map[string]any {
	index := map[string]map[string]any{}
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if id := stringValue(node["@id"]); id != "" {
			index[id] = node
		}
	}
	return index
}

func rootLayoutNode(layout []any) map[string]any {
	for _, raw := range layout {
		node, ok := raw.(map[string]any)
		if ok && boolValue(node["dcs:isRoot"]) {
			return node
		}
	}
	return nil
}

func remappedChildren(node map[string]any, remap func(string) string) map[string]any {
	children, _ := listValue(node["dcs:children"])
	out := make([]any, 0, len(children))
	for _, raw := range children {
		ref, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{"@id": remap(stringValue(ref["@id"]))})
	}
	return jsonList(out)
}

func cloneMap(m map[string]any) map[string]any {
	raw, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func numberValue(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}
