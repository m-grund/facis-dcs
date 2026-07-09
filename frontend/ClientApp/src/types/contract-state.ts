export type ContractState = (typeof ContractState)[keyof typeof ContractState]

// Kept in sync with backend/internal/contractworkflowengine/datatype/contractstate/contractstate.go.
// The four new states (offered/withdrawn/active/revoked) are first-class
// contract-formation/post-signing states added by Workstream C4
// (contract-state-machine-refactor) around the pre-existing workflow states.
export const ContractState = {
  draft: 'DRAFT',
  offered: 'OFFERED',
  rejected: 'REJECTED',
  withdrawn: 'WITHDRAWN',
  negotiation: 'NEGOTIATION',
  submitted: 'SUBMITTED',
  reviewed: 'REVIEWED',
  approved: 'APPROVED',
  signed: 'SIGNED',
  active: 'ACTIVE',
  revoked: 'REVOKED',
  terminated: 'TERMINATED',
  expired: 'EXPIRED',
} as const

export const contractStates: ContractState[] = Object.values(ContractState)
