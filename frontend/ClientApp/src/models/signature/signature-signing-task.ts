import type { SigningTaskState } from '@/types/signing-task-state'

export interface SignatureSigningTask {
  did: string
  contract_version: number
  state: SigningTaskState
  signer: string
  created_at: string
}
