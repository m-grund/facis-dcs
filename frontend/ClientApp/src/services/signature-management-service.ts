import http from '@/api/http'

export interface SignatureContract {
  did: string
  contract_version?: number
  state: string
  name?: string
  description?: string
  created_at: string
  updated_at: string
}

export interface SignatureEnvelope {
  contract_did: string
  signer_did: string
  credential_type: string
  status: string
  signed_at?: string
  revoked_at?: string
  ipfs_cid?: string
}

export interface SignatureVerifyResult {
  did: string
  match: boolean
  jsonld_hash?: string
  base_pdf_hash?: string
  sig_count: number
  findings?: string[]
}

export interface SignatureValidateResult {
  did: string
  findings?: string[]
}

export interface SignatureComplianceResult {
  did: string
  findings?: string[]
}

export interface SignatureAuditEntry {
  id: number
  component: string
  event_type: string
  event_data: unknown
  did?: string
  created_at: string
  res_log_pred_cid?: string
  global_log_pred_cid?: string
}

export const signatureManagementService = {
  async retrieveContracts(): Promise<SignatureContract[]> {
    return http
      .get<{ contracts: SignatureContract[]; signing_tasks: unknown[] }>('/signature/retrieve')
      .then((res) => res.data.contracts ?? [])
  },

  async applySignature(
    did: string,
    signerDid: string,
    credentialType = 'stub',
  ): Promise<SignatureEnvelope | undefined> {
    return http
      .post<{ did: string; signature_envelope?: SignatureEnvelope }>('/signature/apply', {
        did,
        signer_did: signerDid,
        credential_type: credentialType,
        updated_at: new Date().toISOString(),
      })
      .then((res) => res.data.signature_envelope)
  },

  async verifySignature(did: string): Promise<SignatureVerifyResult> {
    return http.post<SignatureVerifyResult>('/signature/verify', { did }).then((res) => res.data)
  },

  async validateSignature(did: string): Promise<SignatureValidateResult> {
    return http.post<SignatureValidateResult>('/signature/validate', { did }).then((res) => res.data)
  },

  async complianceCheck(did: string): Promise<SignatureComplianceResult> {
    return http.post<SignatureComplianceResult>('/signature/compliance', { did }).then((res) => res.data)
  },

  async revokeSignature(did: string, signerDid: string): Promise<void> {
    await http.post('/signature/revoke', { did, signer_did: signerDid })
  },

  async getAudit(did: string): Promise<SignatureAuditEntry[]> {
    return http.get<SignatureAuditEntry[]>('/signature/audit', { params: { did } }).then((res) => res.data ?? [])
  },
}
