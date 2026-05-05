import type { ContractChangeRequest } from './contract'
import type { ContractNegotiationDecision } from './contract-negotiation-decision'

export interface ContractNegotiation {
  id: string
  change_request: ContractChangeRequest
  created_by: string
  created_at: string
  contract_version?: number
  negotiation_decisions: ContractNegotiationDecision[]
}
