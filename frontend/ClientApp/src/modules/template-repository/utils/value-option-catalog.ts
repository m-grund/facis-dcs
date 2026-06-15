import type { SemanticValueConstraint } from '@/modules/template-repository/models/contract-template'
import { resolveAllowedValues } from '@template-repository/utils/value-constraint-catalog'

export interface ValueOption {
  value: string
  label: string
}

export function resolveValueOptions(constraint?: SemanticValueConstraint): readonly ValueOption[] {
  if (!constraint) return []
  return resolveAllowedValues(constraint).map((value) => ({ value, label: formatValueLabel(value) }))
}

export function isTokenValueConstraint(constraint?: SemanticValueConstraint): boolean {
  if (!constraint) return false
  return !!constraint.pattern || !!constraint.format || !!constraint.allowedValuesRef
}

export function formatValueOption(value: unknown, options: readonly ValueOption[]): string {
  const raw = String(value)
  const option = options.find((item) => item.value === raw)
  return option ? `${option.label} (${option.value})` : raw
}

function formatValueLabel(value: string): string {
  return value.replace(/[-_]+/g, ' ').replace(/\b\w/g, (char) => char.toUpperCase())
}
