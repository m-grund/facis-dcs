import type { ComponentType } from '@/types/component-type'
import type { ContractData } from '../contract-data'
import type { ContractState } from '@/types/contract-state'
import type { ContractActionFlag } from '@/types/contract-action-flag'
import type { ContractChangeRequest } from './contract'

export interface ContractCreateEvent {
  did: string
  template_did: string
  created_by: string
  name: string
  description: string
  contract_data: ContractData
  occurred_at: string
}

export interface ContractUpdateEvent {
  did: string
  updated_at: string
  old_contract_version?: number
  new_contract_version?: number
  old_name?: string
  new_name?: string
  old_description?: string
  new_description?: string
  old_contract_data?: ContractData
  new_contract_data?: ContractData
  occurred_at: string
  old_expiration_date?: string
  new_expiration_date?: string
}

export interface ContractSubmitEvent {
  did: string
  previous_state: ContractState
  new_state: ContractState
  submitted_by: string
  occurred_at: string
  contract_version?: number
  action_flag: ContractActionFlag
  comments: string[]
}

export interface ContractRetrieveByIDEvent {
  did: string
  retrieved_by: string
  occurred_at: string
}

export interface ContractRetrieveAllEvent {
  retrieved_by: string
  occurred_at: string
}

export interface ContractVerifyEvent {
  did: string
  contract_version?: number
  verified_by: string
  occurred_at: string
}

export interface ContractNegotiationEvent {
  did: string
  contract_version?: number
  change_request?: ContractChangeRequest
  negotiated_by: string
  occurred_at: string
  negotiators: string[]
}

export interface ContractAcceptNegotiationEvent {
  did: string
  contract_version?: number
  accepted_by: string
  occurred_at: string
}

export interface ContractRejectNegotiationEvent {
  did: string
  contract_version?: number
  rejected_by: string
  rejection_reason?: string
  occurred_at: string
}

export interface ContractApproveEvent {
  did: string
  contract_version?: number
  approved_by: string
  occurred_at: string
}

export interface ContractRejectEvent {
  did: string
  contract_version?: string
  rejected_by: string
  reason: string
  occurred_at: string
}

export interface ContractTerminateEvent {
  did: string
  contract_version?: string
  reason: string
  terminated_by: string
  occurred_at: string
}

export interface ContractRecordEvidenceEvent {
  did: string
  contract_version?: string
  recorded_by: string
  occurred_at: string
}

export interface ContractAuditEvent {
  did: string
  audited_by: string
  occurred_at: string
  component_type: ComponentType
}

export interface ContractReviewEvent {
  did: string
  reviewed_by: string
  occurred_at: string
}

export interface ContractIncreaseContractVersionEvent {
  did: string
  old_contract_version?: number
  new_contract_versiom?: number
  submitted_by: string
  occurred_at: string
}

export type ContractEvent =
  | ContractCreateEvent
  | ContractUpdateEvent
  | ContractSubmitEvent
  | ContractRetrieveByIDEvent
  | ContractRetrieveAllEvent
  | ContractVerifyEvent
  | ContractNegotiationEvent
  | ContractAcceptNegotiationEvent
  | ContractRejectNegotiationEvent
  | ContractApproveEvent
  | ContractRejectEvent
  | ContractTerminateEvent
  | ContractRecordEvidenceEvent
  | ContractAuditEvent
  | ContractReviewEvent
  | ContractIncreaseContractVersionEvent
