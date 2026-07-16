import { Parser, type Quad } from 'n3'
import http from '@/api/http'
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
const DCS = 'https://w3id.org/facis/dcs/ontology/v1#'
const DCST = 'https://w3id.org/facis/dcs/taxonomy/v1#'

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
  const response = await http.get('/semantic/ontology/facis-sla')
  const content: string = response.data.content
  return new OntologyGraph(new Parser().parse(content))
}

function localName(iri: string): string {
  return iri.replace(/^.*[:#/]/, '')
}

/** Prefixed form for document emission, matching the hub context's prefixes. */
function compactResource(iri: string): string {
  if (iri.startsWith(DCST)) return `dcst:${iri.slice(DCST.length)}`
  if (iri.startsWith(DCS)) return `dcs:${iri.slice(DCS.length)}`
  return iri
}

function formatOntologyLabel(value: string): string {
  const spaced = value.replace(/([a-z0-9])([A-Z])/g, '$1 $2').replace(/[-_]+/g, ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function isPrimitiveCodeType(name: string): boolean {
  return name.endsWith('Code')
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
    const name = localName(subject)
    if (name) labels.set(name, graph.first(subject, `${RDFS}label`) || name)
  }
  return labels
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
      const semanticPath = graph.first(subject, `${DCS}semanticPath`)
      const schemaRef = graph.first(subject, `${DCS}schemaRef`)
      const type = graph.first(subject, `${DCS}parameterType`) as SemanticParameterType
      const label = graph.first(subject, `${RDFS}label`)
      if (!semanticPath || !schemaRef || !type || !label) {
        throw new Error(`Ontology domain field ${subject} is incomplete.`)
      }
      const valueConstraintRef = graph.first(subject, `${DCS}hasValueConstraint`)
      const statementTypeIRI = graph.first(subject, `${DCS}statementType`)
      const statementType = statementTypeIRI ? compactResource(statementTypeIRI) : undefined
      return {
        ontologyId: subject,
        semanticPath,
        schemaRef,
        type,
        label,
        statementType,
        statementTypeLabel: statementTypeIRI ? classLabels.get(localName(statementTypeIRI)) : undefined,
        valueConstraint: valueConstraintRef ? cloneConstraint(constraints.get(valueConstraintRef)) : undefined,
      }
    })
    .sort((left, right) => left.semanticPath.localeCompare(right.semanticPath))
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
    .map((value) => ({ value: value as SemanticEntityRole, label: formatOntologyLabel(value) }))
    .sort((left, right) => left.label.localeCompare(right.label))
}

function parseOntologyEntityTypes(graph: OntologyGraph): OntologyEntityTypeOption[] {
  const statementTypeNames = new Set(
    graph
      .subjectsOfType(`${DCS}DomainField`)
      .map((subject) => localName(graph.first(subject, `${DCS}statementType`)))
      .filter(Boolean),
  )
  const documentEntityTypeNames = new Set(
    graph
      .subjects()
      .filter((subject) => graph.first(subject, `${DCS}documentProperty`) !== '')
      .map((subject) => localName(graph.first(subject, `${RDFS}range`)))
      .filter((name) => name && statementTypeNames.has(name)),
  )
  const entityTypeNames = new Set(
    documentEntityTypeNames.size
      ? documentEntityTypeNames
      : [...statementTypeNames].filter((name) => !isPrimitiveCodeType(name)),
  )
  const roleRequired = parseOntologyEntityRoles(graph).length > 0

  return graph
    .subjectsOfType(`${RDFS}Class`)
    .filter((subject) => entityTypeNames.has(localName(subject)))
    .map((subject) => ({
      value: localName(subject) as SemanticEntityType,
      label: graph.first(subject, `${RDFS}label`) || localName(subject),
      roleRequired,
    }))
    .sort((left, right) => left.label.localeCompare(right.label))
}

const graph = await loadOntologyGraph()

export const ONTOLOGY_DOMAIN_FIELDS: readonly DomainFieldDefinition[] = parseOntologyDomainFields(graph)
export const ONTOLOGY_ENTITY_TYPES: readonly OntologyEntityTypeOption[] = parseOntologyEntityTypes(graph)
export const ONTOLOGY_ENTITY_ROLES: readonly OntologySelectOption<SemanticEntityRole>[] =
  parseOntologyEntityRoles(graph)
