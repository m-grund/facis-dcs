import http from '@/api/http'

/**
 * Semantic Hub clause catalog (DCS-FR-TR-03/TR-04, Phase 3, ADR-10):
 * typed clause NodeShapes pre-digested into a form-schema, plus the raw
 * SHACL Turtle they were derived from. Public endpoint (no auth), like
 * resolve_context — see backend/design/semantic_hub.go "clauses".
 */
export interface ClauseCatalogProperty {
  path: string
  datatype?: string
  in?: string[]
  min_count?: number
  max_count?: number
  min_inclusive?: number
  max_inclusive?: number
}

export interface ClauseCatalogType {
  type: string
  label: string
  properties: ClauseCatalogProperty[]
}

export interface ClauseCatalogResponse {
  version: number
  clauses: ClauseCatalogType[]
  shapes: string
}

export async function getClauseCatalog(): Promise<ClauseCatalogResponse> {
  return http.get('/semantic/clauses').then((res) => res.data)
}
