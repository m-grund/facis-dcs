import type {
  ArchiveAuditRequest,
  ArchiveDeleteRequest,
  ArchiveRetrieveRequest,
  ArchiveSearchRequest,
  ArchiveStoreRequest,
  ArchiveTerminateRequest,
} from '../requests/archive-request'
import type {
  ArchiveAuditResponse,
  ArchiveDeleteResponse,
  ArchiveRetrieveResponse,
  ArchiveSearchResponse,
  ArchiveStoreResponse,
  ArchiveTerminateResponse,
} from '../responses/archive-response'

export interface ContractStorageArchiveService {
  retrieve: (request?: ArchiveRetrieveRequest) => Promise<ArchiveRetrieveResponse>
  search: (request: ArchiveSearchRequest) => Promise<ArchiveSearchResponse>
  store: (request: ArchiveStoreRequest) => Promise<ArchiveStoreResponse>
  terminate: (request: ArchiveTerminateRequest) => Promise<ArchiveTerminateResponse>
  delete: (request: ArchiveDeleteRequest) => Promise<ArchiveDeleteResponse>
  audit: (request: ArchiveAuditRequest) => Promise<ArchiveAuditResponse>
}
