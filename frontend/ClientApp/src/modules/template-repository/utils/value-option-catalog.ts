import type {
  SemanticValueConstraint,
  SemanticValueOption,
} from '@/modules/template-repository/models/contract-template'
import {
  resolveAllowedValues,
  resolveValueConstraintOptions,
} from '@template-repository/utils/value-constraint-catalog'

export type ValueOption = Required<Pick<SemanticValueOption, 'value' | 'label'>> & Pick<SemanticValueOption, 'symbol'>

export function resolveValueOptions(constraint?: SemanticValueConstraint): readonly ValueOption[] {
  if (!constraint) return []
  const optionsByValue = new Map(
    (resolveValueConstraintOptions(constraint) ?? []).map((option) => [option.value, option]),
  )
  return resolveAllowedValues(constraint).map((value) => {
    const option = optionsByValue.get(value)
    return {
      value,
      label: option?.label ?? value,
      symbol: option?.symbol,
    }
  })
}

export function isTokenValueConstraint(constraint?: SemanticValueConstraint): boolean {
  if (!constraint) return false
  return !!constraint.pattern || !!constraint.format || !!constraint.allowedValuesRef
}

export function formatValueOption(value: unknown, options: readonly ValueOption[]): string {
  const raw = String(value)
  const option = options.find((item) => item.value === raw)
  if (!option) return raw
  if (option.symbol) return `${option.symbol} ${option.value}`
  return option.label === option.value ? option.value : `${option.label} (${option.value})`
}
