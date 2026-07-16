import { isDcsDocumentData } from '@/models/dcs-jsonld'
import type { SemanticConditionValue } from '@/models/contract-data'
import type { DcsContractData, DcsDataRequirement, DcsSemanticConditionValue } from '@/models/dcs-jsonld'

/**
 * Boundary between the editor's (blockId, conditionId, parameterName)
 * view-model and the canonical document shape, where a submitted value
 * references its dcs:RequirementField by IRI (dcs:forField) — the same IRI
 * the ODRL constraint names as its odrl:leftOperand.
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

export function toDocumentSemanticValues(
  values: SemanticConditionValue[],
  requirements: DcsDataRequirement[],
): DcsSemanticConditionValue[] {
  return values.map((value) => {
    const requirement = requirements.find((r) => r['dcs:conditionId'] === value.conditionId)
    const field = requirement?.['dcs:fields'].find((f) => f['dcs:parameterName'] === value.parameterName)
    if (!field) {
      throw new Error(
        `No requirement field declared for condition "${value.conditionId}" parameter "${value.parameterName}".`,
      )
    }
    return {
      forField: field['@id'],
      ...(value.blockId ? { blockId: value.blockId } : {}),
      ...(value.parameterValue !== undefined ? { parameterValue: value.parameterValue } : {}),
    }
  })
}

export function fromDocumentSemanticValues(
  values: DcsSemanticConditionValue[],
  requirements: DcsDataRequirement[],
): SemanticConditionValue[] {
  const fieldIndex = new Map<string, { conditionId: string; parameterName: string }>()
  for (const requirement of requirements) {
    for (const field of requirement['dcs:fields']) {
      fieldIndex.set(field['@id'], {
        conditionId: requirement['dcs:conditionId'],
        parameterName: field['dcs:parameterName'],
      })
    }
  }
  return values.map((value) => {
    const field = fieldIndex.get(value.forField)
    if (!field) {
      throw new Error(`Submitted value references undeclared requirement field "${value.forField}".`)
    }
    return {
      blockId: value.blockId ?? '',
      conditionId: field.conditionId,
      parameterName: field.parameterName,
      ...(value.parameterValue !== undefined ? { parameterValue: value.parameterValue } : {}),
    }
  })
}
