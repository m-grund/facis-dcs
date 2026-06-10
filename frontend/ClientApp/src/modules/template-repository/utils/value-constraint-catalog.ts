import type { SemanticValueConstraint } from '@/modules/template-repository/models/contract-template'
import { ONTOLOGY_DOMAIN_FIELDS } from '@/modules/template-repository/utils/ontology-domain-fields'

export function resolveAllowedValues(constraint?: SemanticValueConstraint): readonly string[] {
  if (!constraint) return []
  if (constraint.allowedValues?.length) return constraint.allowedValues

  const ref = normalizeAllowedValuesRef(constraint.allowedValuesRef)
  if (!ref) return []

  return (
    ONTOLOGY_DOMAIN_FIELDS.find((field) => {
      const fieldConstraint = field.valueConstraint
      return (
        normalizeAllowedValuesRef(fieldConstraint?.allowedValuesRef) === ref && !!fieldConstraint?.allowedValues?.length
      )
    })?.valueConstraint?.allowedValues ?? []
  )
}

function normalizeAllowedValuesRef(value?: string) {
  return value?.trim().replace(/\s+/g, ' ').toLowerCase() ?? ''
}
