import type { SubTemplateSnapshot } from '@/models/contract-template'
import type { DcsBlock, DcsLayoutNode, DcsContractData } from '@/models/dcs-jsonld'
import type { MergedApprovedTemplateBlock } from '@template-repository/store/dcsDraftStore'
import { buildMergedChildBlockId, isSameTemplateDataRef } from '@template-repository/utils/template-data-ref'
import { getBlocksFromTemplateData, getLayoutFromTemplateData } from '@template-repository/store/dcsDraftStore'
import { isDcsDocumentData } from '@/models/dcs-jsonld'

export interface PreprocessedContractData {
  blocks: (DcsBlock | MergedApprovedTemplateBlock)[]
  layout: DcsLayoutNode[]
  contractData: DcsContractData['dcs:contractData']
  policies: DcsContractData['dcs:policies']
  semanticConditionValues: DcsContractData['semanticConditionValues']
  subTemplateSnapshots: SubTemplateSnapshot[]
  sourceTemplate?: DcsContractData['sourceTemplate']
  derivedFromTemplate?: string
}

/**
 * Preprocesses contract data (DcsContractData) for use in the contract workflow engine:
 * - Injects sub-template blocks as MergedApprovedTemplate virtual blocks into the main layout
 * - Supports composed contract templates with multiple sub-template references
 */
export function useContractDataPreprocess() {
  function preprocessContractData(cd: unknown): PreprocessedContractData | null {
    if (!isDcsDocumentData(cd)) return null

    const contractData = cd as DcsContractData
    const rawBlocks: DcsBlock[] = deepClone(contractData['dcs:documentStructure']['dcs:blocks']['@list'])
    const rawLayout: DcsLayoutNode[] = deepClone(contractData['dcs:documentStructure']['dcs:layout'])
    const subTemplateSnapshots = deserializeSubTemplates(contractData)
    const blocks: (DcsBlock | MergedApprovedTemplateBlock)[] = [...rawBlocks]
    const layout: DcsLayoutNode[] = [...rawLayout]

    // Find approved template blocks and inject sub-template content
    const approvedTemplateBlocks = rawBlocks.filter((b) => b['@type'] === 'dcs:ApprovedTemplate')
    for (const approvedBlock of approvedTemplateBlocks) {
      const subTemplateSnapshot = findSnapshotByApprovedBlock(subTemplateSnapshots, approvedBlock)
      if (!subTemplateSnapshot) continue

      const subBlocks = getBlocksFromTemplateData(subTemplateSnapshot.template_data)
      const subLayout = getLayoutFromTemplateData(subTemplateSnapshot.template_data)
      const subRoot = subLayout.find((n) => n['dcs:isRoot'])
      const approvedLayoutNode = layout.find((n) => n['@id'] === approvedBlock['@id'])
      if (!subRoot || !approvedLayoutNode) continue

      const blockIdMap = buildBlockIdMap(subLayout, approvedBlock['@id'])
      const subRootChildIds = subRoot['dcs:children']['@list'].map((r) => r['@id'])
      const mergedChildIds = subRootChildIds.map((id) => getMappedId(blockIdMap, id))

      // Replace approved block with merged block in blocks array
      const mergedBlock: MergedApprovedTemplateBlock = {
        '@type': 'dcs:MergedApprovedTemplate',
        '@id': approvedBlock['@id'],
        'dcs:templateDid': approvedBlock['dcs:templateDid'],
        'dcs:version': approvedBlock['dcs:version'],
        'dcs:documentNumber': approvedBlock['dcs:documentNumber'] ?? '',
      }
      const idx = blocks.findIndex((b) => b['@id'] === approvedBlock['@id'])
      if (idx >= 0) blocks[idx] = mergedBlock

      // Add remapped sub-blocks (non-root)
      for (const subBlock of subBlocks) {
        const newId = getMappedId(blockIdMap, subBlock['@id'])
        blocks.push({ ...deepClone(subBlock), '@id': newId })
      }

      // Add remapped sub-layout nodes (non-root)
      for (const node of subLayout) {
        if (node['dcs:isRoot']) continue
        const newId = getMappedId(blockIdMap, node['@id'])
        layout.push({
          '@id': newId,
          'dcs:children': {
            '@list': node['dcs:children']['@list'].map((r) => ({ '@id': getMappedId(blockIdMap, r['@id']) })),
          },
        })
      }

      // Prepend merged children to the approved block's layout node
      const existingChildren = approvedLayoutNode['dcs:children']['@list'].map((r) => r['@id'])
      approvedLayoutNode['dcs:children'] = {
        '@list': [...mergedChildIds, ...existingChildren].map((id) => ({ '@id': id })),
      }
    }

    return {
      blocks,
      layout,
      contractData: contractData['dcs:contractData'],
      policies: contractData['dcs:policies'],
      semanticConditionValues: contractData.semanticConditionValues ?? [],
      subTemplateSnapshots,
      sourceTemplate: contractData.sourceTemplate,
      derivedFromTemplate: contractData.derivedFromTemplate,
    }
  }

  return { preprocessContractData }
}

function deserializeSubTemplates(cd: DcsContractData): SubTemplateSnapshot[] {
  const raw = cd['dcs:metadata']?.['dcs:subTemplates'] ?? []
  return raw.map((s) => ({
    did: s['@id'],
    version: s['dcs:version'],
    document_number: s['dcs:documentNumber'],
    name: s['dcs:name'],
    description: s['dcs:description'],
    template_data: s['dcs:template'],
  }))
}

function findSnapshotByApprovedBlock(
  snapshots: SubTemplateSnapshot[],
  approvedBlock: DcsBlock,
): SubTemplateSnapshot | undefined {
  const b = approvedBlock as import('@/models/dcs-jsonld').DcsApprovedTemplate
  return snapshots.find((snapshot) =>
    isSameTemplateDataRef(
      { templateId: snapshot.did, version: snapshot.version, document_number: snapshot.document_number },
      { templateId: b['dcs:templateDid'], version: b['dcs:version'], document_number: b['dcs:documentNumber'] },
    ),
  )
}

function buildBlockIdMap(subLayout: DcsLayoutNode[], approvedBlockId: string): Map<string, string> {
  const map = new Map<string, string>()
  for (const node of subLayout) {
    if (!node['dcs:isRoot'] && !map.has(node['@id'])) {
      map.set(node['@id'], buildMergedChildBlockId(approvedBlockId, node['@id']))
    }
    for (const ref of node['dcs:children']['@list']) {
      if (!map.has(ref['@id'])) {
        map.set(ref['@id'], buildMergedChildBlockId(approvedBlockId, ref['@id']))
      }
    }
  }
  return map
}

function getMappedId(map: Map<string, string>, id: string): string {
  return map.get(id) ?? id
}

function deepClone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}
