import { ONTOLOGY_DOMAIN_FIELDS } from './ontology-domain-fields'
import type { SemanticConditionParameter } from '@/modules/template-repository/models/contract-template'

const ontologyLabelByPath = new Map(ONTOLOGY_DOMAIN_FIELDS.map((field) => [field.semanticPath, field.label]))

export function semanticParameterLabel(parameter: SemanticConditionParameter): string {
  return parameter.uiMetadata?.label ?? ontologyLabelByPath.get(parameter.semanticPath) ?? parameter.parameterName
}
