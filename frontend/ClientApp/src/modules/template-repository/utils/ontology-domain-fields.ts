import { Parser, type Quad } from 'n3'
import type {
  DomainFieldDefinition,
  SemanticParameterType,
  SemanticValueConstraint,
  SemanticValueOption,
} from '@/modules/template-repository/models/contract-template'

/**
 * The builder's pickable domain-field vocabulary, discovered at startup from
 * every schema the Semantic Hub holds — each registered ontology's
 * dcs:DomainField individuals and each registered shapes graph's property
 * shapes — and parsed with N3. Registering a schema in the hub (a new ontology
 * version, an imported Gaia-X profile) changes what the builder offers with no
 * rebuild and no hardcoded schema name.
 */

const RDF = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#'
const RDF_TYPE = `${RDF}type`
const RDF_NIL = `${RDF}nil`
const RDFS = 'http://www.w3.org/2000/01/rdf-schema#'
const OWL_CLASS = 'http://www.w3.org/2002/07/owl#Class'
const SKOS = 'http://www.w3.org/2004/02/skos/core#'
const XSD = 'http://www.w3.org/2001/XMLSchema#'
const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'
const SH = 'http://www.w3.org/ns/shacl#'

class OntologyGraph {
  private bySubject = new Map<string, Quad[]>()

  constructor(quads: Quad[]) {
    for (const quad of quads) {
      const key = quad.subject.value
      const list = this.bySubject.get(key)
      if (list) list.push(quad)
      else this.bySubject.set(key, [quad])
    }
  }

  subjectsOfType(typeIRI: string): string[] {
    const subjects: string[] = []
    for (const [subject, quads] of this.bySubject) {
      if (quads.some((quad) => quad.predicate.value === RDF_TYPE && quad.object.value === typeIRI)) {
        subjects.push(subject)
      }
    }
    return subjects
  }

  values(subject: string, predicateIRI: string): string[] {
    return (this.bySubject.get(subject) ?? [])
      .filter((quad) => quad.predicate.value === predicateIRI)
      .map((quad) => quad.object.value)
  }

  first(subject: string, predicateIRI: string): string {
    return this.values(subject, predicateIRI)[0] ?? ''
  }

  firstNumber(subject: string, predicateIRI: string): number | undefined {
    const raw = this.first(subject, predicateIRI)
    if (raw === '') return undefined
    const parsed = Number(raw)
    return Number.isNaN(parsed) ? undefined : parsed
  }

  subjects(): string[] {
    return [...this.bySubject.keys()]
  }
}

interface SchemaListEntry {
  name: string
  kind: string
}

// Raw fetch, deliberately not the app's http client: this module loads at
// import time (top-level await below), before Pinia exists — and the http
// client's auth interceptor needs an active Pinia. The hub's resolve and
// list routes are public.
async function fetchJson<T>(route: string): Promise<T> {
  const response = await fetch(route, { headers: { Accept: 'application/json' } })
  if (!response.ok) {
    throw new Error(`Semantic Hub route ${route} is unavailable: HTTP ${response.status}`)
  }
  return (await response.json()) as T
}

/** The hub route serving a registered schema's content, by kind. */
function schemaContentRoute(kind: string, name: string): string | null {
  const encoded = encodeURIComponent(name)
  switch (kind) {
    case 'ontology':
      return `/api/semantic/ontology/${encoded}`
    case 'shapes':
      return `/api/semantic/shapes/${encoded}`
    default:
      return null
  }
}

async function loadSchemaGraph(kind: string, name: string): Promise<OntologyGraph> {
  const route = schemaContentRoute(kind, name)
  if (!route) throw new Error(`No content route for schema kind ${kind}`)
  const body = await fetchJson<{ content: string }>(route)
  return new OntologyGraph(new Parser().parse(body.content))
}

function localName(iri: string): string {
  return iri.replace(/^.*[:#/]/, '')
}

function formatOntologyLabel(value: string): string {
  const spaced = value.replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/[-_]+/g, ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function parseValueOptions(graph: OntologyGraph): ReadonlyMap<string, SemanticValueOption> {
  const options = new Map<string, SemanticValueOption>()
  for (const subject of graph.subjects()) {
    const value = graph.first(subject, `${SKOS}notation`)
    if (!value) continue
    options.set(value, {
      value,
      label: graph.first(subject, `${SKOS}prefLabel`) || undefined,
      symbol: graph.first(subject, `${DCS}valueSymbol`) || undefined,
    })
  }
  return options
}

function parseValueConstraints(graph: OntologyGraph): ReadonlyMap<string, SemanticValueConstraint> {
  const valueOptions = parseValueOptions(graph)
  const constraints = new Map<string, SemanticValueConstraint>()
  for (const subject of graph.subjectsOfType(`${DCS}ValueConstraint`)) {
    const allowedValues = graph.values(subject, `${DCS}allowedValue`)
    constraints.set(subject, {
      format: (graph.first(subject, `${DCS}format`) || undefined) as SemanticValueConstraint['format'],
      pattern: graph.first(subject, `${DCS}pattern`) || undefined,
      allowedValues,
      valueOptions: allowedValues
        .map((value) => valueOptions.get(value))
        .filter((option): option is SemanticValueOption => !!option),
      allowedValuesRef: graph.first(subject, `${DCS}allowedValuesRef`) || undefined,
      min: graph.firstNumber(subject, `${DCS}minInclusive`),
      max: graph.firstNumber(subject, `${DCS}maxInclusive`),
      description: graph.first(subject, `${RDFS}label`) || undefined,
    })
  }
  return constraints
}

function parseClassLabels(graph: OntologyGraph): ReadonlyMap<string, string> {
  const labels = new Map<string, string>()
  for (const subject of [...graph.subjectsOfType(`${RDFS}Class`), ...graph.subjectsOfType(OWL_CLASS)]) {
    labels.set(subject, graph.first(subject, `${RDFS}label`) || localName(subject))
  }
  return labels
}

/** Maps a field's rdfs:range xsd datatype to the builder's parameter type. */
function parameterTypeForRange(rangeIRI: string): SemanticParameterType {
  switch (rangeIRI) {
    case `${XSD}decimal`:
      return 'decimal'
    case `${XSD}integer`:
      return 'integer'
    case `${XSD}boolean`:
      return 'boolean'
    case `${XSD}date`:
      return 'date'
    default:
      return 'string'
  }
}

/** The dcs:parameterName a field's IRI encodes: its local name without the "field-" marker, dot-separated. */
function parameterNameFor(fieldIRI: string): string {
  return localName(fieldIRI)
    .replace(/^field-/, '')
    .replace(/-/g, '.')
}

function cloneConstraint(constraint?: SemanticValueConstraint): SemanticValueConstraint | undefined {
  if (!constraint) return undefined
  return {
    ...constraint,
    allowedValues: constraint.allowedValues ? [...constraint.allowedValues] : undefined,
    valueOptions: constraint.valueOptions ? constraint.valueOptions.map((option) => ({ ...option })) : undefined,
  }
}

function parseOntologyDomainFields(graph: OntologyGraph): DomainFieldDefinition[] {
  const constraints = parseValueConstraints(graph)
  const classLabels = parseClassLabels(graph)

  return graph
    .subjectsOfType(`${DCS}DomainField`)
    .map((subject) => {
      const range = graph.first(subject, `${RDFS}range`)
      const label = graph.first(subject, `${RDFS}label`)
      if (!range || !label) {
        throw new Error(`Ontology domain field ${subject} is incomplete.`)
      }
      const valueConstraintRef = graph.first(subject, `${DCS}hasValueConstraint`)
      const valueConstraint = valueConstraintRef ? cloneConstraint(constraints.get(valueConstraintRef)) : undefined
      const domain = graph.first(subject, `${RDFS}domain`) || undefined
      // A field is an enum when its constraint enumerates allowed values.
      const type: SemanticParameterType = valueConstraint?.allowedValues?.length ? 'enum' : parameterTypeForRange(range)
      return {
        ontologyId: subject,
        parameterName: parameterNameFor(subject),
        type,
        label,
        domain,
        domainLabel: domain ? classLabels.get(domain) : undefined,
        valueConstraint,
      }
    })
    .sort((left, right) => left.ontologyId.localeCompare(right.ontologyId))
}

/** Reads an RDF collection (rdf:first/rdf:rest) into its member IRIs/literals. */
function readRdfList(graph: OntologyGraph, head: string): string[] {
  const members: string[] = []
  let node = head
  const guard = new Set<string>()
  while (node && node !== RDF_NIL && !guard.has(node)) {
    guard.add(node)
    const first = graph.first(node, `${RDF}first`)
    if (first) members.push(first)
    node = graph.first(node, `${RDF}rest`)
  }
  return members
}

/**
 * Extracts pickable fields from a SHACL shapes graph: every property shape
 * (a node carrying sh:path) becomes a field, so any registered shapes schema
 * — the DCS clause catalog, an imported Gaia-X profile — surfaces in the
 * builder. sh:in enumerations become enum value constraints; sh:datatype maps
 * to the parameter type; the owning node shape's sh:targetClass groups it.
 */
function parseShapesDomainFields(graph: OntologyGraph): DomainFieldDefinition[] {
  const targetClassByPropShape = new Map<string, string>()
  for (const nodeShape of graph.subjectsOfType(`${SH}NodeShape`)) {
    const targetClass = graph.first(nodeShape, `${SH}targetClass`)
    if (!targetClass) continue
    for (const propShape of graph.values(nodeShape, `${SH}property`)) {
      targetClassByPropShape.set(propShape, targetClass)
    }
  }

  const fields: DomainFieldDefinition[] = []
  const seenPaths = new Set<string>()
  for (const subject of graph.subjects()) {
    const path = graph.first(subject, `${SH}path`)
    if (!path || seenPaths.has(path)) continue
    seenPaths.add(path)

    const allowedValues = readRdfList(graph, graph.first(subject, `${SH}in`)).filter(Boolean)
    const datatype = graph.first(subject, `${SH}datatype`)
    const label =
      graph.first(subject, `${SH}name`) || graph.first(subject, `${RDFS}label`) || formatOntologyLabel(localName(path))
    const domain = targetClassByPropShape.get(subject)
    const pattern = graph.first(subject, `${SH}pattern`) || undefined
    const min = graph.firstNumber(subject, `${SH}minInclusive`)
    const max = graph.firstNumber(subject, `${SH}maxInclusive`)
    const hasConstraint = allowedValues.length > 0 || pattern !== undefined || min !== undefined || max !== undefined
    fields.push({
      ontologyId: path,
      parameterName: parameterNameFor(path),
      type: allowedValues.length ? 'enum' : parameterTypeForRange(datatype),
      label,
      domain,
      domainLabel: domain ? graph.first(domain, `${RDFS}label`) || localName(domain) : undefined,
      valueConstraint: hasConstraint
        ? {
            pattern,
            min,
            max,
            allowedValues: allowedValues.length ? allowedValues : undefined,
            valueOptions: allowedValues.map((value) => ({ value })),
          }
        : undefined,
    })
  }
  return fields
}

/**
 * The builder's pickable vocabulary, discovered from the whole Semantic Hub:
 * every registered ontology contributes its dcs:DomainField individuals and
 * every registered shapes graph its property shapes. Registering a schema in
 * the hub — including an imported Gaia-X profile — makes its objects
 * immediately pickable, with no hardcoded schema name.
 */
async function loadHubDomainFields(): Promise<DomainFieldDefinition[]> {
  const inventory = await fetchJson<SchemaListEntry[]>('/api/semantic/schema/list')
  const sources = inventory.filter((entry) => entry.kind === 'ontology' || entry.kind === 'shapes')

  const perSource = await Promise.all(
    sources.map(async (entry) => {
      try {
        const graph = await loadSchemaGraph(entry.kind, entry.name)
        const fields = entry.kind === 'ontology' ? parseOntologyDomainFields(graph) : parseShapesDomainFields(graph)
        return fields.map((field) => ({ ...field, source: { name: entry.name, kind: entry.kind } }))
      } catch {
        // A single malformed schema must not blank the whole picker.
        return [] as DomainFieldDefinition[]
      }
    }),
  )

  // Dedupe by field IRI (kept from the first schema that declares it), then
  // order by source name so the picker groups predictably.
  const byId = new Map<string, DomainFieldDefinition>()
  for (const field of perSource.flat()) {
    if (!byId.has(field.ontologyId)) byId.set(field.ontologyId, field)
  }
  return [...byId.values()].sort(
    (left, right) =>
      (left.source?.name ?? '').localeCompare(right.source?.name ?? '') || left.label.localeCompare(right.label),
  )
}

export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] = await loadHubDomainFields()
