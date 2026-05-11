import type { ContractState } from '@/types/contract-state'
import type { ContractNegotiation } from './contract-negotiation'
import type { ContractData } from '../contract-data'

export const ExpirationPolicy = {
  renewal: "RENEWAL",
  archiving: "ARCHIVING",
  termination: "TERMINATION"
} as const;

export type ExpirationPolicy = typeof ExpirationPolicy[keyof typeof ExpirationPolicy];

export interface Contract {
  did: string
  contract_version?: number
  state: ContractState
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  start_date?: string
  exp_date?: string
  exp_notice_period?: number
  exp_policy?: ExpirationPolicy
  contract_data?: ContractData
  negotiations?: ContractNegotiation[]
}

export type ContractChangeRequest = Pick<Contract, 'name' | 'description' | 'start_date' | 'exp_date' | 'exp_notice_period' | 'exp_policy'> & { contract_data?: Partial<Contract['contract_data']> }
