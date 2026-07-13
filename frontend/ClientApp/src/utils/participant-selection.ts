/**
 * Assignee IDs are raw participant identifiers (e.g. a did:web peer DID for
 * cross-instance federation, or a local user/org identifier) recorded as-is
 * on the contract's `responsible` reviewers/approvers/negotiators lists.
 * There is no JWT-`sub` binding/validation here or on the backend — any
 * syntactically accepted identifier can be assigned, including a raw peer
 * DID that never authenticates via this instance's own JWTs (see the
 * two-instance peer-trust pack, features/17_peer_trust).
 */

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
