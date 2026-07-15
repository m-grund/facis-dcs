import { getClauseCatalog, type ClauseCatalogType } from '@/services/semantic-hub-service'
import { ref } from 'vue'

/**
 * Fetches the Semantic Hub's active clause catalog (Phase 3, ADR-10) once
 * per composable instance and exposes it for the template builder's typed
 * clause palette (TypedClausePalette.vue) to render — the palette and
 * server-side enforcement both derive from the same hub-stored SHACL
 * shapes, so registering a new clause type here is reflected in the
 * palette without a frontend deploy.
 */
export function useClauseCatalog() {
  const clauses = ref<ClauseCatalogType[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load() {
    loading.value = true
    error.value = null
    try {
      const catalog = await getClauseCatalog()
      clauses.value = catalog.clauses
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load the clause catalog'
    } finally {
      loading.value = false
    }
  }

  return { clauses, loading, error, load }
}
