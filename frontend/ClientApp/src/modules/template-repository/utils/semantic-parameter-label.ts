import type { SemanticConditionParameter, SemanticParameterType } from '@/modules/template-repository/models/contract-template'
import { ONTOLOGY_DOMAIN_FIELDS } from './ontology-domain-fields'

const ontologyLabelByPath = new Map(ONTOLOGY_DOMAIN_FIELDS.map((field) => [field.semanticPath, field.label]))
const parameterTypeLabels: Record<SemanticParameterType, string> = {
  string: 'Text',
  decimal: 'Decimal number',
  integer: 'Whole number',
  boolean: 'Yes/No',
  date: 'Date',
  enum: 'Selection',
}

export function semanticParameterLabel(parameter: SemanticConditionParameter): string {
  return parameter.uiMetadata?.label ?? ontologyLabelByPath.get(parameter.semanticPath) ?? parameter.parameterName
}

export function semanticParameterTypeLabel(type: SemanticParameterType): string {
  return parameterTypeLabels[type] ?? type
}
