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

export type CeremonyStatus = 'pending' | 'verified' | 'expired' | 'failed'

export interface CeremonyStartResult {
  ceremony_id: string
  wallet_uri: string
  expires_at: string
  status: CeremonyStatus
}

export interface CeremonyStatusResult {
  ceremony_id: string
  contract_did: string
  field_name?: string
  status: CeremonyStatus
  signer_did?: string
  expires_at?: string
}

export const signatureManagementService = {
  async retrieveContracts(): Promise<SignatureContract[]> {
    return http
      .get<{ contracts: SignatureContract[]; signing_tasks: unknown[] }>('/signature/retrieve')
      .then((res) => res.data.contracts ?? [])
  },

  async startCeremony(contractDid: string, fieldName: string): Promise<CeremonyStartResult> {
    return http
      .post<CeremonyStartResult>('/signature/request', { contract_did: contractDid, field_name: fieldName })
      .then((res) => res.data)
  },

  async getCeremonyStatus(ceremonyId: string): Promise<CeremonyStatusResult> {
    return http.get<CeremonyStatusResult>(`/signature/request/${ceremonyId}`).then((res) => res.data)
  },

  // The DCS holds no signing key (ADR-12). Signing is two steps: prepare the
  // to-be-signed PDF (PoA + summary embedded, signature field placed), which the
  // signatory signs externally (their wallet/QTSP, or a desktop PAdES signer),
  // then submit the signed PDF for validation and recording.
  async prepareSignature(did: string, signerDid: string, credentialType: string): Promise<Blob> {
    const res = await http.post<{ document: string }>('/signature/prepare', {
      did,
      signer_did: signerDid,
      credential_type: credentialType,
    })
    const bytes = Uint8Array.from(atob(res.data.document), (c) => c.charCodeAt(0))
    return new Blob([bytes], { type: 'application/pdf' })
  },

  async submitSignature(
    did: string,
    signerDid: string,
    credentialType: string,
    signedPdf: Blob,
    expectedSignatory: string,
  ): Promise<SignatureEnvelope | undefined> {
    const buffer = await signedPdf.arrayBuffer()
    let binary = ''
    new Uint8Array(buffer).forEach((b) => {
      binary += String.fromCharCode(b)
    })
    const res = await http.post<{ did: string; signature_envelope?: SignatureEnvelope }>('/signature/submit', {
      did,
      signer_did: signerDid,
      credential_type: credentialType,
      expected_signatory: expectedSignatory,
      signed_pdf: btoa(binary),
      jades_signature: '',
    })
    return res.data.signature_envelope
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
