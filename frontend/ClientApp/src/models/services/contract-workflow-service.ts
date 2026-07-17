import type { Contract } from '@/models/contract/contract'
import type {
  ContractApproveRequest,
  ContractAuditRequest,
  ContractCreateRequest,
  ContractDeployRequest,
  ContractHistoryRetrieveRequest,
  ContractNegotiationRequest,
  ContractNegotiationRespondRequest,
  ContractRejectRequest,
  ContractRetrieveByIdRequest,
  ContractRetrieveRequest,
  ContractReviewRequest,
  ContractSearchRequest,
  ContractStoreRequest,
  ContractSubmitRequest,
  ContractTerminateRequest,
  ContractUpdateRequest,
} from '@/models/requests/contract-requests'
import type {
  ApprovedContractTemplateRetrieveResponse,
  ContractApproveResponse,
  ContractAuditResponse,
  ContractCreateResponse,
  ContractDeployResponse,
  ContractHistoryResponse,
  ContractNegotiationRespondResponse,
  ContractNegotiationResponse,
  ContractRejectResponse,
  ContractRetrieveResponse,
  ContractReviewResponse,
  ContractSearchResponse,
  ContractStoreResponse,
  ContractSubmitResponse,
  ContractTerminateResponse,
  ContractUpdateResponse,
} from '@/models/responses/contract-response'

export interface ContractWorkflowService {
  create: (request: ContractCreateRequest) => Promise<ContractCreateResponse>
  update: (request: ContractUpdateRequest) => Promise<ContractUpdateResponse>
  submit: (request: ContractSubmitRequest) => Promise<ContractSubmitResponse>
  negotiate: (request: ContractNegotiationRequest) => Promise<ContractNegotiationResponse>
  respond: (request: ContractNegotiationRespondRequest) => Promise<ContractNegotiationRespondResponse>
  review: (request: ContractReviewRequest) => Promise<ContractReviewResponse>
  retrieveApprovedTemplates: () => Promise<ApprovedContractTemplateRetrieveResponse>
  retrieve: (request?: ContractRetrieveRequest) => Promise<ContractRetrieveResponse>
  retrieveById: (request: ContractRetrieveByIdRequest) => Promise<Contract | null>
  search: (request: ContractSearchRequest) => Promise<ContractSearchResponse>
  approve: (request: ContractApproveRequest) => Promise<ContractApproveResponse>
  reject: (request: ContractRejectRequest) => Promise<ContractRejectResponse>
  store: (request: ContractStoreRequest) => Promise<ContractStoreResponse>
  terminate: (request: ContractTerminateRequest) => Promise<ContractTerminateResponse>
  deploy: (request: ContractDeployRequest) => Promise<ContractDeployResponse>
  audit: (request: ContractAuditRequest) => Promise<ContractAuditResponse>
  retrieveHistoryByDid: (request: ContractHistoryRetrieveRequest) => Promise<ContractHistoryResponse>
  exportPdf: (did: string) => Promise<Blob>
  exportBundle: (did: string) => Promise<Blob>
  verifyPdf: (did: string) => Promise<{
    match: boolean
    jsonld_hash: string
    base_pdf_hash: string
    stored_base_pdf_hash: string
    c2pa_manifest_found?: boolean
    c2pa_signature_valid?: boolean
    vc_proof_valid?: boolean
    status_list_uri?: string
    lifecycle_status?: string
    status_list_status?: string
  }>
}
