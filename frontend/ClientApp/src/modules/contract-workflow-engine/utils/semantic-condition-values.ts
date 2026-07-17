import { isDcsDocumentData } from '@/models/dcs-jsonld'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { DcsContractData, DcsDataRequirement } from '@/models/dcs-jsonld'

/**
 * Boundary between the editor's (blockId, conditionId, parameterName)
 * view-model and the canonical document shape, where a submitted value is
 * carried inline on its dcs:RequirementField (dcs:parameterValue) — the same
 * field an ODRL constraint names as its odrl:leftOperand. One node holds the
 * field and its value; there is no separate semanticConditionValues array.
 */

/** The document's declared requirements, including sub-template snapshots'. */
export function collectDeclaredRequirements(cd: Partial<DcsContractData>): DcsDataRequirement[] {
  const requirements = [...(cd['dcs:contractData'] ?? [])]
  for (const snapshot of cd['dcs:metadata']?.['dcs:subTemplates'] ?? []) {
    const template = snapshot['dcs:template']
    if (isDcsDocumentData(template)) {
      requirements.push(...(template['dcs:contractData'] ?? []))
    }
  }
  return requirements
}

/**
 * Writes each submitted value inline onto the dcs:RequirementField it targets
 * (matched by conditionId + parameterName), returning new requirement objects.
 * A field with no submitted value carries no dcs:parameterValue.
 */
export function applyInlineSemanticValues(
  requirements: DcsDataRequirement[],
  values: SemanticConditionValue[],
): DcsDataRequirement[] {
  const byField = new Map<string, SemanticConditionValue>()
  for (const value of values) {
    byField.set(fieldKey(value.conditionId, value.parameterName), value)
  }
  return requirements.map((requirement) => ({
    ...requirement,
    'dcs:fields': requirement['dcs:fields'].map((field) => {
      const value = byField.get(fieldKey(requirement['dcs:conditionId'], field['dcs:parameterName']))
      const { 'dcs:parameterValue': _value, 'dcs:blockId': _block, ...rest } = field
      if (value?.parameterValue === undefined) {
        return rest
      }
      return {
        ...rest,
        'dcs:parameterValue': value.parameterValue,
        ...(value.blockId ? { 'dcs:blockId': value.blockId } : {}),
      }
    }),
  }))
}

/**
 * Applies submitted values inline to each sub-template snapshot's own
 * requirement fields, returning new snapshots — a value targeting a composed
 * sub-template's field is carried on that field, wherever it is declared.
 */
export function applyInlineSemanticValuesToSnapshots(
  snapshots: SubTemplateSnapshot[],
  values: SemanticConditionValue[],
): SubTemplateSnapshot[] {
  return snapshots.map((snapshot) => {
    const template = snapshot.template_data
    if (!isDcsDocumentData(template)) return snapshot
    return {
      ...snapshot,
      template_data: {
        ...template,
        'dcs:contractData': applyInlineSemanticValues(template['dcs:contractData'] ?? [], values),
      },
    }
  })
}

/** The editor view-model reconstructed from the fields' inline values. */
export function fromDocumentSemanticValues(requirements: DcsDataRequirement[]): SemanticConditionValue[] {
  const values: SemanticConditionValue[] = []
  for (const requirement of requirements) {
    for (const field of requirement['dcs:fields']) {
      if (field['dcs:parameterValue'] === undefined) continue
      values.push({
        blockId: field['dcs:blockId'] ?? '',
        conditionId: requirement['dcs:conditionId'],
        parameterName: field['dcs:parameterName'],
        parameterValue: field['dcs:parameterValue'],
      })
    }
  }
  return values
}

function fieldKey(conditionId: string, parameterName: string): string {
  return `${conditionId}::${parameterName}`
}
