import http from '@/api/http'
import type {
  ArchiveAuditRequest,
  ArchiveDeleteRequest,
  ArchiveRetrieveRequest,
  ArchiveSearchRequest,
  ArchiveStoreRequest,
  ArchiveTerminateRequest,
} from '@/models/requests/archive-request'
import type {
  ArchiveAuditResponse,
  ArchiveDeleteResponse,
  ArchiveRetrieveResponse,
  ArchiveSearchResponse,
  ArchiveStoreResponse,
  ArchiveTerminateResponse,
} from '@/models/responses/archive-response'
import type { ContractStorageArchiveService } from '@/models/services/contract-storage-archive-service'

export const contractStorageArchiveService: ContractStorageArchiveService = {
  async retrieve(_request?: ArchiveRetrieveRequest) {
    return http.get<ArchiveRetrieveResponse>('/archive/retrieve').then((res) => res.data)
  },

  async search(request: ArchiveSearchRequest) {
    return http.get<ArchiveSearchResponse>('/archive/search', { params: request }).then((res) => res.data)
  },

  async store(request: ArchiveStoreRequest) {
    return http.post<ArchiveStoreResponse>('/archive/store', request).then((res) => res.data)
  },

  async terminate(request: ArchiveTerminateRequest) {
    return http.post<ArchiveTerminateResponse>('/archive/terminate', request).then((res) => res.data)
  },

  async delete(request: ArchiveDeleteRequest) {
    return http.delete<ArchiveDeleteResponse>('/archive/delete', { params: request }).then((res) => res.data)
  },

  async audit(request: ArchiveAuditRequest) {
    return http.get<ArchiveAuditResponse>('/archive/audit', { params: request }).then((res) => res.data)
  },
}
