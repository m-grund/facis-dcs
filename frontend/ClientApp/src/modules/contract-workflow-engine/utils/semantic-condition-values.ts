import type { SemanticConditionValue } from '@/models/contract-data'
import type { DcsContractData, DcsPlaceholder } from '@/models/dcs-jsonld'

/**
 * Boundary between the editor's (blockId, conditionId, parameterName)
 * view-model and the canonical document shape, where a submitted value is
 * carried inline on its dcs:Placeholder (dcs:value) — the same node an ODRL
 * constraint names as its odrl:leftOperand. conditionId is the placeholder @id,
 * so a value is matched to its placeholder by @id.
 */

/** The document's declared placeholders. */
export function collectDeclaredRequirements(cd: Partial<DcsContractData>): DcsPlaceholder[] {
  return [...(cd['dcs:contractData'] ?? [])]
}

/**
 * Writes each submitted value inline onto the dcs:Placeholder it targets
 * (matched by @id), returning new placeholder objects. A placeholder with no
 * submitted value carries no dcs:value.
 */
export function applyInlineSemanticValues(
  placeholders: DcsPlaceholder[],
  values: SemanticConditionValue[],
): DcsPlaceholder[] {
  const byId = new Map<string, SemanticConditionValue>()
  for (const value of values) {
    byId.set(value.conditionId, value)
  }
  return placeholders.map((placeholder) => {
    const value = byId.get(placeholder['@id'])
    const { 'dcs:value': _value, ...rest } = placeholder
    if (value?.parameterValue === undefined) {
      return rest
    }
    return { ...rest, 'dcs:value': value.parameterValue }
  })
}

/** The editor view-model reconstructed from the placeholders' inline values. */
export function fromDocumentSemanticValues(placeholders: DcsPlaceholder[]): SemanticConditionValue[] {
  const values: SemanticConditionValue[] = []
  for (const placeholder of placeholders) {
    if (placeholder['dcs:value'] === undefined) continue
    values.push({
      blockId: '',
      conditionId: placeholder['@id'],
      parameterName: placeholder['dcs:label'],
      parameterValue: placeholder['dcs:value'],
    })
  }
  return values
}
