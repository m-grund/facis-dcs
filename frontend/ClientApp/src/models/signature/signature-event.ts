import type { ComponentType } from '@/types/component-type'

export interface SignatureRetrieveByIDEvent {
  did: string
  retrieved_by: string
  occured_at: string
}

export interface SignatureRetrieveAllEvent {
  retrieved_by: string
  occurred_at: string
}

export interface SignatureValidateEvent {
  did: string
  contract_version?: number
  validated_by: string
  occurred_at: string
}

export interface SignatureAuditEvent {
  did: string
  audited_by: string
  occurred_at: string
  component_type: ComponentType
}

export interface SignatureRevokeEvent {
  did: string
  contract_version?: number
  revoked_by: string
  occurred_at: string
}

export interface SignatureComplianceValidationEvent {
  did: string
  contract_version?: number
  validated_by: string
  occurred_at: string
}

export type SignatureEvent =
  | SignatureRetrieveByIDEvent
  | SignatureRetrieveAllEvent
  | SignatureValidateEvent
  | SignatureAuditEvent
  | SignatureRevokeEvent
  | SignatureComplianceValidationEvent
