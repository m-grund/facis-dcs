import type { DcsBlock } from '@/models/dcs-jsonld'
import type { MergedApprovedTemplateBlock } from '@template-repository/store/dcsDraftStore'

/**
 * One block row in the editor list:
 * flattened outline item + block data + toolbar capabilities.
 */
export interface EnrichedBlockItem {
  /** Full JSON-LD @id IRI. */
  blockId: string
  block?: DcsBlock | MergedApprovedTemplateBlock
  siblingIndex: number
  siblingCount: number
  /** Full JSON-LD @id IRI. */
  parentBlockId: string
  depthLevel: number
  prevSiblingBlockId?: string
  nextSiblingBlockId?: string
  canOutdent: boolean
  canIndent: boolean
  outdentGrandparentBlockId: string
  outdentInsertIndex: number
  indentParentBlockId: string
  indentInsertIndex: number
  /**
   * Some approved blocks are merged into the main document for editing
   * without conflicting with the original approved template.
   */
  mergedApprovedBlock?: MergedApprovedTemplateBlock
}
