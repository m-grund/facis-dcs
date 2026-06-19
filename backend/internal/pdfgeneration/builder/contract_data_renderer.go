package builder

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-pdf/fpdf"
)

// ---- JSON structs mirroring the frontend ContractData / DocumentBlock model ----

type contractDataJSON struct {
	DocumentOutline         []outlineNodeJSON         `json:"documentOutline"`
	DocumentBlocks          []json.RawMessage         `json:"documentBlocks"`
	SemanticConditions      []semanticConditionJSON   `json:"semanticConditions"`
	SemanticConditionValues []conditionValueJSON      `json:"semanticConditionValues"`
	SubTemplateSnapshots    []subTemplateSnapshotJSON `json:"subTemplateSnapshots"`
}

type outlineNodeJSON struct {
	BlockID  string   `json:"blockId"`
	IsRoot   bool     `json:"isRoot"`
	Children []string `json:"children"`
}

// baseBlockJSON covers all block types; type-specific fields are optional.
type baseBlockJSON struct {
	BlockID        string `json:"blockId"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Text           string `json:"text"`
	TemplateID     string `json:"templateId"`
	Version        int    `json:"version"`
	DocumentNumber string `json:"document_number"`
}

type semanticConditionJSON struct {
	ConditionID   string `json:"conditionId"`
	ConditionName string `json:"conditionName"`
}

type conditionValueJSON struct {
	BlockID        string          `json:"blockId"`
	ConditionID    string          `json:"conditionId"`
	ParameterName  string          `json:"parameterName"`
	ParameterValue json.RawMessage `json:"parameterValue"`
}

type subTemplateSnapshotJSON struct {
	DID            string            `json:"did"`
	Version        int               `json:"version"`
	DocumentNumber string            `json:"document_number"`
	TemplateData   *templateDataJSON `json:"template_data"`
}

type templateDataJSON struct {
	DocumentOutline    []outlineNodeJSON       `json:"documentOutline"`
	DocumentBlocks     []json.RawMessage       `json:"documentBlocks"`
	SemanticConditions []semanticConditionJSON `json:"semanticConditions"`
}

type canonicalEnvelopeJSON struct {
	DocumentStructure       canonicalDocumentStructureJSON `json:"dcs:documentStructure"`
	Metadata                canonicalMetadataJSON          `json:"dcs:metadata"`
	ContractData            []canonicalRequirementJSON     `json:"dcs:contractData"`
	SemanticConditionValues []conditionValueJSON           `json:"semanticConditionValues"`
}

type canonicalMetadataJSON struct {
	SubTemplates []canonicalSubTemplateJSON `json:"dcs:subTemplates"`
}

type canonicalSubTemplateJSON struct {
	ID       string                `json:"@id"`
	Template canonicalEnvelopeJSON `json:"dcs:template"`
}

type canonicalDocumentStructureJSON struct {
	Blocks []json.RawMessage `json:"dcs:blocks"`
	Layout []canonicalLayout `json:"dcs:layout"`
}

type canonicalLayout struct {
	ID       string                 `json:"@id"`
	IsRoot   bool                   `json:"dcs:isRoot"`
	Children canonicalReferenceList `json:"dcs:children"`
}

type canonicalReferenceList struct {
	List []canonicalReference `json:"@list"`
}

type canonicalReference struct {
	ID string `json:"@id"`
}

type canonicalBlockJSON struct {
	ID             string          `json:"@id"`
	Type           string          `json:"@type"`
	Title          string          `json:"dcs:title"`
	Text           string          `json:"dcs:text"`
	Content        json.RawMessage `json:"dcs:content"`
	TemplateDID    string          `json:"dcs:templateDid"`
	Version        int             `json:"dcs:version"`
	DocumentNumber string          `json:"dcs:documentNumber"`
}

type canonicalContentList struct {
	List []json.RawMessage `json:"@list"`
}

type canonicalRequirementJSON struct {
	Type        string               `json:"@type"`
	ConditionID string               `json:"dcs:conditionId"`
	Fields      []canonicalFieldJSON `json:"dcs:fields"`
}

type canonicalFieldJSON struct {
	ID            string `json:"@id"`
	ParameterName string `json:"dcs:parameterName"`
	SemanticPath  string `json:"dcs:semanticPath"`
}

// ---- Render context ----

type contractRenderCtx struct {
	blockMap        map[string]baseBlockJSON
	outlineMap      map[string]outlineNodeJSON
	rootBlockIDs    []string
	conditions      []semanticConditionJSON
	conditionValues []conditionValueJSON
	snapshots       []subTemplateSnapshotJSON
}

func buildRenderCtx(data *contractDataJSON) contractRenderCtx {
	ctx := contractRenderCtx{
		blockMap:        make(map[string]baseBlockJSON, len(data.DocumentBlocks)),
		outlineMap:      make(map[string]outlineNodeJSON, len(data.DocumentOutline)),
		conditions:      data.SemanticConditions,
		conditionValues: data.SemanticConditionValues,
		snapshots:       data.SubTemplateSnapshots,
	}
	for _, raw := range data.DocumentBlocks {
		var b baseBlockJSON
		if json.Unmarshal(raw, &b) == nil {
			ctx.blockMap[b.BlockID] = b
		}
	}
	for _, node := range data.DocumentOutline {
		ctx.outlineMap[node.BlockID] = node
		if node.IsRoot {
			ctx.rootBlockIDs = node.Children
		}
	}
	return ctx
}

func buildRenderCtxFromTemplate(td *templateDataJSON, parentCtx *contractRenderCtx) contractRenderCtx {
	ctx := contractRenderCtx{
		blockMap:        make(map[string]baseBlockJSON, len(td.DocumentBlocks)),
		outlineMap:      make(map[string]outlineNodeJSON, len(td.DocumentOutline)),
		conditions:      td.SemanticConditions,
		conditionValues: parentCtx.conditionValues, // condition values always come from the contract
		snapshots:       parentCtx.snapshots,
	}
	for _, raw := range td.DocumentBlocks {
		var b baseBlockJSON
		if json.Unmarshal(raw, &b) == nil {
			ctx.blockMap[b.BlockID] = b
		}
	}
	for _, node := range td.DocumentOutline {
		ctx.outlineMap[node.BlockID] = node
		if node.IsRoot {
			ctx.rootBlockIDs = node.Children
		}
	}
	return ctx
}

// ---- Placeholder resolution ----

var placeholderRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

const defaultPlaceholderText = "__________"

func resolvePlaceholders(text, blockID string, ctx *contractRenderCtx) string {
	return placeholderRe.ReplaceAllStringFunc(text, func(match string) string {
		inner := match[2 : len(match)-2]
		dot := strings.IndexByte(inner, '.')
		var condID, paramName string
		if dot >= 0 {
			condID, paramName = inner[:dot], inner[dot+1:]
		} else {
			condID = inner
		}
		for _, cv := range ctx.conditionValues {
			if cv.BlockID != blockID || cv.ConditionID != condID || cv.ParameterName != paramName {
				continue
			}
			if len(cv.ParameterValue) == 0 || string(cv.ParameterValue) == "null" {
				return defaultPlaceholderText
			}
			var s string
			if json.Unmarshal(cv.ParameterValue, &s) == nil {
				return s
			}
			return string(cv.ParameterValue)
		}
		return defaultPlaceholderText
	})
}

// ---- Block rendering ----

func renderBlocks(f *fpdf.Fpdf, ctx *contractRenderCtx, blockIDs []string, level int) {
	for _, id := range blockIDs {
		renderOneBlock(f, ctx, id, level)
	}
}

func renderOneBlock(f *fpdf.Fpdf, ctx *contractRenderCtx, blockID string, level int) {
	block, ok := ctx.blockMap[blockID]
	if !ok {
		return
	}
	switch block.Type {
	case "SECTION":
		renderSectionBlock(f, ctx, block, level)
	case "TEXT":
		renderTextBlock(f, block)
	case "CLAUSE":
		renderClauseBlock(f, ctx, block)
	case "APPROVED_TEMPLATE":
		renderApprovedTemplateBlock(f, ctx, block, level)
	case "MERGED_APPROVED_TEMPLATE":
		// Children are already merged into the outline; just recurse them.
		if node, ok := ctx.outlineMap[block.BlockID]; ok {
			renderBlocks(f, ctx, node.Children, level)
		}
	}
}

func renderSectionBlock(f *fpdf.Fpdf, ctx *contractRenderCtx, block baseBlockJSON, level int) {
	heading := block.Title
	if heading == "" {
		heading = block.Text
	}
	renderContractHeading(f, heading, level)
	if node, ok := ctx.outlineMap[block.BlockID]; ok {
		renderBlocks(f, ctx, node.Children, level+1)
	}
}

// renderContractHeading mirrors frontend styles: section1=16pt, section2=14pt, section3+=12pt.
func renderContractHeading(f *fpdf.Fpdf, text string, level int) {
	var size float64
	var topMargin float64
	var tag string
	switch level {
	case 1:
		size, topMargin, tag = sizeTitle, 6, "H1"
	case 2:
		size, topMargin, tag = 14.0, 4, "H2"
	default:
		size, topMargin, tag = sizeHeading, 2, "H3"
	}
	f.Ln(topMargin)
	f.SetFont(fontFamily, fontBold, size)
	f.SetTextColor(31, 41, 55) // #1f2937
	semanticWithTag(f, tag, func() {
		f.MultiCell(bodyWidth, lineHeight, text, "", "L", false)
	})
}

func renderTextBlock(f *fpdf.Fpdf, block baseBlockJSON) {
	if block.Text == "" {
		return
	}
	f.SetFont(fontFamily, fontRegular, sizeHeading) // 12pt to match frontend
	f.SetTextColor(55, 65, 81)                      // #374151
	semanticWithTag(f, "P", func() {
		f.MultiCell(bodyWidth, lineHeight, block.Text, "", "L", false)
	})
	f.Ln(1)
}

func renderClauseBlock(f *fpdf.Fpdf, ctx *contractRenderCtx, block baseBlockJSON) {
	if block.Text == "" {
		return
	}
	text := resolvePlaceholders(block.Text, block.BlockID, ctx)
	f.SetFont(fontFamily, fontRegular, sizeHeading)
	f.SetTextColor(55, 65, 81)
	semanticWithTag(f, "P", func() {
		f.MultiCell(bodyWidth, lineHeight, text, "", "L", false)
	})
	f.Ln(1)
}

func renderApprovedTemplateBlock(f *fpdf.Fpdf, ctx *contractRenderCtx, block baseBlockJSON, level int) {
	for _, snap := range ctx.snapshots {
		if snap.DID == block.TemplateID && snap.Version == block.Version && snap.DocumentNumber == block.DocumentNumber {
			if snap.TemplateData != nil {
				sub := buildRenderCtxFromTemplate(snap.TemplateData, ctx)
				renderBlocks(f, &sub, sub.rootBlockIDs, level)
			}
			break
		}
	}
	// Also render any child blocks attached directly to this block in the outline.
	if node, ok := ctx.outlineMap[block.BlockID]; ok {
		renderBlocks(f, ctx, node.Children, level)
	}
}

func renderCanonicalEnvelope(f *fpdf.Fpdf, envelope canonicalEnvelopeJSON) bool {
	if len(envelope.DocumentStructure.Layout) == 0 {
		return false
	}
	blockMap := make(map[string]canonicalBlockJSON, len(envelope.DocumentStructure.Blocks))
	for _, raw := range envelope.DocumentStructure.Blocks {
		var block canonicalBlockJSON
		if json.Unmarshal(raw, &block) == nil && block.ID != "" {
			blockMap[block.ID] = block
		}
	}
	layoutMap := make(map[string]canonicalLayout, len(envelope.DocumentStructure.Layout))
	rootIDs := []string{}
	for _, node := range envelope.DocumentStructure.Layout {
		layoutMap[node.ID] = node
		if node.IsRoot {
			for _, child := range node.Children.List {
				rootIDs = append(rootIDs, child.ID)
			}
		}
	}
	subTemplates := make(map[string]canonicalEnvelopeJSON, len(envelope.Metadata.SubTemplates))
	for _, subTemplate := range envelope.Metadata.SubTemplates {
		subTemplates[subTemplate.ID] = subTemplate.Template
	}
	placeholderValues := canonicalPlaceholderValues(envelope)
	var render func(ids []string, level int)
	render = func(ids []string, level int) {
		for _, id := range ids {
			block, ok := blockMap[id]
			if !ok {
				continue
			}
			switch block.Type {
			case "dcs:Section":
				renderContractHeading(f, block.Title, level)
			case "dcs:TextBlock":
				renderCanonicalText(f, block.Text)
			case "dcs:Clause":
				renderCanonicalText(f, canonicalClauseText(block.Content, canonicalBlockID(block.ID), placeholderValues))
			case "dcs:ApprovedTemplate":
				if nested, exists := subTemplates[block.TemplateDID]; exists {
					renderCanonicalEnvelope(f, nested)
				}
			}
			if node, exists := layoutMap[id]; exists {
				childIDs := make([]string, 0, len(node.Children.List))
				for _, child := range node.Children.List {
					childIDs = append(childIDs, child.ID)
				}
				render(childIDs, level+1)
			}
		}
	}
	render(rootIDs, 1)
	return len(rootIDs) > 0
}

func canonicalPlaceholderValues(envelope canonicalEnvelopeJSON) map[string]map[string]json.RawMessage {
	result := map[string]map[string]json.RawMessage{}
	for _, requirement := range envelope.ContractData {
		if requirement.Type != "dcs:DataRequirement" {
			continue
		}
		for _, field := range requirement.Fields {
			for _, value := range envelope.SemanticConditionValues {
				if value.ConditionID != requirement.ConditionID {
					continue
				}
				if value.ParameterName != field.ParameterName && value.ParameterName != field.SemanticPath {
					continue
				}
				if result[field.ID] == nil {
					result[field.ID] = map[string]json.RawMessage{}
				}
				result[field.ID][value.BlockID] = value.ParameterValue
			}
		}
	}
	return result
}

func canonicalBlockID(id string) string {
	local := id
	if index := strings.LastIndex(local, "#"); index >= 0 {
		local = local[index+1:]
	}
	local = strings.TrimPrefix(local, "block-")
	if decoded, err := url.PathUnescape(local); err == nil {
		return decoded
	}
	return local
}

func canonicalClauseText(
	raw json.RawMessage,
	blockID string,
	placeholderValues map[string]map[string]json.RawMessage,
) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var content canonicalContentList
	if json.Unmarshal(raw, &content) != nil {
		return ""
	}
	var result strings.Builder
	for _, segment := range content.List {
		if json.Unmarshal(segment, &text) == nil {
			result.WriteString(text)
			continue
		}
		var placeholder struct {
			Type    string             `json:"@type"`
			Token   string             `json:"dcs:token"`
			BindsTo canonicalReference `json:"dcs:bindsTo"`
		}
		if json.Unmarshal(segment, &placeholder) == nil && placeholder.Type == "dcs:Placeholder" {
			rawValue := placeholderValues[placeholder.BindsTo.ID][blockID]
			if len(rawValue) == 0 || string(rawValue) == "null" {
				result.WriteString(defaultPlaceholderText)
				continue
			}
			var stringValue string
			if json.Unmarshal(rawValue, &stringValue) == nil {
				result.WriteString(stringValue)
				continue
			}
			result.Write(rawValue)
		}
	}
	return result.String()
}

func renderCanonicalText(f *fpdf.Fpdf, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	f.SetFont(fontFamily, fontRegular, sizeHeading)
	f.SetTextColor(55, 65, 81)
	semanticWithTag(f, "P", func() {
		f.MultiCell(bodyWidth, lineHeight, text, "", "L", false)
	})
	f.Ln(1)
}

// renderContractData parses raw JSON-LD bytes and renders the structured contract
// document tree into f, mirroring the frontend useContractPlainTextConverter logic.
func renderContractData(f *fpdf.Fpdf, raw []byte) {
	var canonical canonicalEnvelopeJSON
	if json.Unmarshal(raw, &canonical) == nil && renderCanonicalEnvelope(f, canonical) {
		return
	}

	var data contractDataJSON
	if err := json.Unmarshal(raw, &data); err != nil {
		f.SetFont(fontFamily, fontRegular, sizeSmall)
		f.SetTextColor(30, 30, 30)
		semanticWithTag(f, "P", func() {
			f.MultiCell(bodyWidth, lineHeight, string(raw), "", "L", false)
		})
		return
	}

	ctx := buildRenderCtx(&data)
	if len(ctx.rootBlockIDs) == 0 {
		f.SetFont(fontFamily, fontRegular, sizeBody)
		f.SetTextColor(150, 150, 150)
		semanticWithTag(f, "P", func() {
			f.MultiCell(bodyWidth, lineHeight, "(No contract content)", "", "L", false)
		})
		return
	}

	renderBlocks(f, &ctx, ctx.rootBlockIDs, 1)
}
