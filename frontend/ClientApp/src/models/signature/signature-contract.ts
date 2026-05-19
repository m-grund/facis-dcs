import type { ContractState } from "@/types/contract-state"

export interface SignatureContract {
  did: string
  contract_version?: number
  state: ContractState
  name?: string
  description?: string
  created_at: string
  updated_at: string
}
