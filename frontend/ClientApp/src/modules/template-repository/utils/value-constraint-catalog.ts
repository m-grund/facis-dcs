import { ONTOLOGY_DOMAIN_FIELDS } from '@template-repository/utils/ontology-domain-fields'
import type { SemanticValueConstraint } from '@template-repository/models/contract-template'

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

export function resolveValueConstraintOptions(
  constraint?: SemanticValueConstraint,
): SemanticValueConstraint['valueOptions'] {
  if (!constraint) return []
  if (constraint.valueOptions?.length) return constraint.valueOptions

  const ref = normalizeAllowedValuesRef(constraint.allowedValuesRef)
  if (!ref) return []

  return (
    ONTOLOGY_DOMAIN_FIELDS.find((field) => {
      const fieldConstraint = field.valueConstraint
      return (
        normalizeAllowedValuesRef(fieldConstraint?.allowedValuesRef) === ref && !!fieldConstraint?.valueOptions?.length
      )
    })?.valueConstraint?.valueOptions ?? []
  )
}

function normalizeAllowedValuesRef(value?: string) {
  return value?.trim().replace(/\s+/g, ' ').toLowerCase() ?? ''
}
