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
 * A hub asset: a SHACL NodeShape's target class (an imported Gaia-X
 * ServiceOffering/DataResource, a registered entity type) plus the fields its
 * property shapes describe. Declaring an asset makes it an ODRL target; its
 * properties become the fields constrained or filled for that asset.
 */
export interface HubAsset {
  /** The sh:targetClass IRI — the asset type's identity. */
  id: string
  label: string
  properties: DomainFieldDefinition[]
  source?: { name: string; kind: string }
}

/** Builds a domain field from a SHACL property shape (a node carrying sh:path). */
function buildPropertyField(graph: OntologyGraph, propShape: string, path: string): DomainFieldDefinition {
  const allowedValues = readRdfList(graph, graph.first(propShape, `${SH}in`)).filter(Boolean)
  const datatype = graph.first(propShape, `${SH}datatype`)
  const label =
    graph.first(propShape, `${SH}name`) ||
    graph.first(propShape, `${RDFS}label`) ||
    formatOntologyLabel(localName(path))
  const pattern = graph.first(propShape, `${SH}pattern`) || undefined
  const min = graph.firstNumber(propShape, `${SH}minInclusive`)
  const max = graph.firstNumber(propShape, `${SH}maxInclusive`)
  const hasConstraint = allowedValues.length > 0 || pattern !== undefined || min !== undefined || max !== undefined
  // A property with no sh:datatype and no enum is object-valued (sh:class /
  // sh:node) — filled with a reference/identifier, carried as a string.
  const type: SemanticParameterType = allowedValues.length ? 'enum' : parameterTypeForRange(datatype)
  return {
    ontologyId: path,
    parameterName: parameterNameFor(path),
    type,
    label,
    valueConstraint: hasConstraint
      ? {
          pattern,
          min,
          max,
          allowedValues: allowedValues.length ? allowedValues : undefined,
          valueOptions: allowedValues.map((value) => ({ value })),
        }
      : undefined,
  }
}

/**
 * Extracts assets from a SHACL shapes graph: every NodeShape with a
 * sh:targetClass becomes a pickable asset, its property shapes its fields.
 */
function parseShapesAssets(graph: OntologyGraph): HubAsset[] {
  const assets: HubAsset[] = []
  const seenClass = new Set<string>()
  for (const nodeShape of graph.subjectsOfType(`${SH}NodeShape`)) {
    const targetClass = graph.first(nodeShape, `${SH}targetClass`)
    if (!targetClass || seenClass.has(targetClass)) continue
    seenClass.add(targetClass)
    const label =
      graph.first(nodeShape, `${RDFS}label`) ||
      graph.first(targetClass, `${RDFS}label`) ||
      formatOntologyLabel(localName(targetClass))
    const properties: DomainFieldDefinition[] = []
    const seenPath = new Set<string>()
    for (const propShape of graph.values(nodeShape, `${SH}property`)) {
      const path = graph.first(propShape, `${SH}path`)
      if (!path || seenPath.has(path)) continue
      seenPath.add(path)
      properties.push(buildPropertyField(graph, propShape, path))
    }
    assets.push({ id: targetClass, label, properties })
  }
  return assets
}

/**
 * The builder's pickable vocabulary, discovered from the whole Semantic Hub:
 * each registered ontology contributes its dcs:DomainField individuals (flat
 * data fields); each registered shapes graph contributes its NodeShapes as
 * assets. Registering a schema in the hub — including an imported Gaia-X
 * profile — makes its objects pickable, with no hardcoded schema name.
 */
async function loadHub(): Promise<{ fields: DomainFieldDefinition[]; assets: HubAsset[] }> {
  const inventory = await fetchJson<SchemaListEntry[]>('/api/semantic/schema/list')
  const perSource = await Promise.all(
    inventory
      .filter((entry) => entry.kind === 'ontology' || entry.kind === 'shapes')
      .map(async (entry) => {
        const source = { name: entry.name, kind: entry.kind }
        try {
          const graph = await loadSchemaGraph(entry.kind, entry.name)
          if (entry.kind === 'ontology') {
            return {
              fields: parseOntologyDomainFields(graph).map((field) => ({ ...field, source })),
              assets: [] as HubAsset[],
            }
          }
          return {
            fields: [] as DomainFieldDefinition[],
            assets: parseShapesAssets(graph).map((a) => ({ ...a, source })),
          }
        } catch {
          // A single malformed schema must not blank the whole picker.
          return { fields: [] as DomainFieldDefinition[], assets: [] as HubAsset[] }
        }
      }),
  )

  const bySource = (a?: { name: string }, b?: { name: string }) => (a?.name ?? '').localeCompare(b?.name ?? '')
  const fieldsById = new Map<string, DomainFieldDefinition>()
  for (const field of perSource.flatMap((p) => p.fields)) {
    if (!fieldsById.has(field.ontologyId)) fieldsById.set(field.ontologyId, field)
  }
  const assetsById = new Map<string, HubAsset>()
  for (const asset of perSource.flatMap((p) => p.assets)) {
    if (!assetsById.has(asset.id)) assetsById.set(asset.id, asset)
  }
  return {
    fields: [...fieldsById.values()].sort((l, r) => bySource(l.source, r.source) || l.label.localeCompare(r.label)),
    assets: [...assetsById.values()].sort((l, r) => bySource(l.source, r.source) || l.label.localeCompare(r.label)),
  }
}

const hub = await loadHub()
export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] = hub.fields
export const ONTOLOGY_ASSETS: readonly HubAsset[] = hub.assets
