package semantichub

import (
	"strconv"
	"strings"

	"github.com/tggo/goRDFlib/shacl"
)

// ClauseCatalogEntry is one typed clause NodeShape, pre-digested from SHACL
// for form generation (GET /semantic/clauses, Phase 3, ADR-10).
type ClauseCatalogEntry struct {
	Type       string
	Label      string
	Properties []ClauseCatalogPropertyEntry
}

// ClauseCatalogPropertyEntry is one sh:property on a clause NodeShape.
type ClauseCatalogPropertyEntry struct {
	Path         string
	Datatype     string
	In           []string
	MinCount     *int
	MaxCount     *int
	MinInclusive *float64
	MaxInclusive *float64
	Pattern      string
}

// ParseClauseCatalog reads the clause-catalog SHACL Turtle and digests every
// sh:NodeShape with a sh:targetClass into a form-schema entry: the same
// shapes graph validateAgainstHubShapes concatenates into contract
// validation (ADR-9), so the palette a template author sees and what a
// submitted clause is actually checked against never drift apart.
func ParseClauseCatalog(shapesTTL string) ([]ClauseCatalogEntry, error) {
	g, err := shacl.LoadTurtleString(shapesTTL, "urn:dcs:hub:clause-catalog")
	if err != nil {
		return nil, err
	}

	nodeShapeType := shacl.IRI(shacl.SH + "NodeShape")
	rdfTypePred := shacl.IRI(shacl.RDFType)
	targetClassPred := shacl.IRI(shacl.SH + "targetClass")
	labelPred := shacl.IRI(shacl.RDFS + "label")
	propertyPred := shacl.IRI(shacl.SH + "property")

	shapeSubjects := g.Subjects(rdfTypePred, nodeShapeType)
	entries := make([]ClauseCatalogEntry, 0, len(shapeSubjects))
	for _, shape := range shapeSubjects {
		targetClasses := g.Objects(shape, targetClassPred)
		if len(targetClasses) == 0 {
			continue
		}
		targetClass := targetClasses[0]

		label := targetClass.Value()
		if labels := g.Objects(shape, labelPred); len(labels) > 0 {
			label = labels[0].Value()
		} else {
			label = localName(targetClass.Value())
		}

		properties := make([]ClauseCatalogPropertyEntry, 0)
		for _, propNode := range g.Objects(shape, propertyPred) {
			properties = append(properties, parseClauseProperty(g, propNode))
		}

		entries = append(entries, ClauseCatalogEntry{
			Type:       compactClauseTerm(targetClass.Value()),
			Label:      label,
			Properties: properties,
		})
	}
	return entries, nil
}

func parseClauseProperty(g *shacl.Graph, propNode shacl.Term) ClauseCatalogPropertyEntry {
	pathPred := shacl.IRI(shacl.SH + "path")
	datatypePred := shacl.IRI(shacl.SH + "datatype")
	inPred := shacl.IRI(shacl.SH + "in")
	minCountPred := shacl.IRI(shacl.SH + "minCount")
	maxCountPred := shacl.IRI(shacl.SH + "maxCount")
	minInclusivePred := shacl.IRI(shacl.SH + "minInclusive")
	maxInclusivePred := shacl.IRI(shacl.SH + "maxInclusive")
	patternPred := shacl.IRI(shacl.SH + "pattern")

	entry := ClauseCatalogPropertyEntry{}
	if paths := g.Objects(propNode, pathPred); len(paths) > 0 {
		entry.Path = compactClauseTerm(paths[0].Value())
	}
	if datatypes := g.Objects(propNode, datatypePred); len(datatypes) > 0 {
		entry.Datatype = localName(datatypes[0].Value())
	}
	if inLists := g.Objects(propNode, inPred); len(inLists) > 0 {
		values := make([]string, 0)
		for _, v := range g.RDFList(inLists[0]) {
			values = append(values, v.Value())
		}
		entry.In = values
	}
	if patterns := g.Objects(propNode, patternPred); len(patterns) > 0 {
		entry.Pattern = patterns[0].Value()
	}
	entry.MinCount = parseClauseInt(g, propNode, minCountPred)
	entry.MaxCount = parseClauseInt(g, propNode, maxCountPred)
	entry.MinInclusive = parseClauseFloat(g, propNode, minInclusivePred)
	entry.MaxInclusive = parseClauseFloat(g, propNode, maxInclusivePred)
	return entry
}

func parseClauseInt(g *shacl.Graph, subject, predicate shacl.Term) *int {
	objects := g.Objects(subject, predicate)
	if len(objects) == 0 {
		return nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(objects[0].Value()))
	if err != nil {
		return nil
	}
	return &value
}

func parseClauseFloat(g *shacl.Graph, subject, predicate shacl.Term) *float64 {
	objects := g.Objects(subject, predicate)
	if len(objects) == 0 {
		return nil
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(objects[0].Value()), 64)
	if err != nil {
		return nil
	}
	return &value
}

// localName extracts the fragment/last-segment local name from a full IRI.
func localName(iri string) string {
	if iri == "" {
		return ""
	}
	if i := strings.LastIndexAny(iri, "#/"); i >= 0 && i < len(iri)-1 {
		return iri[i+1:]
	}
	return iri
}

// compactClauseTerm renders a full ontology IRI as a "dcs:Term"-style
// compact name for readability in the API response (all clause-catalog
// terms live under the dcs: ontology namespace today).
func compactClauseTerm(iri string) string {
	const dcsNS = "https://w3id.org/facis/dcs/ontology/v1#"
	if strings.HasPrefix(iri, dcsNS) {
		return "dcs:" + strings.TrimPrefix(iri, dcsNS)
	}
	return iri
}
