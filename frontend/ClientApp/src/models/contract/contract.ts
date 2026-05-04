import type { ContractState } from '@/types/contract-state'
import type { ContractNegotiation } from './contract-negotiation'
import type { ContractData } from '../contract-data'

export interface Contract {
  did: string
  contract_version?: number
  state: ContractState
  name?: string
  description?: string
  created_by: string
  created_at: string
  updated_at: string
  expiration_date?: string
  contract_data?: ContractData
  negotiations?: ContractNegotiation[]
}

export type ContractChangeRequest = Pick<Contract, 'name' | 'description'> & { contract_data?: Partial<Contract['contract_data']> }
