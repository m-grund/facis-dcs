import type {
  SignatureApplyRequest,
  SignatureAuditRequest,
  SignatureComplianceRequest,
  SignatureRetrieveByIDRequest,
  SignatureRetrieveRequest,
  SignatureRevokeRequest,
  SignatureValidateRequest,
  SignatureVerifyRequest,
} from '../requests/signature-request'
import type {
  SignatureApplyResponse,
  SignatureAuditResponse,
  SignatureComplianceResponse,
  SignatureRetrieveByIDResponse,
  SignatureRetrieveResponse,
  SignatureRevokeResponse,
  SignatureValidateResponse,
  SignatureVerifyResponse,
} from '../responses/signature-response'

export interface SignatureManagementService {
  retrieve: (request?: SignatureRetrieveRequest) => Promise<SignatureRetrieveResponse>
  retrieveByID: (request: SignatureRetrieveByIDRequest) => Promise<SignatureRetrieveByIDResponse>
  verify: (request: SignatureVerifyRequest) => Promise<SignatureVerifyResponse>
  apply: (request: SignatureApplyRequest) => Promise<SignatureApplyResponse>
  validate: (request: SignatureValidateRequest) => Promise<SignatureValidateResponse>
  revoke: (request: SignatureRevokeRequest) => Promise<SignatureRevokeResponse>
  audit: (request: SignatureAuditRequest) => Promise<SignatureAuditResponse>
  compliance: (request: SignatureComplianceRequest) => Promise<SignatureComplianceResponse>
}
