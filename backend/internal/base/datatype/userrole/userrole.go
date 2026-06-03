package userrole

import (
	"fmt"
)

type UserRole string

const (
	TemplateCreator  UserRole = "Template Creator"
	TemplateReviewer UserRole = "Template Reviewer"
	TemplateApprover UserRole = "Template Approver"
	TemplateManager  UserRole = "Template Manager"

	ContractCreator  UserRole = "Contract Creator"
	ContractReviewer UserRole = "Contract Reviewer"
	ContractApprover UserRole = "Contract Approver"
	ContractManager  UserRole = "Contract Manager"
	ContractSigner   UserRole = "Contract Signer"
	ContractObserver UserRole = "Contract Observer"

	ArchiveManager      UserRole = "Archive Manager"
	Auditor             UserRole = "Auditor"
	SystemAdministrator UserRole = "Sys. Administrator"
	ComplianceOfficer   UserRole = "Compliance Officer"
	IntegrationManager  UserRole = "Ingestion Manager"

	ProcessOrchestrator UserRole = "Process Orchestrator"
	Validator           UserRole = "Validator"

	SystemContractCreator  UserRole = "Sys. Contract Creator"
	SystemContractReviewer UserRole = "Sys. Contract Reviewer"
	SystemContractApprover UserRole = "Sys. Contract Approver"
	SystemContractManager  UserRole = "Sys. Contract Manager"
	SystemContractSigner   UserRole = "Sys. Contract Signer"
	ContractTargetSystem   UserRole = "Contract Target System"
)

func NewUserRole(s string) (UserRole, error) {
	ts := UserRole(s)
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid user role state: %s", s)
	}
	return ts, nil
}

// IsValid checks if the UserRole is a valid role
func (r UserRole) IsValid() bool {
	switch r {
	case TemplateCreator, TemplateReviewer, TemplateApprover, TemplateManager,
		ContractCreator, ContractReviewer, ContractApprover, ContractManager,
		ContractSigner, ContractObserver,
		ArchiveManager, Auditor, SystemAdministrator, ComplianceOfficer, IntegrationManager,
		ProcessOrchestrator, Validator,
		SystemContractCreator, SystemContractReviewer, SystemContractApprover,
		SystemContractManager, SystemContractSigner, ContractTargetSystem:
		return true
	}
	return false
}

// String returns the string representation of the UserRole
func (r UserRole) String() string {
	return string(r)
}

// IsSystemRole returns true if the role is a system/automated role
func (r UserRole) IsSystemRole() bool {
	switch r {
	case SystemContractCreator, SystemContractReviewer, SystemContractApprover,
		SystemContractManager, SystemContractSigner, ContractTargetSystem:
		return true
	}
	return false
}

// IsHumanRole returns true if the role is a human user role
func (r UserRole) IsHumanRole() bool {
	return r.IsValid() && !r.IsSystemRole()
}

type UserRoles []UserRole

func (r UserRoles) HasRole(requiredRole UserRole) bool {
	for _, role := range r {
		if role == requiredRole {
			return true
		}
	}
	return false
}

func (r UserRoles) HasRoles(requiredRoles ...UserRole) bool {
	for _, requiredRole := range requiredRoles {
		for _, role := range r {
			if role == requiredRole {
				return true
			}
		}
	}
	return false
}
