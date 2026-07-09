package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ContractTemplate is the Go representation of a dcs:ContractTemplate node in
// its canonical compact JSON-LD form. Field names match the compact keys
// produced by CanonicalizePayload so json.Unmarshal works directly.
//
// Policies keys off the fully expanded IRI rather than the bare `policies`
// term: the hosted context types `policies` as xsd:anyURI, so a node-object
// value (the odrl:Set) is not compacted to the short term during canonicalization.
type ContractTemplate struct {
	Context           json.RawMessage    `json:"@context,omitempty"`
	ID                string             `json:"@id,omitempty"`
	Type              string             `json:"@type,omitempty"`
	Metadata          *TemplateMetadata  `json:"metadata,omitempty"`
	DocumentStructure *DocumentStructure `json:"documentStructure,omitempty"`
	SignatureFields   []SignatureField   `json:"signatureFields,omitempty"`
	Policies          *OdrlSet           `json:"https://w3id.org/facis/dcs/ontology/v1#policies,omitempty"`
}

// OdrlRef is an ODRL node reference carrying a single @id IRI (action, party,
// target, operator, left operand, profile).
type OdrlRef struct {
	ID string `json:"@id,omitempty"`
}

// OdrlConstraint holds one odrl:Constraint. RightOperand is kept raw because
// the canonical form collapses it to a bare scalar, an array of scalars, or
// (uncompacted) @value objects; odrlRightOperandValues resolves all three.
type OdrlConstraint struct {
	LeftOperand  OdrlRef         `json:"odrl:leftOperand"`
	Operator     OdrlRef         `json:"odrl:operator"`
	RightOperand json.RawMessage `json:"odrl:rightOperand,omitempty"`
}

// OdrlRule holds one odrl:Duty, odrl:Permission, or odrl:Prohibition.
type OdrlRule struct {
	ID         string          `json:"@id,omitempty"`
	Type       string          `json:"@type,omitempty"`
	Action     OdrlRef         `json:"odrl:action"`
	Assigner   OdrlRef         `json:"odrl:assigner"`
	Assignee   OdrlRef         `json:"odrl:assignee"`
	Target     OdrlRef         `json:"odrl:target"`
	Constraint *OdrlConstraint `json:"odrl:constraint,omitempty"`
}

// OdrlSet is the single enclosing odrl:Set policy container of dcs:policies.
type OdrlSet struct {
	ID          string       `json:"@id,omitempty"`
	Type        string       `json:"@type,omitempty"`
	Profile     OdrlRef      `json:"odrl:profile"`
	Duty        odrlRuleList `json:"odrl:duty,omitempty"`
	Permission  odrlRuleList `json:"odrl:permission,omitempty"`
	Prohibition odrlRuleList `json:"odrl:prohibition,omitempty"`
}

// UnmarshalJSON accepts the real odrl:Set object shape, but also tolerates the
// empty-array shape (`"dcs:policies": []`) that a contract with no policies
// at all legitimately carries (the structural validation rule is: absent,
// empty array, or a single odrl:Set — never a non-empty flat array). An empty
// array decodes to a zero-value OdrlSet, which buildPolicySection already
// treats as "nothing to render".
func (s *OdrlSet) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		if len(arr) == 0 {
			*s = OdrlSet{}
			return nil
		}
		return fmt.Errorf("dcs:policies: non-empty flat array is not a valid odrl:Set (expected an object or an empty array)")
	}
	type plain OdrlSet
	return json.Unmarshal(b, (*plain)(s))
}

// odrlRuleList accepts a rule bucket that the canonical form emits either as a
// JSON array (multiple rules) or as a single object (exactly one rule, which
// JSON-LD compaction collapses out of its array).
type odrlRuleList []OdrlRule

func (l *odrlRuleList) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []OdrlRule
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*l = arr
		return nil
	}
	var single OdrlRule
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*l = odrlRuleList{single}
	return nil
}

// TemplateMetadata holds the dcs:TemplateMetadata node.
type TemplateMetadata struct {
	Type  string `json:"@type,omitempty"`
	Title string `json:"title"`
}

// DocumentStructure holds the dcs:DocumentStructure node.
// Layout and Blocks are ordered lists; with @container:@list in the stable
// context they always deserialize as JSON arrays, never as scalars.
type DocumentStructure struct {
	Type   string       `json:"@type,omitempty"`
	Layout []LayoutNode `json:"layout"`
	Blocks []Block      `json:"blocks"`
}

// LayoutNode describes one node in the document tree. Children are IRI strings
// referencing blocks (or other layout nodes) by @id.
type LayoutNode struct {
	ID       string   `json:"@id,omitempty"`
	Type     string   `json:"@type,omitempty"`
	IsRoot   bool     `json:"isRoot,omitempty"`
	Children []string `json:"children,omitempty"`
}

// Block holds any dcs:Section, dcs:Clause, or dcs:TextBlock node. All three
// share the same compact keys, so one struct covers all types; the Type field
// discriminates (values: "Section", "Clause", "TextBlock").
type Block struct {
	ID      string            `json:"@id,omitempty"`
	Type    string            `json:"@type"`
	Title   string            `json:"title,omitempty"`
	Text    string            `json:"text,omitempty"`
	Content []json.RawMessage `json:"content,omitempty"`
}

// SignatureField holds a dcs:SignatureField node.
type SignatureField struct {
	ID            string `json:"@id"`
	Type          string `json:"@type,omitempty"`
	SignatoryName string `json:"signatoryName"`
	Title         string `json:"title,omitempty"`
}

// ContentItem represents one element in a Clause/TextBlock content list.
// The compact canonical form can be:
//   - a plain string   → Value is set, ID and Datatype are empty
//   - a typed literal  → Value and Datatype are set  (e.g. xsd:decimal)
//   - an IRI reference → ID is set  (dcs/odrl/external term)
//   - an external link → ID is set (schema:url carried on same node)
type ContentItem struct {
	Value    string          `json:"@value,omitempty"`
	Datatype string          `json:"@type,omitempty"`
	ID       string          `json:"@id,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// UnmarshalJSON handles the two surface forms a content item can take in the
// canonical compact form: a plain JSON string or a JSON object.
func (c *ContentItem) UnmarshalJSON(b []byte) error {
	c.Raw = append(json.RawMessage(nil), b...)
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		c.Value = s
		return nil
	}
	type plain ContentItem
	return json.Unmarshal(b, (*plain)(c))
}
