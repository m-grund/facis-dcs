import http from '@/api/http'
import type {
  SignatureApplyRequest,
  SignatureAuditRequest,
  SignatureComplianceRequest,
  SignatureRetrieveByIDRequest,
  SignatureRetrieveRequest,
  SignatureRevokeRequest,
  SignatureValidateRequest,
  SignatureVerifyRequest,
} from '@/models/requests/signature-request'
import type {
  SignatureApplyResponse,
  SignatureAuditResponse,
  SignatureComplianceResponse,
  SignatureRetrieveByIDResponse,
  SignatureRetrieveResponse,
  SignatureRevokeResponse,
  SignatureValidateResponse,
  SignatureVerifyResponse,
} from '@/models/responses/signature-response'
import type { SignatureManagementService } from '@/models/services/signature-management-service'

export const signatureManagementService: SignatureManagementService = {
  async retrieve(_request?: SignatureRetrieveRequest) {
    return http
      .get<SignatureRetrieveResponse>('/signature/retrieve')
      .then((res) => res.data)
      .catch((err) => {
        console.error('Retrieve Error:', err)
        return { contracts: [], signing_tasks: [] }
      })
  },

  async retrieveByID(request: SignatureRetrieveByIDRequest) {
    return http.get<SignatureRetrieveByIDResponse>(`/signature/retrieve/${request.did}`).then((res) => res.data)
  },

  async verify(request: SignatureVerifyRequest) {
    return http.post<SignatureVerifyResponse>('/signature/verify', request).then((res) => res.data)
  },

  async apply(request: SignatureApplyRequest) {
    return http.post<SignatureApplyResponse>('/signature/apply', request).then((res) => res.data)
  },

  async validate(request: SignatureValidateRequest) {
    return http.post<SignatureValidateResponse>('/signature/validate', request).then((res) => res.data)
  },

  async revoke(request: SignatureRevokeRequest) {
    return http.post<SignatureRevokeResponse>('/signature/revoke', request).then((res) => res.data)
  },

  async audit(request: SignatureAuditRequest) {
    return http
      .get<SignatureAuditResponse>(`/signature/audit/${request.did}`)
      .then((res) => res.data)
      .catch((err) => {
        console.error('Audit Error:', err)
        return []
      })
  },

  async compliance(request: SignatureComplianceRequest) {
    return http.post<SignatureComplianceResponse>('/signature/compliance', request).then((res) => res.data)
  },
}
