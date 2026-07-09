import type { ContractData } from '../contract-data'
import type { ContractNegotiation } from './contract-negotiation'
import type { ContractResponsible } from './contract-responsible'
import type { ContractState } from '@/types/contract-state'

export const ExpirationPolicy = {
  renewal: 'RENEWAL',
  archiving: 'ARCHIVING',
  termination: 'TERMINATION',
} as const

export type ExpirationPolicy = (typeof ExpirationPolicy)[keyof typeof ExpirationPolicy]

export interface Contract {
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
  exp_notice_period?: number
  exp_policy?: ExpirationPolicy
  responsible?: ContractResponsible
  contract_data?: ContractData
  negotiations?: ContractNegotiation[]
  outdated?: boolean
  latest_template_did?: string
  template_did?: string
  template_version?: number
  template_is_deprecated?: boolean
  parent_contract_did?: string
}

export type ContractChangeRequest = Pick<Contract, 'name' | 'description' | 'exp_notice_period' | 'exp_policy'> & {
  contract_data?: Partial<Contract['contract_data']>
}
