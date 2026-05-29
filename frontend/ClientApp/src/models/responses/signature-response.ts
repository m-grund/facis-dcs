import type { ComponentType } from '@/types/component-type'
import type { SignatureContract } from '../signature/signature-contract'
import type { SignatureSigningTask as SignatureSigningTask } from '../signature/signature-signing-task'
import type { SignatureEventType } from '@/types/signature-event-type'
import type { SignatureEvent } from '../signature/signature-event'

export interface SignatureRetrieveResponse {
  contracts: SignatureContract[]
  signing_tasks: SignatureSigningTask[]
}

export interface SignatureRetrieveByIDResponse {
  contract: SignatureContract
  signature_envelope: unknown
}

export interface SignatureVerifyResponse {
  did: string
}

export interface SignatureApplyResponse {
  did: string
}

export interface SignatureValidateResponse {
  did: string
  findings?: string[]
}

export interface SignatureRevokeResponse {
  did: string
}

export interface SignatureAuditResponseItem {
  id: number
  component: ComponentType
  event_type: SignatureEventType
  event_data: SignatureEvent
  did: string
  created_at: string
  res_log_pred_cid?: string
  global_log_pred_cid?: string
}

export type SignatureAuditResponse = SignatureAuditResponseItem[]

export interface SignatureComplianceResponse {
  did: string
  findings?: string[]
}
