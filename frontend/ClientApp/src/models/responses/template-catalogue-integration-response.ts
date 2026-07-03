import type { TemplateResource, TemplateResourcesItem } from '@/modules/template-catalogue/models/template-resource'

export interface TemplateCatalogueRetrieveResponse {
  totalCount: number
  items: TemplateResourcesItem[]
}

export type TemplateCatalogueRetrieveByIdResponse = TemplateResource
