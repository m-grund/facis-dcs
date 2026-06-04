import type { ContractState } from '@/types/contract-state'

export interface SignatureContract {
  did: string
  contract_version: number
  state: ContractState
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  start_date?: string
  exp_date?: string
  exp_policy?: string
  exp_notice_period?: string
  responsible?: unknown
}
