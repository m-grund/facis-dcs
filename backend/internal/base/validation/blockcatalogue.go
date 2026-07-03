// Package validation normalizes and validates template/contract JSON-LD
// data against the ontology's block catalogue before persistence (used by
// both templaterepository and contractworkflowengine command handlers).
package validation

import "fmt"

var blockCatalogue = mustLoadOntologyBlockCatalogue()

func mustLoadOntologyBlockCatalogue() map[string]blockDefinition {
	catalogue := loadOntologyBlockCatalogue()
	if len(catalogue) == 0 {
		panic("ontology does not define dcs:BlockCatalogueEntry entries")
	}
	return catalogue
}

func loadOntologyBlockCatalogue() map[string]blockDefinition {
	catalogue := map[string]blockDefinition{}
	for _, statement := range ontologyStatementsFromConfiguredFile() {
		if !ontologyStatementHasType(statement, "dcs:BlockCatalogueEntry") {
			continue
		}
		catalogueID := ontologyString(statement, "dcs:blockCatalogueId")
		schemaRef := ontologyString(statement, "dcs:schemaRef")
		semanticPath := ontologyString(statement, "dcs:semanticPath")
		if catalogueID == "" || schemaRef == "" || semanticPath == "" {
			continue
		}
		catalogue[catalogueID] = blockDefinition{
			SchemaRef:    schemaRef,
			SemanticPath: semanticPath,
		}
	}
	return catalogue
}

func normalizeBlockCatalogue(block map[string]any) {
	if _, ok := block["blockCatalogueId"].(string); ok {
		return
	}
	blockType, _ := block["type"].(string)
	catalogueID := defaultBlockCatalogueID(blockType)
	if catalogueID == "" {
		return
	}
	applyBlockDefinition(block, catalogueID)
}

func defaultBlockCatalogueID(blockType string) string {
	for _, statement := range ontologyStatementsFromConfiguredFile() {
		if !ontologyStatementHasType(statement, "dcs:BlockCatalogueEntry") {
			continue
		}
		if ontologyString(statement, "dcs:blockType") == blockType {
			return ontologyString(statement, "dcs:blockCatalogueId")
		}
	}
	return ""
}

func applyBlockDefinition(block map[string]any, catalogueID string) {
	def, ok := blockCatalogue[catalogueID]
	if !ok {
		return
	}
	block["blockCatalogueId"] = catalogueID
	block["schemaRef"] = def.SchemaRef
	block["semanticPath"] = def.SemanticPath
}

func validateBlockCatalogue(block map[string]any) error {
	catalogueID, _ := block["blockCatalogueId"].(string)
	def, ok := blockCatalogue[catalogueID]
	if !ok {
		return fmt.Errorf("unknown blockCatalogueId %q", catalogueID)
	}
	schemaRef, _ := block["schemaRef"].(string)
	semanticPath, _ := block["semanticPath"].(string)
	if schemaRef != def.SchemaRef {
		return fmt.Errorf("schemaRef must be %q for blockCatalogueId %q", def.SchemaRef, catalogueID)
	}
	if semanticPath != def.SemanticPath {
		return fmt.Errorf("semanticPath must be %q for blockCatalogueId %q", def.SemanticPath, catalogueID)
	}
	return nil
}
