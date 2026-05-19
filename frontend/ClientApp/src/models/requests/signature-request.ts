export interface SignatureRetrieveRequest {}

export interface SignatureRetrieveByIDRequest {
  did: string
}

export interface SignatureVerifyRequest {
  did: string
}

export interface SignatureApplyRequest {
  did: string
  updated_at: string
}

export interface SignatureValidateRequest {
  did: string
}

export interface SignatureRevokeRequest {
  did: string
}

export interface SignatureAuditRequest {
  did: string
}

export interface SignatureComplianceRequest {
  did: string
}
