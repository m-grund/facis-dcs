import { defineStore } from 'pinia'
import type { ContractEditorTabId, ContractEditorUiState } from '../models/contract-editor-ui-store'
import type { ContractState as ContractStateType } from '@/types/contract-state'
import { ContractState } from '@/types/contract-state'
import { useAuthStore } from '@/stores/auth-store'

const storeId = 'contractEditorUi'
const defaultState: Readonly<ContractEditorUiState> = {
  activeTab: 'details',
  tabs: [
    { id: 'details', label: 'Contract Details' },
    { id: 'content', label: 'Contract Content' },
    { id: 'semantic', label: 'Semantic Rules' },
    { id: 'clauses', label: 'Clauses' },
    { id: 'builder', label: 'Builder' },
    { id: 'diff', label: 'Diff View' },
    { id: 'audit', label: 'Audit History' },
  ],
}

export const useContractEditorUiStore = defineStore(storeId, {
  state: (): ContractEditorUiState => getInitialState(),
  actions: {
    setActiveTab(tab: ContractEditorTabId) {
      this.activeTab = tab
    },
    availableTabs(contractState: ContractStateType) {
      const isManager = useAuthStore().user?.roles?.includes('CONTRACT_MANAGER') ?? false

      switch (contractState) {
        case ContractState.draft:
          // Keep the edit page simple for now, same for the negotiation states
          return this.tabs.filter(tab => ['details', 'content'].includes(tab.id))
        case ContractState.negotiation:
          return this.tabs.filter((tab) => ['details', 'content', 'diff'].includes(tab.id) || isManager && tab.id === 'audit')
        default:
          return this.tabs.filter(tab => ['details', 'content'].includes(tab.id) || isManager && tab.id === 'audit')
      }
    },
    reset(overrides?: Partial<ContractEditorUiState>) {
      Object.assign(this, getInitialState())
      if (overrides) Object.assign(this, overrides)
    },
  },
})

function getInitialState(): ContractEditorUiState {
  return {
    ...defaultState,
    tabs: [...defaultState.tabs],
  }
}
