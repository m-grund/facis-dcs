import { Parser, type Quad } from 'n3'
import type {
  DomainFieldDefinition,
  SemanticEntityRole,
  SemanticEntityType,
  SemanticParameterType,
  SemanticValueConstraint,
  SemanticValueOption,
} from '@/modules/template-repository/models/contract-template'

/**
 * The SLA domain-field catalog (dcs:DomainField/dcs:ValueConstraint
 * individuals plus taxonomy value options) driving the semantic-condition
 * builder. Fetched at startup from the Semantic Hub's registered
 * "facis-sla" ontology — registering a new version in the hub changes what
 * the builder offers, no rebuild involved — and parsed with N3.
 */

export interface OntologySelectOption<TValue extends string = string> {
  value: TValue
  label: string
}

export interface OntologyEntityTypeOption extends OntologySelectOption<SemanticEntityType> {
  roleRequired: boolean
}

const RDF_TYPE = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#type'
const RDF_PROPERTY = 'http://www.w3.org/1999/02/22-rdf-syntax-ns#Property'
const RDFS = 'http://www.w3.org/2000/01/rdf-schema#'
const OWL_CLASS = 'http://www.w3.org/2002/07/owl#Class'
const SKOS = 'http://www.w3.org/2004/02/skos/core#'
const XSD = 'http://www.w3.org/2001/XMLSchema#'
const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'

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

async function loadOntologyGraph(): Promise<OntologyGraph> {
  // Raw fetch, deliberately not the app's http client: this module loads at
  // import time (top-level await below), before Pinia exists — and the http
  // client's auth interceptor needs an active Pinia. The hub's resolve
  // routes are public.
  const response = await fetch('/api/semantic/ontology/facis-sla', { headers: { Accept: 'application/json' } })
  if (!response.ok) {
    throw new Error(`Semantic Hub ontology facis-sla is unavailable: HTTP ${response.status}`)
  }
  const body = (await response.json()) as { content: string }
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
  return localName(fieldIRI).replace(/^field-/, '').replace(/-/g, '.')
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
      const type: SemanticParameterType = valueConstraint?.allowedValues?.length
        ? 'enum'
        : parameterTypeForRange(range)
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

function parseOntologyEntityRoles(graph: OntologyGraph): OntologySelectOption<SemanticEntityRole>[] {
  const constraints = parseValueConstraints(graph)
  const allowedValues = graph
    .subjectsOfType(RDF_PROPERTY)
    .filter((subject) => localName(graph.first(subject, `${RDFS}range`)).endsWith('RoleCode'))
    .map((subject) => graph.first(subject, `${DCS}hasValueConstraint`))
    .filter(Boolean)
    .flatMap((ref) => constraints.get(ref)?.allowedValues ?? [])

  return allowedValues
    .map((value) => ({ value: value, label: formatOntologyLabel(value) }))
    .sort((left, right) => left.label.localeCompare(right.label))
}

function parseOntologyEntityTypes(graph: OntologyGraph): OntologyEntityTypeOption[] {
  const domainClassIRIs = new Set(
    graph
      .subjectsOfType(`${DCS}DomainField`)
      .map((subject) => graph.first(subject, `${RDFS}domain`))
      .filter(Boolean),
  )
  // Document-level entity classes (the ranges of properties carried by the
  // document itself, e.g. dcs:party → dcs:CompanyParty) represent parties
  // and require a contract role on their requirements.
  const roleRequiredIRIs = new Set(
    graph
      .subjects()
      .filter((subject) => graph.first(subject, `${DCS}documentProperty`) !== '')
      .map((subject) => graph.first(subject, `${RDFS}range`))
      .filter((iri) => domainClassIRIs.has(iri)),
  )
  const rolesExist = parseOntologyEntityRoles(graph).length > 0

  return [...domainClassIRIs]
    .map((iri) => ({
      value: localName(iri),
      label: graph.first(iri, `${RDFS}label`) || localName(iri),
      roleRequired: rolesExist && roleRequiredIRIs.has(iri),
    }))
    .sort((left, right) => left.label.localeCompare(right.label))
}

const graph = await loadOntologyGraph()

export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] = parseOntologyDomainFields(graph)
export const ONTOLOGY_ENTITY_TYPES: readonly OntologyEntityTypeOption[] = parseOntologyEntityTypes(graph)
export const ONTOLOGY_ENTITY_ROLES: readonly OntologySelectOption<SemanticEntityRole>[] =
  parseOntologyEntityRoles(graph)
