/** Assignee IDs must match JWT sub. */

export interface ParticipantSelection {
  reviewers: string[]
  approvers: string[]
  negotiators: string[]
}

/** True if DID already exists in the same role list. */
export function isDuplicateInList(did: string, list: string[]): boolean {
  const normalized = did.trim()
  return list.some((entry) => entry === normalized)
}

/** Trim list entries, merge a pending draft if non-empty, drop blanks. */
export function mergeDraftIntoList(list: string[], draft: string): string[] {
  const result = list.map((entry) => entry.trim()).filter(Boolean)
  const pending = draft.trim()
  if (pending && !isDuplicateInList(pending, result)) {
    result.push(pending)
  }
  return result
}
