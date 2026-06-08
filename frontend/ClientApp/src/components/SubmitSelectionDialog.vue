<script setup lang="ts">
import type { SelectedUserRole, UserProfile } from '@/models/user'
import { userService } from '@/services/user-service'
import type { UserRole } from '@/types/user-role'
import { toProperCase } from '@/utils/string'
import { computed, ref, useTemplateRef } from 'vue'

interface User extends UserProfile {
  availableRoles: (UserRole | 'CONTRACT_NEGOTIATOR')[]
}

interface SelectionRow {
  user: User
  selected: boolean
  roles: (UserRole | 'CONTRACT_NEGOTIATOR')[]
}

const props = defineProps<{
  dialogType: 'template' | 'contract'
}>()

const emit = defineEmits<{
  submit: [value: SelectedUserRole[]]
}>()

const userSelectionModal = useTemplateRef('user-selection-modal')

const selectionRows = ref<SelectionRow[]>([])
const isLoading = ref(true)

const roles = computed(() => {
  const roleMap: Record<typeof props.dialogType, { review: UserRole; approve: UserRole; negotiate?: UserRole }> = {
    template: { review: 'TEMPLATE_REVIEWER', approve: 'TEMPLATE_APPROVER' },
    contract: { review: 'CONTRACT_REVIEWER', approve: 'CONTRACT_APPROVER', negotiate: 'CONTRACT_CREATOR' },
  }
  return roleMap[props.dialogType]
})
const reviewRole = computed(() => roles.value.review)
const approveRole = computed(() => roles.value.approve)
const negotiateRole = computed(() => roles.value.negotiate)

const selectedRows = computed(() => selectionRows.value.filter((row) => row.selected))

const allAssignedRoles = computed(() => selectedRows.value.flatMap((row) => row.roles))

const hasSelectedUsers = computed(() => selectedRows.value.length > 0)
const allSelectedUsersHaveRoles = computed(() => selectedRows.value.every((row) => row.roles.length > 0))
const hasValidSelection = computed(() => {
  const assigned = allAssignedRoles.value
  const hasApprove = assigned.includes(approveRole.value)
  const hasReview = assigned.includes(reviewRole.value)
  const hasNegotiate = !negotiateRole.value || assigned.includes('CONTRACT_NEGOTIATOR')
  return hasApprove && hasReview && hasNegotiate
})
const isSubmitDisabled = computed(
  () => !hasSelectedUsers.value || !allSelectedUsersHaveRoles.value || !hasValidSelection.value,
)

function clearSelectionRows() {
  selectionRows.value = []
}

async function openModal() {
  clearSelectionRows()
  isLoading.value = true
  userSelectionModal.value?.showModal()

  const receivedUsers = await userService.getAuthorizedUsersWithRoles(
    approveRole.value,
    reviewRole.value,
    negotiateRole.value,
  )
  selectionRows.value = receivedUsers.map((user) => ({
    user: {
      ...user,
      availableRoles:
        user.roleIds?.reduce<(UserRole | 'CONTRACT_NEGOTIATOR')[]>((acc, role) => {
          const newRole = 'CONTRACT_NEGOTIATOR'
          if (role === 'CONTRACT_CREATOR') {
            acc.push(newRole)
          } else if (role === 'CONTRACT_REVIEWER') {
            acc.push(role, newRole)
          } else {
            acc.push(role)
          }
          return [...new Set(acc)]
        }, []) ?? [],
    },
    selected: false,
    roles: [],
  }))

  isLoading.value = false
}

function onRowSelectedChange(row: SelectionRow) {
  if (!row.selected) {
    row.roles = []
  }
}

function roleDropdownLabel(row: SelectionRow) {
  if (row.roles.length === 0) return 'No role'
  return row.roles.map((role) => toProperCase(role)).join(', ')
}

function isRoleChecked(row: SelectionRow, role: UserRole | 'CONTRACT_NEGOTIATOR') {
  return row.roles.includes(role)
}

function toggleRole(row: SelectionRow, role: UserRole | 'CONTRACT_NEGOTIATOR', checked: boolean) {
  if (checked) {
    if (!row.roles.includes(role)) {
      row.roles.push(role)
    }
  } else {
    row.roles = row.roles.filter((r) => r !== role)
  }
}

function onModalSubmit() {
  emit('submit', [])
  onModalClose()
}

function onModalClose() {
  userSelectionModal.value?.close()
  clearSelectionRows()
  isLoading.value = true
}

const roleInfoText = computed(() => {
  return props.dialogType === 'template'
    ? 'Select users, then choose roles from the dropdown (one user may be Approver and Reviewer).'
    : 'Select users, then choose roles from the dropdown (roles may overlap on one user).'
})
</script>

<template>
  <button :="$attrs" @click="openModal">Submit</button>
  <dialog ref="user-selection-modal" class="modal modal-bottom transition-none sm:modal-middle" @close="onModalClose">
    <div class="modal-box flex max-h-2/3 flex-col">
      <h3 class="text-lg font-bold">
        User Selection for {{ dialogType === 'template' ? 'Template' : 'Contract' }} Submission
      </h3>
      <p class="py-4 text-sm">
        {{ roleInfoText }}
      </p>
      <div class="grow overflow-y-auto">
        <div v-if="isLoading">Loading...</div>
        <ul v-else class="list">
          <li
            v-for="row in selectionRows"
            :key="row.user.id"
            class="list-row mb-1 items-center border border-secondary py-2"
          >
            <label class="list-col-grow label min-w-0">
              <input
                v-model="row.selected"
                type="checkbox"
                class="checkbox mr-4 shrink-0 checkbox-primary"
                @change="onRowSelectedChange(row)"
              />
              <span class="truncate">{{ `${row.user.firstName} ${row.user.lastName}` }}</span>
            </label>
            <div
              class="dropdown dropdown-end w-full max-w-56 sm:max-w-64"
              :class="{ 'pointer-events-none opacity-50': !row.selected }"
            >
              <div
                tabindex="0"
                role="button"
                class="select w-full truncate select-sm text-left select-primary sm:select-md"
                :aria-disabled="!row.selected"
              >
                {{ roleDropdownLabel(row) }}
              </div>
              <ul
                tabindex="0"
                class="dropdown-content menu z-20 mt-1 w-full min-w-52 rounded-box border border-base-300 bg-base-100 p-2 shadow-lg"
                @click.stop
              >
                <li v-for="role in row.user.availableRoles" :key="role" @click.stop>
                  <label class="flex cursor-pointer items-center gap-3 px-2 py-2">
                    <input
                      type="checkbox"
                      class="checkbox checkbox-sm checkbox-primary"
                      :checked="isRoleChecked(row, role)"
                      :disabled="!row.selected"
                      @change="toggleRole(row, role, ($event.target as HTMLInputElement).checked)"
                      @click.stop
                    />
                    <span>{{ toProperCase(role) }}</span>
                  </label>
                </li>
              </ul>
            </div>
          </li>
        </ul>
      </div>
      <div class="modal-action">
        <div v-if="isSubmitDisabled" class="flex items-center text-sm text-error">
          <span v-if="!hasSelectedUsers">Select at least one user</span>
          <span v-else-if="!allSelectedUsersHaveRoles">Assign at least one role to each selected user</span>
          <span v-else>{{ roleInfoText }}</span>
        </div>
        <button class="btn btn-outline" @click="onModalClose">Cancel</button>
        <button class="btn btn-primary" @click="onModalSubmit">Apply</button>
      </div>
    </div>
  </dialog>
</template>
