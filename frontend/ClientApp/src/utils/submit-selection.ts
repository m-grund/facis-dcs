import type { SelectedUserRole } from '@/models/user'
import type { UserRole } from '@/types/user-role'

/** Assignee IDs must match JWT `sub` (see backend middleware.GetDID / task retrieve). */
export function assigneeIdsForRole(selection: SelectedUserRole[], role: UserRole | 'CONTRACT_NEGOTIATOR'): string[] {
  return [...new Set(selection.filter((item) => item.role === role).map((item) => item.user.id))]
}

export function firstAssigneeIdForRole(selection: SelectedUserRole[], role: UserRole): string | undefined {
  return assigneeIdsForRole(selection, role)[0]
}
