// swaggerpatch post-processes the Goa-generated openapi3.yaml to add requestBody
// sections that Goa omits when SkipRequestBodyEncodeDecode is used, and replaces
// generated Lorem-Ipsum examples with valid values.
//
// Usage: swaggerpatch <openapi3.yaml>
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: swaggerpatch <openapi3.yaml>")
		os.Exit(1)
	}
	path := os.Args[1]

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
		os.Exit(1)
	}

	// Parse as a generic node tree to preserve YAML structure.
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", path, err)
		os.Exit(1)
	}

	patchDoc(&root)

	out, err := yaml.Marshal(&root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
		os.Exit(1)
	}
}

// ---- per-endpoint configuration -------------------------------------------

type endpointConfig struct {
	contentTypeExample string
	requestBody        *yaml.Node
}

var endpoints = map[string]endpointConfig{
	"/download": {
		contentTypeExample: "application/ld+json",
		requestBody:        jsonLDRequestBody("JSON-LD or plain-JSON semantic payload"),
	},
	"/verify": {
		contentTypeExample: "application/pdf",
		requestBody:        binaryRequestBody("application/pdf", "Compiled PDF document to verify"),
	},
	"/update": {
		contentTypeExample: "multipart/form-data",
		requestBody:        multipartRequestBody(),
	},
	"/claim": {
		contentTypeExample: "multipart/form-data",
		requestBody:        claimRequestBody(),
	},
}

// ---- YAML node builders ---------------------------------------------------

func mappingNode(pairs ...interface{}) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i < len(pairs); i += 2 {
		key := pairs[i].(string)
		val := pairs[i+1].(*yaml.Node)
		n.Content = append(n.Content, strNode(key), val)
	}
	return n
}

func strNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

func boolNode(b bool) *yaml.Node {
	v := "false"
	if b {
		v = "true"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: v}
}

func seqNode(items ...*yaml.Node) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	n.Content = append(n.Content, items...)
	return n
}

func binarySchema(description string) *yaml.Node {
	return mappingNode(
		"type", strNode("string"),
		"format", strNode("binary"),
		"description", strNode(description),
	)
}

func mediaTypeNode(schema *yaml.Node) *yaml.Node {
	return mappingNode("schema", schema)
}

func binaryRequestBody(contentType, description string) *yaml.Node {
	return mappingNode(
		"required", boolNode(true),
		"content", mappingNode(
			contentType, mediaTypeNode(binarySchema(description)),
		),
	)
}

func jsonLDRequestBody(description string) *yaml.Node {
	return mappingNode(
		"required", boolNode(true),
		"content", mappingNode(
			"application/ld+json", mediaTypeNode(binarySchema(description)),
			"application/json", mediaTypeNode(binarySchema(description)),
		),
	)
}

func multipartRequestBody() *yaml.Node {
	schema := mappingNode(
		"type", strNode("object"),
		"required", seqNode(strNode("pdf"), strNode("payload")),
		"properties", mappingNode(
			"pdf", mappingNode(
				"type", strNode("string"),
				"format", strNode("binary"),
				"description", strNode("Existing PDF document to amend"),
			),
			"payload", mappingNode(
				"type", strNode("string"),
				"format", strNode("binary"),
				"description", strNode("New JSON-LD semantic payload"),
			),
		),
	)
	return mappingNode(
		"required", boolNode(true),
		"content", mappingNode(
			"multipart/form-data", mediaTypeNode(schema),
		),
	)
}

func claimRequestBody() *yaml.Node {
	schema := mappingNode(
		"type", strNode("object"),
		"required", seqNode(strNode("pdf"), strNode("payload")),
		"properties", mappingNode(
			"pdf", mappingNode(
				"type", strNode("string"),
				"format", strNode("binary"),
				"description", strNode("PDF whose page content is being claimed (embedded JSON-LD need not be present)"),
			),
			"payload", mappingNode(
				"type", strNode("string"),
				"format", strNode("binary"),
				"description", strNode("JSON-LD document claiming to produce the submitted PDF"),
			),
		),
	)
	return mappingNode(
		"required", boolNode(true),
		"content", mappingNode(
			"multipart/form-data", mediaTypeNode(schema),
		),
	)
}

// ---- document-level patching ---------------------------------------------

func patchDoc(root *yaml.Node) {
	// root is a document node; its first child is the top-level mapping.
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return
	}
	top := root.Content[0]
	pathsNode := mappingValue(top, "paths")
	if pathsNode == nil {
		return
	}
	// Iterate path entries.
	for i := 0; i+1 < len(pathsNode.Content); i += 2 {
		pathKey := pathsNode.Content[i].Value
		pathItem := pathsNode.Content[i+1]
		cfg, ok := endpoints[pathKey]
		if !ok {
			continue
		}
		postNode := mappingValue(pathItem, "post")
		if postNode == nil {
			continue
		}
		// Add requestBody if not already present.
		if mappingValue(postNode, "requestBody") == nil {
			postNode.Content = append(postNode.Content,
				strNode("requestBody"), cfg.requestBody,
			)
		}
		// Fix Content-Type header example.
		fixContentTypeExample(postNode, cfg.contentTypeExample)
		// Remove byte-array response examples (Lorem Ipsum noise from Goa faker).
		cleanResponseExamples(postNode)
	}
}

// fixContentTypeExample sets a valid example string on the Content-Type
// header parameter (both at the parameter level and in its schema).
func fixContentTypeExample(postNode *yaml.Node, example string) {
	params := mappingValue(postNode, "parameters")
	if params == nil {
		return
	}
	for _, param := range params.Content {
		if param.Kind != yaml.MappingNode {
			continue
		}
		if v := mappingValue(param, "name"); v == nil || v.Value != "Content-Type" {
			continue
		}
		setOrReplace(param, "example", strNode(example))
		if schema := mappingValue(param, "schema"); schema != nil {
			setOrReplace(schema, "example", strNode(example))
		}
	}
}

// cleanResponseExamples removes byte-array example values Goa faker emits for
// binary response bodies (arrays of integers – useless in documentation).
func cleanResponseExamples(postNode *yaml.Node) {
	responses := mappingValue(postNode, "responses")
	if responses == nil {
		return
	}
	for i := 1; i < len(responses.Content); i += 2 {
		resp := responses.Content[i]
		content := mappingValue(resp, "content")
		if content == nil {
			continue
		}
		for j := 1; j < len(content.Content); j += 2 {
			media := content.Content[j]
			deleteKey(media, "example")
			if schema := mappingValue(media, "schema"); schema != nil {
				deleteKey(schema, "example")
			}
		}
	}
}

// ---- yaml.Node helpers ---------------------------------------------------

// mappingValue returns the value node for the given key in a mapping node,
// or nil if not found.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setOrReplace sets key=val in a mapping node, replacing any existing value.
func setOrReplace(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, strNode(key), val)
}

// deleteKey removes a key-value pair from a mapping node.
func deleteKey(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
