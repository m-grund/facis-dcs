package userrole

type UserRole string

const (
	// Human User Roles - Template Management
	TemplateCreator  UserRole = "Template Creator"
	TemplateReviewer UserRole = "Template Reviewer"
	TemplateApprover UserRole = "Template Approver"
	TemplateManager  UserRole = "Template Manager"

	// Human User Roles - Contract Management
	ContractCreator  UserRole = "Contract Creator"
	ContractReviewer UserRole = "Contract Reviewer"
	ContractApprover UserRole = "Contract Approver"
	ContractManager  UserRole = "Contract Manager"
	ContractSigner   UserRole = "Contract Signer"
	ContractObserver UserRole = "Contract Observer"

	// Human User Roles - System Administration
	ArchiveManager      UserRole = "Archive Manager"
	Auditor             UserRole = "Auditor"
	SystemAdministrator UserRole = "System Administrator"
	ComplianceOfficer   UserRole = "Compliance Officer"
	IntegrationManager  UserRole = "Ingestion Manager"

	// Human User Roles - Process Management
	ProcessOrchestrator UserRole = "Process Orchestrator"
	Validator           UserRole = "Validator"

	// System User Roles - API/Automated
	SystemContractCreator  UserRole = "System Contract Creator"
	SystemContractReviewer UserRole = "System Contract Reviewer"
	SystemContractApprover UserRole = "System Contract Approver"
	SystemContractManager  UserRole = "System Contract Manager"
	SystemContractSigner   UserRole = "System Contract Signer"
	ContractTargetSystem   UserRole = "Contract Target System"
)

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
