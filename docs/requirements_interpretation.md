Agent-Readable Requirements Interpretation
==========================================

source_matrix: /home/kengels/Downloads/DCS Requirements -- Use Cases Matrix (1).xlsx
source_srs: docs/SRS_FACIS_DCS.txt
authority: Primary source of truth for DCS requirements used by implementation agents.
purpose: Single interpretation file for implementation agents. Use the effective_* fields as the
  implementation and verification target. Use source_* fields only as traceability back to the
  matrix/SRS. The SRS is supplementary context and does not override this file.

status_mapping:
  d: Done
  o: Ongoing
  n: Not Started

agent_rules:
  - Treat effective_requirement as authoritative for implementation planning.
  - Treat effective_acceptance_criteria as authoritative for verification when present.
  - Use docs/SRS_FACIS_DCS.txt only for additional context, definitions, and background.
  - If the SRS conflicts with this file, follow this file.
  - If interpretation_status is Unchanged, the source requirement remains unchanged.
  - If interpretation_status contains Blocked or Pending, do not invent a product decision;
    implement only the explicitly allowed subset or adapter boundary.
  - Non-relevant matrix notes are context only and must not weaken the requirement.

summary:
  total_entries: 290
  adjusted_entries: 13
  unchanged_entries: 277


## System Requirements

### DCS-PC-01 - Use of XFSC Components
id: DCS-PC-01
area: System Requirements
implementation_status: Ongoing
category: Product constraints
interpretation_status: Adjusted Alternative Implementation
source_requirement: |
  The DCS MUST leverage existing XFSC components (e.g., Catalogue, Orchestration Engine,
  Revocation List) wherever applicable. Alternative implementations of these functionalities are
  not permitted unless an XFSC equivalent is demonstrably unsuitable. The goal is to maximize
  reuse of established and standards-compliant ecosystem services.
effective_requirement: |
  Use XFSC components only where stable APIs and deployed components exist. For unavailable or
  unstable XFSC components, implement a decoupled adapter or local substitute that keeps a later
  XFSC binding possible.
effective_acceptance_criteria: |
  DCS exposes an adapter boundary for the Federated Catalogue/related XFSC integrations; current
  implementation works without undeployed OCM-W/PCM components; integration points are
  documented for later re-alignment.
implementation_decision: |
  Re-align API structure for the Federated Catalogue and avoid direct coupling to unstable
  Catalogue APIs.
constraint_note: |
  catalogue API changed again; OCM-W stack and PCM stack is not deployed

### DCS-PC-02 - Legal Contract Definition
id: DCS-PC-02
area: System Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The DCS may only handle contracts that meet EU legal validity requirements. In the initial
  scope, this includes contracts signed using Advanced Electronic Signatures (AES) for natural
  persons, with optional support for Qualified Electronic Signatures (QES) or organizational
  seals in future releases. Contract forms requiring legal
  processes outside these signature methods are excluded unless explicitly defined in future
  specifications.
effective_requirement: |
  The DCS may only handle contracts that meet EU legal validity requirements. In the initial
  scope, this includes contracts signed using Advanced Electronic Signatures (AES) for natural
  persons, with optional support for Qualified Electronic Signatures (QES) or organizational
  seals in future releases. Contract forms requiring legal
  processes outside these signature methods are excluded unless explicitly defined in future
  specifications.
context_note: |
  untested with real PKI

### DCS-PC-03 - Technology Stack Preferences
id: DCS-PC-03
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The preferred implementation language is Go (Golang) due to its suitability for confidential
  computing and compatibility with XFSC architecture. APIs SHOULD be designed using the Goa
  design-first framework with Go-based code generation. Java MAY be used only if justified
  (e.g., in modules that use or extend the Catalogue) and if it does not conflict with Goa-based
  code generation. Recommended frameworks and tools include Goa for REST APIs, NATS for
  eventing, Docker containers, and databases with PostgreSQL-equivalent functionality (database
  vendor MUST remain abstracted).
effective_requirement: |
  The preferred implementation language is Go (Golang) due to its suitability for confidential
  computing and compatibility with XFSC architecture. APIs SHOULD be designed using the Goa
  design-first framework with Go-based code generation. Java MAY be used only if justified
  (e.g., in modules that use or extend the Catalogue) and if it does not conflict with Goa-based
  code generation. Recommended frameworks and tools include Goa for REST APIs, NATS for
  eventing, Docker containers, and databases with PostgreSQL-equivalent functionality (database
  vendor MUST remain abstracted).

### DCS-PC-04 - Deployment Environment
id: DCS-PC-04
area: System Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST support deployment on Kubernetes clusters, with deployment configurations
  provided via Helm charts and ArgoCD. Docker Compose deployments are not acceptable for
  demonstration purposes. Continuous Integration/Continuous Deployment (CI/CD) pipelines MUST be
  implemented using GitHub Actions or equivalent services.
effective_requirement: |
  The DCS MUST support deployment on Kubernetes clusters, with deployment configurations
  provided via Helm charts and ArgoCD. Docker Compose deployments are not acceptable for
  demonstration purposes. Continuous Integration/Continuous Deployment (CI/CD) pipelines MUST be
  implemented using GitHub Actions or equivalent services.
context_note: |
  prod undeployed

### DCS-PC-05 - Ecosystem Constraints for Identity Wallets and TSP Services
id: DCS-PC-05
area: System Requirements
implementation_status: Ongoing
interpretation_status: Adjusted Alternative Implementation
source_requirement: |
  The DCS MUST integrate with the existing identity wallet infrastructures (e.g., Animo
  solutions framework for XFSC) and Trust Service Providers (TSPs) for the provisioning of AES
  and, optionally, QES or organizational seals. These services are provided externally and will
  not be developed or operated within DCS. Version 1 will focus exclusively on AES for
  natural-person signatures, with QES/seal support planned for future iterations.
effective_requirement: |
  Prepare wallet and TSP integration through replaceable interfaces. Productive integration with
  private wallets or TSPs is required only after signing architecture and wallet infrastructure
  are clarified.
effective_acceptance_criteria: |
  Signing and credential flows can run through a replaceable wallet/signing adapter;
  private-wallet support is not required until the target wallet infrastructure and signing mode
  are clarified.
implementation_decision: |
  Clarify signing architecture; API signing may exclude private wallets.
constraint_note: |
  depending on Wallet infastructure

### DCS-PC-06 - Template and Data Formats SLA and Data Exchange
id: DCS-PC-06
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract templates MUST be supported in machine-readable formats with semantic definitions.
  JSON-LD is the preferred format for representing these templates to ensure semantic
  interoperability across systems.
effective_requirement: |
  Contract templates MUST be supported in machine-readable formats with semantic definitions.
  JSON-LD is the preferred format for representing these templates to ensure semantic
  interoperability across systems.

### DCS-OE-01 - Kubernetes Environment
id: DCS-OE-01
area: System Requirements
implementation_status: Ongoing
category: Operating Environment
interpretation_status: Unchanged
source_requirement: |
  The product MUST be operable on standard Kubernetes-based environments without any hardware
  restrictions. The reference environment for demonstration and development purposes MUST be
  deployed
  on IONOS Kubernetes as well as T-Systems Open Sovereign Cloud (OSC).
effective_requirement: |
  The product MUST be operable on standard Kubernetes-based environments without any hardware
  restrictions. The reference environment for demonstration and development purposes MUST be
  deployed
  on IONOS Kubernetes as well as T-Systems Open Sovereign Cloud (OSC).
context_note: |
  prod undeployed

### DCS-OE-02 - Containerization
id: DCS-OE-02
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  All software components MUST be delivered as Linux-based Docker containers for deployment in
  Kubernetes environments. Docker Compose deployments are NOT acceptable for demonstration
  purposes.
effective_requirement: |
  All software components MUST be delivered as Linux-based Docker containers for deployment in
  Kubernetes environments. Docker Compose deployments are NOT acceptable for demonstration
  purposes.

### DCS-OE-03 - Deployment Tooling
id: DCS-OE-03
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Deployment configurations MUST be provided as Helm charts, and GitOps-based delivery MUST be
  supported via ArgoCD.
effective_requirement: |
  Deployment configurations MUST be provided as Helm charts, and GitOps-based delivery MUST be
  supported via ArgoCD.

### DCS-OE-04 - CI/CD Pipelines
id: DCS-OE-04
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Continuous Integration/Continuous Deployment (CI/CD) MUST be implemented using GitHub Actions
  or an equivalent platform.
effective_requirement: |
  Continuous Integration/Continuous Deployment (CI/CD) MUST be implemented using GitHub Actions
  or an equivalent platform.

### DCS-OE-05 - Database Support
id: DCS-OE-05
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The product MUST support PostgreSQL or databases with equivalent functionality. No hard
  dependency on a specific vendor is permitted.
effective_requirement: |
  The product MUST support PostgreSQL or databases with equivalent functionality. No hard
  dependency on a specific vendor is permitted.

### DCS-OE-06 - Ecosystem Integrations
id: DCS-OE-06
area: System Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The product MUST integrate with:
  - XFSC components (Catalogue, Orchestration Engine, Revocation List).
  - Identity wallet infrastructures for authentication and authorization. TSPs for AES and
  optional QES/seals.
  - External systems via RESTful APIs for contract automation and lifecycle integration
effective_requirement: |
  The product MUST integrate with:
  - XFSC components (Catalogue, Orchestration Engine, Revocation List).
  - Identity wallet infrastructures for authentication and authorization. TSPs for AES and
  optional QES/seals.
  - External systems via RESTful APIs for contract automation and lifecycle integration



## Functional Requirements

### DCS-FR-TR-01 - Machine-Readable Format
id: DCS-FR-TR-01
area: Functional Requirements
implementation_status: Done
category: Template Repository
interpretation_status: Unchanged
source_requirement: |
  The Template Repository facilitates the structured storage and management of contract
  templates in machine readable source forms. Ensuring contract templates in machine-readable
  forms allows for automation and validation across digital contracting systems. Thus, the
  repository MUST store contract templates in structured, machine-readable formats. This does
  not indicate that the repository needs to be open to arbitrary contract languages.
effective_requirement: |
  The Template Repository facilitates the structured storage and management of contract
  templates in machine readable source forms. Ensuring contract templates in machine-readable
  forms allows for automation and validation across digital contracting systems. Thus, the
  repository MUST store contract templates in structured, machine-readable formats. This does
  not indicate that the repository needs to be open to arbitrary contract languages.

### DCS-FR-TR-02 - Multi-Tiered Contract Template Management
id: DCS-FR-TR-02
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST support hierarchical contract structures, including Frame
  Agreements, Sub Agreements, and Addendums as digital contracts often involve multiple levels
  of agreements, requiring structured template management that allows customization at different
  levels. Examples include contract schemas, credential schemas, provenance schemas, and trust
  chaining schemas.
effective_requirement: |
  The Template Repository MUST support hierarchical contract structures, including Frame
  Agreements, Sub Agreements, and Addendums as digital contracts often involve multiple levels
  of agreements, requiring structured template management that allows customization at different
  levels. Examples include contract schemas, credential schemas, provenance schemas, and trust
  chaining schemas.
context_note: |
  simplistic implementation

### DCS-FR-TR-03 - Semantic Hub for Schema Storage
id: DCS-FR-TR-03
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The Semantic Hub facilitates standardization and interoperability across different contract
  templates by providing a structured schema repository for validation and compliance. For this
  reason, the repository MUST include a Semantic Hub to store schemas supporting
  machine-readable formats, such as SHACL shapes and JSON-LD contexts, ensuring interoperability
  and extensibility. The Semantic Hub MUST also support versioning of the schemas.
effective_requirement: |
  The Semantic Hub facilitates standardization and interoperability across different contract
  templates by providing a structured schema repository for validation and compliance. For this
  reason, the repository MUST include a Semantic Hub to store schemas supporting
  machine-readable formats, such as SHACL shapes and JSON-LD contexts, ensuring interoperability
  and extensibility. The Semantic Hub MUST also support versioning of the schemas.

### DCS-FR-TR-04 - Machine-Readable and Human-Readable Template Linking
id: DCS-FR-TR-04
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  All templates MUST have a bidirectional link between machine-readable and human-readable
  formats.
effective_requirement: |
  All templates MUST have a bidirectional link between machine-readable and human-readable
  formats.

### DCS-FR-TR-05 - Template Version Control
id: DCS-FR-TR-05
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST track changes, versions, and approvals for each template. These
  tracked changes MUST be made available for logging, monitoring, auditing, and reporting
  purposes to ensure compliance, transparency, and dispute resolution among evolving contract
  templates. The versions, changes, and approvals can be logged into an external system.
effective_requirement: |
  The Template Repository MUST track changes, versions, and approvals for each template. These
  tracked changes MUST be made available for logging, monitoring, auditing, and reporting
  purposes to ensure compliance, transparency, and dispute resolution among evolving contract
  templates. The versions, changes, and approvals can be logged into an external system.

### DCS-FR-TR-06 - Role-Based Access Control for Template Repository
id: DCS-FR-TR-06
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system must implement RBAC to restrict template access and modifications according to user
  roles, ensuring that only authorized personnel can create, approve, or retrieve templates.
  RBAC will be managed through role assertions in the form of verifiable credentials stored in
  users' wallets, which will be presented during the authentication and authorization process.
  This approach prevents unauthorized alterations that could lead to errors, compliance risks,
  or unintended contract modifications. Specifically, it ensures that only Template Managers can
  create templates, Template Approvers can approve them, Template Reviewers can review them, and
  Template Creators can initiate their creation
effective_requirement: |
  The system must implement RBAC to restrict template access and modifications according to user
  roles, ensuring that only authorized personnel can create, approve, or retrieve templates.
  RBAC will be managed through role assertions in the form of verifiable credentials stored in
  users' wallets, which will be presented during the authentication and authorization process.
  This approach prevents unauthorized alterations that could lead to errors, compliance risks,
  or unintended contract modifications. Specifically, it ensures that only Template Managers can
  create templates, Template Approvers can approve them, Template Reviewers can review them, and
  Template Creators can initiate their creation

### DCS-FR-TR-07 - Compliance & Legal Validation
id: DCS-FR-TR-07
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST validate templates against regulatory frameworks before they can
  be used in contract creation, with the specific frameworks depending on the use case domain.
  This validation ensures that contract templates meet legal and compliance requirements before
  they are applied in digital agreements, adhering to the relevant regulations for each domain.
  The validation occurs over the process audit and compliance component of the Digital
  Contracting Service, which provides access to a user with the Compliance Officer role over the
  non-compliance investigation tool.
effective_requirement: |
  The Template Repository MUST validate templates against regulatory frameworks before they can
  be used in contract creation, with the specific frameworks depending on the use case domain.
  This validation ensures that contract templates meet legal and compliance requirements before
  they are applied in digital agreements, adhering to the relevant regulations for each domain.
  The validation occurs over the process audit and compliance component of the Digital
  Contracting Service, which provides access to a user with the Compliance Officer role over the
  non-compliance investigation tool.

### DCS-FR-TR-08 - Provenance Tracking
id: DCS-FR-TR-08
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The repository MUST maintain provenance tracking for all contract templates, recording their
  creation, modifications, approvals, and historical versions to prevent unauthorized
  modifications and enabling verification of the contract's legal history. "Provenance" means
  being able to track who created, modified, reviewed, or approved a template at each step with
  logged data. It MUST be ensured that each role - template creator, template reviewer, template
  approver, and template manager - leaves a traceable record when interacting with the template.
effective_requirement: |
  The repository MUST maintain provenance tracking for all contract templates, recording their
  creation, modifications, approvals, and historical versions to prevent unauthorized
  modifications and enabling verification of the contract's legal history. "Provenance" means
  being able to track who created, modified, reviewed, or approved a template at each step with
  logged data. It MUST be ensured that each role - template creator, template reviewer, template
  approver, and template manager - leaves a traceable record when interacting with the template.

### DCS-FR-TR-09 - Template Provenance and Versioning
id: DCS-FR-TR-09
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The repository MUST support the ability for a Template Creator, Reviewer, and Approver to add
  provenance claims to each template version. Provenance and versioning assertions MUST be
  linked and MUST be verifiable using W3C VCs in JSON-LD. Template users MUST be able to verify
  the provenance of a given template by verifying the template provenance credentials.
effective_requirement: |
  The repository MUST support the ability for a Template Creator, Reviewer, and Approver to add
  provenance claims to each template version. Provenance and versioning assertions MUST be
  linked and MUST be verifiable using W3C VCs in JSON-LD. Template users MUST be able to verify
  the provenance of a given template by verifying the template provenance credentials.

### DCS-FR-TR-10 - Searchable Metadata & Categorization
id: DCS-FR-TR-10
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  In order to enable organizations to find appropriate templates, the repository MUST allow
  advanced searching based on metadata such as contract type, jurisdiction, industry, and
  regulatory compliance status.
effective_requirement: |
  In order to enable organizations to find appropriate templates, the repository MUST allow
  advanced searching based on metadata such as contract type, jurisdiction, industry, and
  regulatory compliance status.

### DCS-FR-TR-11 - Template UUID / DID Assignment
id: DCS-FR-TR-11
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Each contract template MUST have a UUID or DID to ensure its traceability across contract
  workflows, providing consistency and authenticity when templates are referenced in
  negotiations, agreements, and compliance audits.
effective_requirement: |
  Each contract template MUST have a UUID or DID to ensure its traceability across contract
  workflows, providing consistency and authenticity when templates are referenced in
  negotiations, agreements, and compliance audits.

### DCS-FR-TR-12 - Template Customization
id: DCS-FR-TR-12
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The repository SHOULD support dynamic placeholders and automated population of contract terms
  based on predefined SLA rules, reducing manual input errors and enhancing efficiency by
  allowing templates to automatically adjust to specific service agreements.
effective_requirement: |
  The repository SHOULD support dynamic placeholders and automated population of contract terms
  based on predefined SLA rules, reducing manual input errors and enhancing efficiency by
  allowing templates to automatically adjust to specific service agreements.
context_note: |
  ontology ongoing

### DCS-FR-TR-13 - Template Creation
id: DCS-FR-TR-13
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow a Template Manager to register a new contract template via the
  /template/register endpoint to ensure that only authorized personnel can create and manage
  contract templates.
effective_requirement: |
  The system MUST allow a Template Manager to register a new contract template via the
  /template/register endpoint to ensure that only authorized personnel can create and manage
  contract templates.

### DCS-FR-TR-14 - Template Submission for Approval
id: DCS-FR-TR-14
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST enforce an approval workflow where a Template Manager submits a newly created
  template for review by a Template Approver, ensuring that all templates meet quality and
  compliance standards before being made available for use.
effective_requirement: |
  The system MUST enforce an approval workflow where a Template Manager submits a newly created
  template for review by a Template Approver, ensuring that all templates meet quality and
  compliance standards before being made available for use.

### DCS-FR-TR-15 - Template Approval Process
id: DCS-FR-TR-15
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow a Template Approver to review, approve, or reject a submitted template
  via a dedicated approval interface, preventing unapproved or incorrect templates from being
  used in contract workflows.
effective_requirement: |
  The system MUST allow a Template Approver to review, approve, or reject a submitted template
  via a dedicated approval interface, preventing unapproved or incorrect templates from being
  used in contract workflows.

### DCS-FR-TR-16 - Template Update Management
id: DCS-FR-TR-16
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The repository MUST allow authorized users to update existing contract templates while
  maintaining version history and linking updates to previous versions, ensuring continuity and
  transparency in template evolution by tracking modifications and preserving historical
  versions for auditability. The entire history can be maintained by an external system.
effective_requirement: |
  The repository MUST allow authorized users to update existing contract templates while
  maintaining version history and linking updates to previous versions, ensuring continuity and
  transparency in template evolution by tracking modifications and preserving historical
  versions for auditability. The entire history can be maintained by an external system.

### DCS-FR-TR-17 - Template Retirement and Deprecation
id: DCS-FR-TR-17
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The repository MUST support the deprecation and retirement of contract templates, ensuring
  that outdated versions are archived and no longer used in new contracts, while maintaining
  compliance and contract integrity through the preservation of historical records for reference
  and audits.
effective_requirement: |
  The repository MUST support the deprecation and retirement of contract templates, ensuring
  that outdated versions are archived and no longer used in new contracts, while maintaining
  compliance and contract integrity through the preservation of historical records for reference
  and audits.

### DCS-FR-TR-18 - Template Deletion
id: DCS-FR-TR-18
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow a Template Manager to delete a deprecated template, preventing outdated
  templates from being used. The deletion process may be subject to specific compliance
  requirements that are out of scope.
effective_requirement: |
  The system MUST allow a Template Manager to delete a deprecated template, preventing outdated
  templates from being used. The deletion process may be subject to specific compliance
  requirements that are out of scope.

### DCS-FR-TR-19 - Template Retrieval
id: DCS-FR-TR-19
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow a Template User to retrieve an approved template via the
  /template/retrieve/{template_id} endpoint to ensure that only approved and active templates
  are used in contract workflows.
effective_requirement: |
  The system MUST allow a Template User to retrieve an approved template via the
  /template/retrieve/{template_id} endpoint to ensure that only approved and active templates
  are used in contract workflows.

### DCS-FR-TR-20 - Template Compliance and Integrity Verification
id: DCS-FR-TR-20
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide an endpoint (/template/verify/{template_id}) that enables Template
  Users to verify the integrity and compliance of a retrieved template, ensuring that it adheres
  to regulatory standards and has not been altered since approval.
effective_requirement: |
  The system MUST provide an endpoint (/template/verify/{template_id}) that enables Template
  Users to verify the integrity and compliance of a retrieved template, ensuring that it adheres
  to regulatory standards and has not been altered since approval.

### DCS-FR-TR-21 - Audit Logs for Template Changes
id: DCS-FR-TR-21
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST maintain an audit log that records all template creation, modification,
  approval, and deletion activities, providing transparency and accountability by tracking
  template-related actions.
effective_requirement: |
  The system MUST maintain an audit log that records all template creation, modification,
  approval, and deletion activities, providing transparency and accountability by tracking
  template-related actions.

### DCS-FR-TR-22 - Notification System for Template Updates
id: DCS-FR-TR-22
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system SHOULD notify Template Users when a contract template they have used has been
  updated or deprecated, ensuring they work with the latest approved templates and are aware of
  changes that may impact ongoing contract workflows.
effective_requirement: |
  The system SHOULD notify Template Users when a contract template they have used has been
  updated or deprecated, ensuring they work with the latest approved templates and are aware of
  changes that may impact ongoing contract workflows.

### DCS-FR-TR-23 - Structural Dependency Mapping
id: DCS-FR-TR-23
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST allow Template Managers to define and enforce dependencies
  between frame contracts, contracts, contract components, and appendices, preventing
  misconfiguration of multi-part templates and ensuring structural consistency at runtime.
effective_requirement: |
  The Template Repository MUST allow Template Managers to define and enforce dependencies
  between frame contracts, contracts, contract components, and appendices, preventing
  misconfiguration of multi-part templates and ensuring structural consistency at runtime.
context_note: |
  simplistic implementation

### DCS-FR-TR-24 - Structural Export in Unified Format
id: DCS-FR-TR-24
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST support export of a fully assembled contract structure (main
  contract, appendices, components) into a bundled format, enabling contract preview,
  documentation, and offline review by bundling all elements together.
effective_requirement: |
  The Template Repository MUST support export of a fully assembled contract structure (main
  contract, appendices, components) into a bundled format, enabling contract preview,
  documentation, and offline review by bundling all elements together.
context_note: |
  pdf renderer needs to be aligned to JSON-LD

### DCS-FR-TR-25 - Multi-Contract Template Builder
id: DCS-FR-TR-25
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST provide a visual interface for Template Managers to compose
  contract templates with nested contracts, appendices, and components, enhancing usability,
  reducing errors, and simplifying the management of complex contract structures.
effective_requirement: |
  The Template Repository MUST provide a visual interface for Template Managers to compose
  contract templates with nested contracts, appendices, and components, enhancing usability,
  reducing errors, and simplifying the management of complex contract structures.

### DCS-FR-TR-26 - Logical Validation of Structural Dependencies
id: DCS-FR-TR-26
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST check for consistency and validity of logical dependencies before usage to
  prevent invalid combinations of template components and enforces business rules
effective_requirement: |
  The system MUST check for consistency and validity of logical dependencies before usage to
  prevent invalid combinations of template components and enforces business rules

### DCS-FR-TR-27 - Contract Type Classification
id: DCS-FR-TR-27
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The Template Repository MUST support defining and assigning structured single- and multi-party
  contract types for standardized filtering and governance.
effective_requirement: |
  The Template Repository MUST support defining and assigning structured single- and multi-party
  contract types for standardized filtering and governance.

### DCS-FR-TR-28 - Template Management Dashboard (see Section 3.1)
id: DCS-FR-TR-28
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide a Template Management Dashboard that enables the Template Manager to
  manage and oversee the entire contract template lifecycle. This dashboard is used to manage
  templates through an interface for tracking, modifying, and approving templates efficiently.
effective_requirement: |
  The system MUST provide a Template Management Dashboard that enables the Template Manager to
  manage and oversee the entire contract template lifecycle. This dashboard is used to manage
  templates through an interface for tracking, modifying, and approving templates efficiently.

### DCS-FR-CWE-01 - Multi-Party Contract Management
id: DCS-FR-CWE-01
area: Functional Requirements
implementation_status: Ongoing
category: Contract Workflow Engine
interpretation_status: Unchanged
source_requirement: |
  The system MUST support workflows that involve multiple parties in a single contract or
  contract package. Each party MUST be able to independently sign, approve, or review their
  section in accordance with the predefined process.
effective_requirement: |
  The system MUST support workflows that involve multiple parties in a single contract or
  contract package. Each party MUST be able to independently sign, approve, or review their
  section in accordance with the predefined process.
context_note: |
  signing is still open

### DCS-FR-CWE-02 - Hierarchical Contract Structures
id: DCS-FR-CWE-02
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST support hierarchical structures such as master agreements, sub-agreements, and
  annexes. These MUST be logically linked and version-controlled for consistent contract
  management.
effective_requirement: |
  The system MUST support hierarchical structures such as master agreements, sub-agreements, and
  annexes. These MUST be logically linked and version-controlled for consistent contract
  management.
context_note: |
  simplistic implementation

### DCS-FR-CWE-03 - Contract Assembling
id: DCS-FR-CWE-03
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST support dynamic contract assembling from reusable clauses and templates. The
  assembly process MUST validate structure, required metadata, and content logic.
effective_requirement: |
  The system MUST support dynamic contract assembling from reusable clauses and templates. The
  assembly process MUST validate structure, required metadata, and content logic.
context_note: |
  ontology ongoing

### DCS-FR-CWE-04 - Machine-Readable & Human-Readable Contract Synchronization
id: DCS-FR-CWE-04
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST ensure synchronization between machine-readable and human-readable versions of
  a contract. Both formats MUST be derived from the same source and have matching content
  hashes.
effective_requirement: |
  The system MUST ensure synchronization between machine-readable and human-readable versions of
  a contract. Both formats MUST be derived from the same source and have matching content
  hashes.
context_note: |
  deterministic pdf renderer embeds payload hash

### DCS-FR-CWE-06 - Event-Driven Contract Execution
id: DCS-FR-CWE-06
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST support event-based workflows where contract execution steps are triggered by
  specific lifecycle events (e.g., all parties signed, approval completed). Events MUST be
  logged.
effective_requirement: |
  The system MUST support event-based workflows where contract execution steps are triggered by
  specific lifecycle events (e.g., all parties signed, approval completed). Events MUST be
  logged.

### DCS-FR-CWE-07 - Role-Based Access Control
id: DCS-FR-CWE-07
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST enforce access to contract workflows based on roles (e.g., Reviewer, Signer,
  Manager). Role assignments MUST be verifiable via credentials and restrict unauthorized
  actions.
effective_requirement: |
  The system MUST enforce access to contract workflows based on roles (e.g., Reviewer, Signer,
  Manager). Role assignments MUST be verifiable via credentials and restrict unauthorized
  actions.

### DCS-FR-CWE-08 - Version Control
id: DCS-FR-CWE-08
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Each contract instance MUST maintain version history. Edits MUST generate a new version with
  timestamps and user attribution. Old versions MUST remain accessible for audit and rollback.
effective_requirement: |
  Each contract instance MUST maintain version history. Edits MUST generate a new version with
  timestamps and user attribution. Old versions MUST remain accessible for audit and rollback.

### DCS-FR-CWE-09 - SLA & Compliance Monitoring
id: DCS-FR-CWE-09
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST continuously monitor contractual obligations (e.g., service levels, deadlines)
  and flag SLA violations. Compliance rules MUST be enforced throughout the contract lifecycle.
effective_requirement: |
  The system MUST continuously monitor contractual obligations (e.g., service levels, deadlines)
  and flag SLA violations. Compliance rules MUST be enforced throughout the contract lifecycle.
context_note: |
  ontology ongoing

### DCS-FR-CWE-10 - Contract Expiry
id: DCS-FR-CWE-10
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST automatically detect and flag contracts approaching expiration. It MUST
  trigger alerts for renewal, termination, or archiving workflows depending on configured
  policies.
effective_requirement: |
  The system MUST automatically detect and flag contracts approaching expiration. It MUST
  trigger alerts for renewal, termination, or archiving workflows depending on configured
  policies.

### DCS-FR-CWE-11 - Contract Renewal
id: DCS-FR-CWE-11
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Authorized users MUST be able to renew contracts while retaining linked metadata and
  signatures from the previous version. Renewals MUST generate a new contract instance with
  reference links.
effective_requirement: |
  Authorized users MUST be able to renew contracts while retaining linked metadata and
  signatures from the previous version. Renewals MUST generate a new contract instance with
  reference links.

### DCS-FR-CWE-12 - Termination Handling
id: DCS-FR-CWE-12
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST support formal contract termination, including capturing the reason, author,
  timestamp, and preserving the contract status for compliance and dispute resolution.
effective_requirement: |
  The system MUST support formal contract termination, including capturing the reason, author,
  timestamp, and preserving the contract status for compliance and dispute resolution.

### DCS-FR-CWE-13 - Contract Creation
id: DCS-FR-CWE-13
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow the creation of new contracts from templates, auto-filling metadata such
  as parties, jurisdiction, and applicable schemas. Drafts MUST be editable and versioned.
effective_requirement: |
  The system MUST allow the creation of new contracts from templates, auto-filling metadata such
  as parties, jurisdiction, and applicable schemas. Drafts MUST be editable and versioned.
context_note: |
  ontology ongoing

### DCS-FR-CWE-14 - Contract Submission for Review
id: DCS-FR-CWE-14
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Once created, contracts MUST be submitted to assigned reviewers for validation. The system
  MUST support approval routing, tracking of reviewer input, and status change upon submission.
effective_requirement: |
  Once created, contracts MUST be submitted to assigned reviewers for validation. The system
  MUST support approval routing, tracking of reviewer input, and status change upon submission.

### DCS-FR-CWE-15 - Contract Review and Approval
id: DCS-FR-CWE-15
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Designated reviewers MUST be able to approve or reject contracts. All actions MUST be logged
  with comments, digital credentials, and time of action. Rejection MUST return the contract to
  draft status.
effective_requirement: |
  Designated reviewers MUST be able to approve or reject contracts. All actions MUST be logged
  with comments, digital credentials, and time of action. Rejection MUST return the contract to
  draft status.

### DCS-FR-CWE-16 - Contract Initiation
id: DCS-FR-CWE-16
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Upon approval, the contract MUST be marked as ready for execution. The system MUST transition
  it into the signing phase or deploy it to external systems, depending on configuration.
effective_requirement: |
  Upon approval, the contract MUST be marked as ready for execution. The system MUST transition
  it into the signing phase or deploy it to external systems, depending on configuration.
context_note: |
  external deployment pending

### DCS-FR-CWE-17 - Contract Review
id: DCS-FR-CWE-17
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST support detailed contract review, including side-by-side comparison of
  versions, redlining, and automated checks for missing fields or inconsistencies.
effective_requirement: |
  The system MUST support detailed contract review, including side-by-side comparison of
  versions, redlining, and automated checks for missing fields or inconsistencies.
context_note: |
  machine readable form pending

### DCS-FR-CWE-18 - Contract Negotiation
id: DCS-FR-CWE-18
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST enable parties to engage in structured negotiation workflows. This includes
  comment threads, redline proposals, approvals of changes, and full negotiation audit logs.
effective_requirement: |
  The system MUST enable parties to engage in structured negotiation workflows. This includes
  comment threads, redline proposals, approvals of changes, and full negotiation audit logs.

### DCS-FR-CWE-19 - Contract Signing
id: DCS-FR-CWE-19
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Once approved, the system MUST initiate the signing process. Each party MUST be guided through
  their required steps, with signature validity and completion tracked in real time.
effective_requirement: |
  Once approved, the system MUST initiate the signing process. Each party MUST be guided through
  their required steps, with signature validity and completion tracked in real time.

### DCS-FR-CWE-20 - Store Contract in Archive
id: DCS-FR-CWE-20
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Signed contracts MUST be stored in the contract archive, along with signature metadata,
  version history, and credential hashes. Archived contracts MUST be immutable and auditable.
effective_requirement: |
  Signed contracts MUST be stored in the contract archive, along with signature metadata,
  version history, and credential hashes. Archived contracts MUST be immutable and auditable.
context_note: |
  all stored on IPFS immutably, provenance within pdf

### DCS-FR-CWE-21 - Retrieve Contract from Archive
id: DCS-FR-CWE-21
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Authorized users MUST be able to retrieve archived contracts based on filters such as contract
  ID, status, or participant. Retrieval MUST maintain audit trail and access control.
effective_requirement: |
  Authorized users MUST be able to retrieve archived contracts based on filters such as contract
  ID, status, or participant. Retrieval MUST maintain audit trail and access control.
context_note: |
  participant pending

### DCS-FR-CWE-22 - Contract Renewal Management
id: DCS-FR-CWE-22
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide a dedicated renewal workflow, including template reuse, automatic
  metadata carryover, and notification to involved parties about renewal deadlines.
effective_requirement: |
  The system MUST provide a dedicated renewal workflow, including template reuse, automatic
  metadata carryover, and notification to involved parties about renewal deadlines.

### DCS-FR-CWE-23 - Contract Termination
id: DCS-FR-CWE-23
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Contract Managers MUST be able to formally terminate contracts using a termination interface
  or API. Terminated contracts MUST be marked accordingly and removed from active workflows.
effective_requirement: |
  Contract Managers MUST be able to formally terminate contracts using a termination interface
  or API. Terminated contracts MUST be marked accordingly and removed from active workflows.

### DCS-FR-CWE-24 - Contract Management Dashboard
id: DCS-FR-CWE-24
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  A visual dashboard MUST be available to manage and track contract progress, status,
  responsibilities, and deadlines. The dashboard MUST support filtering, bulk actions, and live
  updates.
effective_requirement: |
  A visual dashboard MUST be available to manage and track contract progress, status,
  responsibilities, and deadlines. The dashboard MUST support filtering, bulk actions, and live
  updates.
context_note: |
  contract KPIs / contract execution environment

### DCS-FR-CWE-25 - Contract Review and Approval Interface
id: DCS-FR-CWE-25
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  A dedicated interface MUST allow reviewers to access, validate, and comment on contracts
  awaiting approval. The interface MUST support highlighting, approval workflows, and role
  attribution.
effective_requirement: |
  A dedicated interface MUST allow reviewers to access, validate, and comment on contracts
  awaiting approval. The interface MUST support highlighting, approval workflows, and role
  attribution.

### DCS-FR-CWE-26 - Contract Signing Interfac
id: DCS-FR-CWE-26
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide a secure signing interface where authorized signers can apply legally
  valid signatures. The interface MUST support wallet integration and display signer tasks.
effective_requirement: |
  The system MUST provide a secure signing interface where authorized signers can apply legally
  valid signatures. The interface MUST support wallet integration and display signer tasks.
context_note: |
  wallet integration

### DCS-FR-CWE-27 - Contract Tracking and Status Overview
id: DCS-FR-CWE-27
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST display real-time status of contracts, including which stage they are in
  (draft, review, signing, active, expired, etc.), with timestamps and action history.
effective_requirement: |
  The system MUST display real-time status of contracts, including which stage they are in
  (draft, review, signing, active, expired, etc.), with timestamps and action history.

### DCS-FR-CWE-28 - Automated Contract Interaction via API
id: DCS-FR-CWE-28
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST offer API endpoints for external systems to create, update, or query
  contracts. API interactions MUST enforce authentication, rate limits, and action validation.
effective_requirement: |
  The system MUST offer API endpoints for external systems to create, update, or query
  contracts. API interactions MUST enforce authentication, rate limits, and action validation.

### DCS-FR-CWE-29 - Multi-Contract Visualization
id: DCS-FR-CWE-29
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST enable visual composition and management of complex contract packages,
  including subcontracts, annexes, and related documents. The visualization MUST show hierarchy
  and dependency links.
effective_requirement: |
  The system MUST enable visual composition and management of complex contract packages,
  including subcontracts, annexes, and related documents. The visualization MUST show hierarchy
  and dependency links.

### DCS-FR-CWE-30 - Contract Package Bundling
id: DCS-FR-CWE-30
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow bundling of multiple related contracts into a single distributable
  package. Each package MUST maintain internal references, shared metadata, and signature
  states.
effective_requirement: |
  The system MUST allow bundling of multiple related contracts into a single distributable
  package. Each package MUST maintain internal references, shared metadata, and signature
  states.

### DCS-FR-CWE-31 - Contract Performance Tracking
id: DCS-FR-CWE-31
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST track key performance indicators defined in the contract such as delivery
  timelines, milestones, and financial terms. Alerts MUST be raised for underperformance or
  missed targets.
effective_requirement: |
  The system MUST track key performance indicators defined in the contract such as delivery
  timelines, milestones, and financial terms. Alerts MUST be raised for underperformance or
  missed targets.

### DCS-FR-SM-01 - Level of Assurance Flexibility for Simple Electronic Signature, Advanced Electronic Signature, and Qualified Electronic Signature
id: DCS-FR-SM-01
area: Functional Requirements
implementation_status: Not Started
category: Signature Management
interpretation_status: Unchanged
source_requirement: |
  The system MUST support flexible signature levels in accordance with the eIDAS Regulation,
  including SES, AES, and QES. Each level MUST be selectable based on contract requirements and
  risk profiles. This ensures compatibility with diverse signing needs while maintaining
  compliance.
effective_requirement: |
  The system MUST support flexible signature levels in accordance with the eIDAS Regulation,
  including SES, AES, and QES. Each level MUST be selectable based on contract requirements and
  risk profiles. This ensures compatibility with diverse signing needs while maintaining
  compliance.

### DCS-FR-SM-02 - Support for PAdES, JAdES, and CAdES Signatures
id: DCS-FR-SM-02
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST support multiple advanced digital signature formats, including PAdES for PDFs,
  JAdES for JSON-based data, and CAdES for CMS structures. These formats MUST be available for
  respective contract representations to support cross-system interoperability and legal
  recognition.
effective_requirement: |
  The system MUST support multiple advanced digital signature formats, including PAdES for PDFs,
  JAdES for JSON-based data, and CAdES for CMS structures. These formats MUST be available for
  respective contract representations to support cross-system interoperability and legal
  recognition.
context_note: |
  pades footwork done

### DCS-FR-SM-03 - Signing Identity and PoA Authorization Credential
id: DCS-FR-SM-03
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Each Contract Signer MUST present a valid identity credential and (if applicable) a PoA
  credential. These credentials MUST be verifiable and issued by recognized authorities. The
  syste
effective_requirement: |
  Each Contract Signer MUST present a valid identity credential and (if applicable) a PoA
  credential. These credentials MUST be verifiable and issued by recognized authorities. The
  syste
context_note: |
  PKI unfinished

### DCS-FR-SM-04 - Counterparty Authorization and PoA Credential Chain Verification
id: DCS-FR-SM-04
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST verify that counterparties possess valid PoA credentials and that the
  delegation chain is valid and traceable. Credential chains MUST be anchored in trusted
  registries or via verifiable credentials to ensure authenticity.
effective_requirement: |
  The system MUST verify that counterparties possess valid PoA credentials and that the
  delegation chain is valid and traceable. Credential chains MUST be anchored in trusted
  registries or via verifiable credentials to ensure authenticity.
context_note: |
  untested against real PKI

### DCS-FR-SM-05 - Integration with Signing Identity and PoA Verifiable Credentials
id: DCS-FR-SM-05
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST integrate with Verifiable Credential frameworks to validate and consume
  identity and PoA credentials. Credentials MUST be issued, presented, and verified in
  compliance with W3C and eIDAScompatible data models
effective_requirement: |
  The system MUST integrate with Verifiable Credential frameworks to validate and consume
  identity and PoA credentials. Credentials MUST be issued, presented, and verified in
  compliance with W3C and eIDAScompatible data models
context_note: |
  sd-jwt service broken, hand rolling within DCS

### DCS-FR-SM-06 - Wallet for Identity, PoA Credential Management, and Signing
id: DCS-FR-SM-06
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Alternative Implementation
source_requirement: |
  Contract Signers MUST be able to manage their identity and PoA credentials within a secure
  digital wallet (e.g., XFSC OCM, Cloud PCM). The wallet MUST support credential presentation
  and digital signature operations.
effective_requirement: |
  DCS does not provide a productive wallet. Use a test wallet or wallet adapter for credential
  presentation and signing flows; verify against OCM/Cloud PCM later.
effective_acceptance_criteria: |
  A signer can present identity/PoA credentials and execute the intended flow through the test
  wallet/adapter; the adapter boundary is compatible with later OCM/Cloud PCM verification.
implementation_decision: |
  Implement with a test wallet; verify with OCM later.
constraint_note: |
  out of scope for DCS, OCM Stack is undeployed, no working cloud PCM available

### DCS-FR-SM-07 - Multi-Signature and Role-Based Signing Flows
id: DCS-FR-SM-07
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The Signature Management system MUST support contracts requiring multiple signatures, where
  each signatory has a distinct role. The system MUST enforce the correct order, dependencies,
  and role-specific conditions within the signing workflow.
effective_requirement: |
  The Signature Management system MUST support contracts requiring multiple signatures, where
  each signatory has a distinct role. The system MUST enforce the correct order, dependencies,
  and role-specific conditions within the signing workflow.
context_note: |
  multiple acroform signatures generated, signature application pending

### DCS-FR-SM-08 - Persisted Contract Signing Summary with Verifiable Credential and PDF/A-3 Embedding
id: DCS-FR-SM-08
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Each signed contract MUST include a persisted signing summary. The summary MUST be available
  as a VC and embedded within the PDF/A-3 document to ensure independent verifiability and
  auditability.
effective_requirement: |
  Each signed contract MUST include a persisted signing summary. The summary MUST be available
  as a VC and embedded within the PDF/A-3 document to ensure independent verifiability and
  auditability.

### DCS-FR-SM-09 - Secure Human-Readable Contract Viewer
id: DCS-FR-SM-09
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide a secure, tamper-proof viewer for human-readable contract content.
  This viewer MUST be used by signers to inspect contract terms before signing and MUST
  guarantee that no modifications can be made during the viewing session.
effective_requirement: |
  The system MUST provide a secure, tamper-proof viewer for human-readable contract content.
  This viewer MUST be used by signers to inspect contract terms before signing and MUST
  guarantee that no modifications can be made during the viewing session.

### DCS-FR-SM-10 - Proof of Contract Execution
id: DCS-FR-SM-10
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  After all required signatures are collected, the system MUST generate cryptographic proof of
  contract execution. This proof MUST include hash references, timestamps, signer identities,
  and status
  confirmations.
effective_requirement: |
  After all required signatures are collected, the system MUST generate cryptographic proof of
  contract execution. This proof MUST include hash references, timestamps, signer identities,
  and status
  confirmations.

### DCS-FR-SM-11 - Linked Machine-Readable and Human-Readable Signatures
id: DCS-FR-SM-11
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Each digital signature MUST be linked to both the machine-readable and human-readable versions
  of the contract. The system MUST ensure that both representations reflect the same content
  hash to guarantee consistency and prevent tampering.
effective_requirement: |
  Each digital signature MUST be linked to both the machine-readable and human-readable versions
  of the contract. The system MUST ensure that both representations reflect the same content
  hash to guarantee consistency and prevent tampering.
context_note: |
  deterministic pdf renderer embeds payload hash, verification endpoint pending

### DCS-FR-SM-12 - Contract Deployment Trigger
id: DCS-FR-SM-12
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Upon completion of the signing process, the system MUST automatically trigger contract
  deployment to connected target systems. The trigger MUST include the signed contract and
  relevant metadata such as hash, version, and timestamp.
effective_requirement: |
  Upon completion of the signing process, the system MUST automatically trigger contract
  deployment to connected target systems. The trigger MUST include the signed contract and
  relevant metadata such as hash, version, and timestamp.

### DCS-FR-SM-13 - Signature Workflow Process
id: DCS-FR-SM-13
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST orchestrate a structured signature workflow, managing signatory assignment,
  status tracking, retries, and completion validation. The workflow MUST enforce order,
  deadlines, and dependencies defined in the contract.
effective_requirement: |
  The system MUST orchestrate a structured signature workflow, managing signatory assignment,
  status tracking, retries, and completion validation. The workflow MUST enforce order,
  deadlines, and dependencies defined in the contract.

### DCS-FR-SM-14 - Signature Request from Signer
id: DCS-FR-SM-14
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow designated signers to request a signature step via wallet, email, or
  integration interfaces. Requests MUST only be valid if the signer's role and authorization are
  verified.
effective_requirement: |
  The system MUST allow designated signers to request a signature step via wallet, email, or
  integration interfaces. Requests MUST only be valid if the signer's role and authorization are
  verified.

### DCS-FR-SM-15 - Contract Retrieval for Signing
id: DCS-FR-SM-15
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow authorized signers to securely retrieve the contract they are asked to
  sign. The retrieval MUST be cryptographically validated and logged with timestamp, signer ID,
  and contract ID.
effective_requirement: |
  The system MUST allow authorized signers to securely retrieve the contract they are asked to
  sign. The retrieval MUST be cryptographically validated and logged with timestamp, signer ID,
  and contract ID.

### DCS-FR-SM-16 - Apply Digital Signature (via Cloud PCM or OCM Signer API Endpoint)
id: DCS-FR-SM-16
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Alternative Implementation With External Dependency
source_requirement: |
  The system MUST allow digital signatures to be applied via an integrated signing service, such
  as Cloud PCM or OCM Signer API. The system MUST ensure secure key usage and enforce signature
  integrity validation upon signing.
effective_requirement: |
  Apply digital signatures through a test wallet or replaceable signing adapter. Cloud PCM or
  OCM Signer API integration is required only when a working target environment and stable
  signer API exist.
effective_acceptance_criteria: |
  The system can create a signature artifact through the adapter and validate signature
  integrity; Cloud PCM/OCM-specific acceptance is blocked until the external API/environment
  exists.
implementation_decision: |
  Implement with a test wallet; verify with OCM later.
constraint_note: |
  OCM Stack is undeployed, no existing testing environment, OCM Signer API non-existent?

### DCS-FR-SM-17 - Multi-Signer Support
id: DCS-FR-SM-17
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The SM module MUST allow multiple users to sign the same contract, either sequentially or in
  parallel, based on the workflow configuration. Each signature MUST be independently
  verifiable.
effective_requirement: |
  The SM module MUST allow multiple users to sign the same contract, either sequentially or in
  parallel, based on the workflow configuration. Each signature MUST be independently
  verifiable.
context_note: |
  only sequentially supported, as parallel breaks PDF/A3

### DCS-FR-SM-18 - Signature Validation
id: DCS-FR-SM-18
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide tools to validate digital signatures applied to a contract, including
  credential status checks, cryptographic integrity, and timestamp verification. Validation
  results MUST be exportable for compliance purposes.
effective_requirement: |
  The system MUST provide tools to validate digital signatures applied to a contract, including
  credential status checks, cryptographic integrity, and timestamp verification. Validation
  results MUST be exportable for compliance purposes.

### DCS-FR-SM-19 - Audit Log for Signatures
id: DCS-FR-SM-19
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  All signature actions MUST be logged in an immutable audit log, capturing signer ID,
  timestamp, credential used, and outcome (success/failure). The log MUST be available to
  auditors and compliance officers.
effective_requirement: |
  All signature actions MUST be logged in an immutable audit log, capturing signer ID,
  timestamp, credential used, and outcome (success/failure). The log MUST be available to
  auditors and compliance officers.

### DCS-FR-SM-20 - Signature Revocation
id: DCS-FR-SM-20
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST support revocation of digital signatures in case of credential invalidation or
  organizational revocation policies. Revocation MUST be logged and MUST invalidate the
  associated contract until re-signing occurs.
effective_requirement: |
  The system MUST support revocation of digital signatures in case of credential invalidation or
  organizational revocation policies. Revocation MUST be logged and MUST invalidate the
  associated contract until re-signing occurs.

### DCS-FR-SM-21 - Signature Compliance Verification
id: DCS-FR-SM-21
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST assess each signature's compliance with legal and organizational signature
  policies, including signature type (QES, AES), credential status, and associated roles. The
  system MUST flag any policy violations.
effective_requirement: |
  The system MUST assess each signature's compliance with legal and organizational signature
  policies, including signature type (QES, AES), credential status, and associated roles. The
  system MUST flag any policy violations.

### DCS-FR-SM-22 - Signature Dashboard for Contract Signers
id: DCS-FR-SM-22
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  A dashboard MUST be available to Contract Signers showing the status of pending, completed,
  and revoked signatures. The dashboard MUST also display associated credentials, timestamps,
  and validation results.
effective_requirement: |
  A dashboard MUST be available to Contract Signers showing the status of pending, completed,
  and revoked signatures. The dashboard MUST also display associated credentials, timestamps,
  and validation results.

### DCS-FR-SM-23 - Signing Interface
id: DCS-FR-SM-23
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide a secure and user-friendly interface for applying digital signatures.
  The interface MUST support wallet-based signing, biometric confirmation (if applicable), and
  real-time validation feedback.
effective_requirement: |
  The system MUST provide a secure and user-friendly interface for applying digital signatures.
  The interface MUST support wallet-based signing, biometric confirmation (if applicable), and
  real-time validation feedback.

### DCS-FR-SM-24 - Signature Status Tracking
id: DCS-FR-SM-24
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow Contract Managers and Signers to track the real-time status of signature
  progress, including pending actions, completion timestamps, and signer acknowledgements.
effective_requirement: |
  The system MUST allow Contract Managers and Signers to track the real-time status of signature
  progress, including pending actions, completion timestamps, and signer acknowledgements.

### DCS-FR-SM-25 - Automated Signature Processing API
id: DCS-FR-SM-25
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide an API for triggering automated signature operations, allowing
  external systems to initiate and complete digital signatures based on pre-authorized
  credentials.
effective_requirement: |
  The system MUST provide an API for triggering automated signature operations, allowing
  external systems to initiate and complete digital signatures based on pre-authorized
  credentials.

### DCS-FR-SM-26 - Signature Compliance Viewer
id: DCS-FR-SM-26
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST offer a viewer that displays signature metadata and compliance status,
  including signer identity, role, credential chain, timestamp, and cryptographic integrity
  proof.
effective_requirement: |
  The system MUST offer a viewer that displays signature metadata and compliance status,
  including signer identity, role, credential chain, timestamp, and cryptographic integrity
  proof.

### DCS-FR-SM-27 - Support for PDF/A Format
id: DCS-FR-SM-27
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST ensure that all signed contracts are exportable in PDF/A format with embedded
  metadata and signature containers. This format MUST comply with long-term archival and
  regulatory requirements.
effective_requirement: |
  The system MUST ensure that all signed contracts are exportable in PDF/A format with embedded
  metadata and signature containers. This format MUST comply with long-term archival and
  regulatory requirements.

### DCS-FR-CSA-01 - Tamper-Proof Contract Storage
id: DCS-FR-CSA-01
area: Functional Requirements
implementation_status: Done
category: Contract Storage and Archive
interpretation_status: Unchanged
source_requirement: |
  The system MUST ensure contracts are stored in a tamper-evident format, using cryptographic
  mechanisms (e.g., hashing) to detect any unauthorized changes after storage. All modifications
  MUST be prohibited or logged with full traceability.
effective_requirement: |
  The system MUST ensure contracts are stored in a tamper-evident format, using cryptographic
  mechanisms (e.g., hashing) to detect any unauthorized changes after storage. All modifications
  MUST be prohibited or logged with full traceability.

### DCS-FR-CSA-02 - Role-Based Access Control
id: DCS-FR-CSA-02
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Access to stored contracts MUST be controlled based on assigned roles. Only authorized roles
  (e.g., Contract Manager, Legal Officer) MAY retrieve, archive, or delete stored contracts.
  Access attempts MUST be audited.
effective_requirement: |
  Access to stored contracts MUST be controlled based on assigned roles. Only authorized roles
  (e.g., Contract Manager, Legal Officer) MAY retrieve, archive, or delete stored contracts.
  Access attempts MUST be audited.

### DCS-FR-CSA-03 - Proof-of-Existence
id: DCS-FR-CSA-03
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST generate a verifiable proof-of-existence for each archived contract. This
  proof MAY include a cryptographic hash, timestamp, and optional anchoring on a distributed
  ledger for independent verification.
effective_requirement: |
  The system MUST generate a verifiable proof-of-existence for each archived contract. This
  proof MAY include a cryptographic hash, timestamp, and optional anchoring on a distributed
  ledger for independent verification.
context_note: |
  anchoring undetermined

### DCS-FR-CSA-04 - Contract Expiry & Renewal Tracking
id: DCS-FR-CSA-04
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST monitor contract expiration timelines and support tracking of renewal status.
  Alerts MUST be generated as contracts approach expiration based on configurable thresholds.
effective_requirement: |
  The system MUST monitor contract expiration timelines and support tracking of renewal status.
  Alerts MUST be generated as contracts approach expiration based on configurable thresholds.

### DCS-FR-CSA-05 - Hierarchical Contract Storage
id: DCS-FR-CSA-05
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Contracts MUST be stored in a structured hierarchy, supporting nesting of frame agreements,
  subcontracts, and appendices. Relationships between contract components MUST be preserved in
  metadata.
effective_requirement: |
  Contracts MUST be stored in a structured hierarchy, supporting nesting of frame agreements,
  subcontracts, and appendices. Relationships between contract components MUST be preserved in
  metadata.

### DCS-FR-CSA-06 - Machine-Readable Contract Storage
id: DCS-FR-CSA-06
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Machine-readable versions of contracts (e.g., JSON-LD, XML) MUST be stored alongside
  human-readable documents. The system MUST validate synchronization between both formats before
  archival.
effective_requirement: |
  Machine-readable versions of contracts (e.g., JSON-LD, XML) MUST be stored alongside
  human-readable documents. The system MUST validate synchronization between both formats before
  archival.
context_note: |
  pdf deterministically re-renderable from machine form

### DCS-FR-CSA-07 - Automated Compliance Checks
id: DCS-FR-CSA-07
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Before contracts are archived, the system MUST perform automated compliance checks based on
  configured business rules or regulations. Non-compliant contracts MUST be flagged for review
  or prevented from storage.
effective_requirement: |
  Before contracts are archived, the system MUST perform automated compliance checks based on
  configured business rules or regulations. Non-compliant contracts MUST be flagged for review
  or prevented from storage.

### DCS-FR-CSA-08 - Store Signed Contract in Archive
id: DCS-FR-CSA-08
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Upon completion of the signature workflow, the system MUST automatically store the finalized
  contract and all signature data in the archive, ensuring document integrity and preservation
  of all verifiable metadata.
effective_requirement: |
  Upon completion of the signature workflow, the system MUST automatically store the finalized
  contract and all signature data in the archive, ensuring document integrity and preservation
  of all verifiable metadata.

### DCS-FR-CSA-09 - Generate and Assign Contract Identifier
id: DCS-FR-CSA-09
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Each archived contract MUST be assigned a globally unique identifier (UUID or DID) for
  referencing across workflows and systems. This ID MUST be persistent and immutable.
effective_requirement: |
  Each archived contract MUST be assigned a globally unique identifier (UUID or DID) for
  referencing across workflows and systems. This ID MUST be persistent and immutable.

### DCS-FR-CSA-10 - Index Contract Metadata
id: DCS-FR-CSA-10
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Archived contracts MUST be indexed with metadata fields such as parties, contract type,
  status, jurisdiction, and validity period. Metadata indexing MUST support efficient searching
  and filtering
effective_requirement: |
  Archived contracts MUST be indexed with metadata fields such as parties, contract type,
  status, jurisdiction, and validity period. Metadata indexing MUST support efficient searching
  and filtering

### DCS-FR-CSA-11 - Create Contract Summary and Tags
id: DCS-FR-CSA-11
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST allow automatic or manual generation of a summary for each archived contract.
  Users MUST also be able to assign tags for thematic categorization and discovery
effective_requirement: |
  The system MUST allow automatic or manual generation of a summary for each archived contract.
  Users MUST also be able to assign tags for thematic categorization and discovery

### DCS-FR-CSA-12 - Retrieve Contract from Archive
id: DCS-FR-CSA-12
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Authorized users MUST be able to retrieve contracts using metadata filters, contract ID, or
  associated tags. Retrieval operations MUST be audited and follow access control policies.
effective_requirement: |
  Authorized users MUST be able to retrieve contracts using metadata filters, contract ID, or
  associated tags. Retrieval operations MUST be audited and follow access control policies.

### DCS-FR-CSA-13 - Search Contracts
id: DCS-FR-CSA-13
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST include a full-text and metadata-based search function. Users MUST be able to
  search by content, participants, dates, tags, and custom fields across all archived contracts.
effective_requirement: |
  The system MUST include a full-text and metadata-based search function. Users MUST be able to
  search by content, participants, dates, tags, and custom fields across all archived contracts.

### DCS-FR-CSA-14 - Contract Expiration Handling
id: DCS-FR-CSA-14
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Expired contracts MUST be flagged in the system and removed from active workflows. The system
  MUST support retention according to configured retention policies and prevent expired contract
  usage.
effective_requirement: |
  Expired contracts MUST be flagged in the system and removed from active workflows. The system
  MUST support retention according to configured retention policies and prevent expired contract
  usage.

### DCS-FR-CSA-15 - Contract Renewal and Extension
id: DCS-FR-CSA-15
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST support creation of renewal or extension contracts linked to archived
  originals. Renewals MUST retain references to the prior contract's version, ID, and
  signatures.
effective_requirement: |
  The system MUST support creation of renewal or extension contracts linked to archived
  originals. Renewals MUST retain references to the prior contract's version, ID, and
  signatures.

### DCS-FR-CSA-16 - Contract Termination
id: DCS-FR-CSA-16
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Terminated contracts MUST be marked as such in the archive with a recorded termination reason,
  effective date, and initiating role. Terminated contracts MUST remain accessible in read-only
  mode.
effective_requirement: |
  Terminated contracts MUST be marked as such in the archive with a recorded termination reason,
  effective date, and initiating role. Terminated contracts MUST remain accessible in read-only
  mode.

### DCS-FR-CSA-17 - Contract Deletion
id: DCS-FR-CSA-17
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  If permitted by policy, authorized users MUST be able to delete archived contracts. Deletion
  operations MUST require justification and MUST be logged with timestamp and user identity
effective_requirement: |
  If permitted by policy, authorized users MUST be able to delete archived contracts. Deletion
  operations MUST require justification and MUST be logged with timestamp and user identity

### DCS-FR-CSA-18 - Audit Log for Contract Storage and Retrieval
id: DCS-FR-CSA-18
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Every archival and retrieval operation MUST be recorded in an immutable audit log. Each log
  entry MUST include actor, timestamp, operation type, contract ID, and success/failure outcome.
effective_requirement: |
  Every archival and retrieval operation MUST be recorded in an immutable audit log. Each log
  entry MUST include actor, timestamp, operation type, contract ID, and success/failure outcome.

### DCS-FR-CSA-19 - Compliance Verification for Archived Contracts
id: DCS-FR-CSA-19
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide tools to verify whether archived contracts meet predefined compliance
  requirements (e.g., retention time, signature policies, metadata completeness). Non-compliant
  entries MUST be flagged.
effective_requirement: |
  The system MUST provide tools to verify whether archived contracts meet predefined compliance
  requirements (e.g., retention time, signature policies, metadata completeness). Non-compliant
  entries MUST be flagged.

### DCS-FR-CSA-20 - Automated Contract Monitoring and Alerts
id: DCS-FR-CSA-20
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The archive MUST include rules-based monitoring for contract status and metadata (e.g.,
  expired, renewal due). Alert notifications MUST be configurable and delivered via UI, email,
  or API.
effective_requirement: |
  The archive MUST include rules-based monitoring for contract status and metadata (e.g.,
  expired, renewal due). Alert notifications MUST be configurable and delivered via UI, email,
  or API.

### DCS-FR-CSA-21 - Contract Archive Dashboard
id: DCS-FR-CSA-21
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  A dashboard MUST provide an overview of archived contract statistics, recent actions, storage
  volume, expiring contracts, and compliance status. The dashboard MUST support drill-down into
  contract details.
effective_requirement: |
  A dashboard MUST provide an overview of archived contract statistics, recent actions, storage
  volume, expiring contracts, and compliance status. The dashboard MUST support drill-down into
  contract details.

### DCS-FR-CSA-22 - Contract Search Interface
id: DCS-FR-CSA-22
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST include a dedicated search interface for archived contracts. It MUST support
  advanced filtering, saved queries, and export of search results for compliance or legal use.
effective_requirement: |
  The system MUST include a dedicated search interface for archived contracts. It MUST support
  advanced filtering, saved queries, and export of search results for compliance or legal use.

### DCS-FR-CSA-23 - Contract Expiration and Renewal Management UI
id: DCS-FR-CSA-23
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST provide a visual interface for monitoring and managing contract expirations
  and renewal tasks. It MUST allow bulk actions and notification management for contract owners.
effective_requirement: |
  The system MUST provide a visual interface for monitoring and managing contract expirations
  and renewal tasks. It MUST allow bulk actions and notification management for contract owners.

### DCS-FR-CSA-24 - Contract Compliance and Audit Viewer
id: DCS-FR-CSA-24
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  A viewer MUST allow auditors to inspect contracts, associated metadata, compliance status, and
  audit logs from a single interface. This viewer MUST support export to standard formats for
  external audits.
effective_requirement: |
  A viewer MUST allow auditors to inspect contracts, associated metadata, compliance status, and
  audit logs from a single interface. This viewer MUST support export to standard formats for
  external audits.

### DCS-FR-CSA-25 - Contract Processing API
id: DCS-FR-CSA-25
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The system MUST expose APIs for contract archival, metadata updates, tagging, and retrieval.
  APIs MUST require authorization and include audit trail generation for each interaction.
effective_requirement: |
  The system MUST expose APIs for contract archival, metadata updates, tagging, and retrieval.
  APIs MUST require authorization and include audit trail generation for each interaction.

### DCS-FR-CSA-26 - Archive Multi-Party Contract Component Assignments
id: DCS-FR-CSA-26
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  For contracts involving multiple parties, each party's assigned sections MUST be individually
  archived and linked to the overall contract package. The system MUST allow per-party access
  restrictions to their respective sections.
effective_requirement: |
  For contracts involving multiple parties, each party's assigned sections MUST be individually
  archived and linked to the overall contract package. The system MUST allow per-party access
  restrictions to their respective sections.

### DCS-FR-PACM-01 - Tamper-Proof Audit Trail for Contract Lifecycle
id: DCS-FR-PACM-01
area: Functional Requirements
implementation_status: Done
category: Process Audit & Compliance
interpretation_status: Unchanged
source_requirement: |
  The system MUST maintain a tamper-proof audit trail capturing all contract lifecycle events -
  including creation, editing, submission, review, signing, renewal, and termination. Each log
  entry MUST include a timestamp, actor identity, action taken, and affected contract component.
  Audit logs MUST be immutable and exportable for forensic review.
effective_requirement: |
  The system MUST maintain a tamper-proof audit trail capturing all contract lifecycle events -
  including creation, editing, submission, review, signing, renewal, and termination. Each log
  entry MUST include a timestamp, actor identity, action taken, and affected contract component.
  Audit logs MUST be immutable and exportable for forensic review.

### DCS-FR-PACM-02 - Compliance Monitoring and Risk Detection
id: DCS-FR-PACM-02
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST continuously monitor contract lifecycle activities for violations of defined
  compliance rules (e.g., missing approvals, expired credentials, unauthorized access). Detected
  risks MUST be flagged and reported in real-time via dashboards and alerts.
effective_requirement: |
  The system MUST continuously monitor contract lifecycle activities for violations of defined
  compliance rules (e.g., missing approvals, expired credentials, unauthorized access). Detected
  risks MUST be flagged and reported in real-time via dashboards and alerts.

### DCS-FR-PACM-03 - Automated Regulatory and Policy Compliance Checks
id: DCS-FR-PACM-03
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST perform automated checks during contract workflows to ensure compliance with
  regulatory frameworks (e.g., eIDAS, GDPR, ISO) and internal policies. Contracts failing these
  checks MUST be blocked from execution or flagged for manual review.
effective_requirement: |
  The system MUST perform automated checks during contract workflows to ensure compliance with
  regulatory frameworks (e.g., eIDAS, GDPR, ISO) and internal policies. Contracts failing these
  checks MUST be blocked from execution or flagged for manual review.

### DCS-FR-PACM-04 - Role-Based Access Control for Audit Logs
id: DCS-FR-PACM-04
area: Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Access to audit logs MUST be restricted based on roles such as Compliance Officer, Auditor, or
  Admin. Each access MUST be logged and include a justification for traceability. Unauthorized
  access attempts MUST be blocked and logged
effective_requirement: |
  Access to audit logs MUST be restricted based on roles such as Compliance Officer, Auditor, or
  Admin. Each access MUST be logged and include a justification for traceability. Unauthorized
  access attempts MUST be blocked and logged

### DCS-FR-PACM-05 - Contract Non-Compliance Investigation and Reporting
id: DCS-FR-PACM-05
area: Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST include tools for investigating non-compliance events, such as incomplete
  workflows, late signatures, or invalid credentials. Investigators MUST be able to generate
  detailed compliance reports and export case files for regulatory review.
effective_requirement: |
  The system MUST include tools for investigating non-compliance events, such as incomplete
  workflows, late signatures, or invalid credentials. Investigators MUST be able to generate
  detailed compliance reports and export case files for regulatory review.

### DCS-FR-PACM-06 - Structural Integrity Validation for Multi-Contract Packages
id: DCS-FR-PACM-06
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST validate the structural integrity of multi-contract packages to ensure
  completeness, logical correctness, and proper linkage between components (e.g., main contract,
  annexes, subagreements). Missing or misconfigured components MUST be flagged before execution
effective_requirement: |
  The system MUST validate the structural integrity of multi-contract packages to ensure
  completeness, logical correctness, and proper linkage between components (e.g., main contract,
  annexes, subagreements). Missing or misconfigured components MUST be flagged before execution

### DCS-FR-PACM-07 - Compliance Reporting by Contract Component and Party
id: DCS-FR-PACM-07
area: Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST generate compliance reports segmented by individual contract components (e.g.,
  clauses, appendices) and by involved parties. Each report MUST include compliance status,
  timestamp, nonconformance indicators, and relevant credential metadata.
effective_requirement: |
  The system MUST generate compliance reports segmented by individual contract components (e.g.,
  clauses, appendices) and by involved parties. Each report MUST include compliance status,
  timestamp, nonconformance indicators, and relevant credential metadata.



## Interface Requirements

### DCS-IR-TR-01 - Template Builder UI
id: DCS-IR-TR-01
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Builder MUST allow Template Creator to create new contract templates and update
  existing ones.
effective_requirement: |
  Template Builder MUST allow Template Creator to create new contract templates and update
  existing ones.

### DCS-IR-TR-02
id: DCS-IR-TR-02
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Builder MUST allow searching and retrieving existing templates for reuse or
  modification.
effective_requirement: |
  Template Builder MUST allow searching and retrieving existing templates for reuse or
  modification.

### DCS-IR-TR-03 - Template Review UI
id: DCS-IR-TR-03
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Review MUST allow Reviewers to retrieve, verify, update, and submit templates.
effective_requirement: |
  Template Review MUST allow Reviewers to retrieve, verify, update, and submit templates.

### DCS-IR-TR-04
id: DCS-IR-TR-04
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Review MUST support forwarding a verified template to approval or returning it to
  draft with comments.
effective_requirement: |
  Template Review MUST support forwarding a verified template to approval or returning it to
  draft with comments.

### DCS-IR-TR-05 - Template Approval UI
id: DCS-IR-TR-05
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Approval MUST allow Approvers to retrieve, approve, reject, or resubmit templates.
effective_requirement: |
  Template Approval MUST allow Approvers to retrieve, approve, reject, or resubmit templates.

### DCS-IR-TR-06
id: DCS-IR-TR-06
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Approval MUST ensure that only validated templates enter the pool of contractready
  assets.
effective_requirement: |
  Template Approval MUST ensure that only validated templates enter the pool of contractready
  assets.

### DCS-IR-TR-07 - Template Management Dashboard UI
id: DCS-IR-TR-07
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Management Dashboard MUST allow Managers to register, archive, update, search,
  and audit templates.
effective_requirement: |
  Template Management Dashboard MUST allow Managers to register, archive, update, search,
  and audit templates.

### DCS-IR-TR-08
id: DCS-IR-TR-08
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Template Management Dashboard MUST provide lifecycle oversight of all templates in the
  repository.
effective_requirement: |
  Template Management Dashboard MUST provide lifecycle oversight of all templates in the
  repository.

### DCS-IR-CWE-01 - Contract Creation UI
id: DCS-IR-CWE-01
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Creation UI MUST allow Contract Creators to create and submit contracts from
  approved templates
effective_requirement: |
  Contract Creation UI MUST allow Contract Creators to create and submit contracts from
  approved templates

### DCS-IR-CWE-02
id: DCS-IR-CWE-02
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Contract Creation UI MUST enable population of contract data, including parties, assets,
  policies, and evidence.
effective_requirement: |
  Contract Creation UI MUST enable population of contract data, including parties, assets,
  policies, and evidence.
context_note: |
  ontology ongoing

### DCS-IR-CWE-03 - Contract Negotiation UI
id: DCS-IR-CWE-03
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Negotiation UI MUST allow parties to exchange responses, redlines, and comments prior
  to contract approval.
effective_requirement: |
  Contract Negotiation UI MUST allow parties to exchange responses, redlines, and comments prior
  to contract approval.

### DCS-IR-CWE-04
id: DCS-IR-CWE-04
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Negotiation UI MUST support comparison of contract versions for transparency and
  traceability.
effective_requirement: |
  Contract Negotiation UI MUST support comparison of contract versions for transparency and
  traceability.

### DCS-IR-CWE-05 - Contract Review UI
id: DCS-IR-CWE-05
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Review UI MUST allow Reviewers to retrieve, inspect, and validate contracts after
  negotiation.
effective_requirement: |
  Contract Review UI MUST allow Reviewers to retrieve, inspect, and validate contracts after
  negotiation.

### DCS-IR-CWE-06
id: DCS-IR-CWE-06
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Review UI MUST allow Reviewers to respond with findings, request modifications, or
  forward contracts for approval.
effective_requirement: |
  Contract Review UI MUST allow Reviewers to respond with findings, request modifications, or
  forward contracts for approval.

### DCS-IR-CWE-07
id: DCS-IR-CWE-07
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Review UI MUST provide search capabilities to locate contracts by metadata, parties,
  or template references
effective_requirement: |
  Contract Review UI MUST provide search capabilities to locate contracts by metadata, parties,
  or template references

### DCS-IR-CWE-08 - Contract Approval UI
id: DCS-IR-CWE-08
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Approval UI MUST allow Approvers to retrieve contracts in reviewed state.
effective_requirement: |
  Contract Approval UI MUST allow Approvers to retrieve contracts in reviewed state.

### DCS-IR-CWE-09
id: DCS-IR-CWE-09
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Approval UI MUST allow Approvers to approve, reject (with reason), or resubmit
  contracts.
effective_requirement: |
  Contract Approval UI MUST allow Approvers to approve, reject (with reason), or resubmit
  contracts.

### DCS-IR-CWE-10
id: DCS-IR-CWE-10
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Approval UI MUST ensure approved contracts are forwarded into the signing workflow
  and catalogue
effective_requirement: |
  Contract Approval UI MUST ensure approved contracts are forwarded into the signing workflow
  and catalogue

### DCS-IR-CWE-11 - Contract Management UI
id: DCS-IR-CWE-11
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Management Dashboard UI MUST allow Managers to retrieve and search contracts across
  lifecycle states.
effective_requirement: |
  Contract Management Dashboard UI MUST allow Managers to retrieve and search contracts across
  lifecycle states.

### DCS-IR-CWE-12
id: DCS-IR-CWE-12
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Contract Management Dashboard UI MUST allow Managers to store evidence, terminate contracts,
  and perform audits.
effective_requirement: |
  Contract Management Dashboard UI MUST allow Managers to store evidence, terminate contracts,
  and perform audits.

### DCS-IR-CWE-13
id: DCS-IR-CWE-13
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Contract Management Dashboard UI MUST provide lifecycle monitoring aligned with XFSC
  lifecycle/log token usage.
effective_requirement: |
  Contract Management Dashboard UI MUST provide lifecycle monitoring aligned with XFSC
  lifecycle/log token usage.
context_note: |
  needs to be clearified; what are XFSC lifecycle/log tokens

### DCS-IR-SM-01 - Secure Contract Viewer UI
id: DCS-IR-SM-01
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Secure Contract Viewer UI MUST allow Signers and Managers to retrieve approved contracts
  prepared for signing.
effective_requirement: |
  Secure Contract Viewer UI MUST allow Signers and Managers to retrieve approved contracts
  prepared for signing.

### DCS-IR-SM-02
id: DCS-IR-SM-02
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Secure Contract Viewer UI MUST allow verification of contract integrity and signature
  envelopes.
effective_requirement: |
  Secure Contract Viewer UI MUST allow verification of contract integrity and signature
  envelopes.

### DCS-IR-SM-03
id: DCS-IR-SM-03
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Secure Contract Viewer UI MUST allow applying signatures with appropriate credentials (e.g.,
  QES/AES with PoA if required).
effective_requirement: |
  Secure Contract Viewer UI MUST allow applying signatures with appropriate credentials (e.g.,
  QES/AES with PoA if required).

### DCS-IR-SM-04
id: DCS-IR-SM-04
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Secure Contract Viewer UI MUST allow validation of applied signatures to ensure compliance and
  integrity.
effective_requirement: |
  Secure Contract Viewer UI MUST allow validation of applied signatures to ensure compliance and
  integrity.

### DCS-IR-SM-05 - Signature Compliance Viewer UI
id: DCS-IR-SM-05
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Signature Compliance Viewer UI MUST allow compliance users to validate trust anchors,
  cryptographic proofs, and timestamps of signatures.
effective_requirement: |
  Signature Compliance Viewer UI MUST allow compliance users to validate trust anchors,
  cryptographic proofs, and timestamps of signatures.

### DCS-IR-SM-06
id: DCS-IR-SM-06
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Signature Compliance Viewer UI MUST allow revocation of signatures if required (e.g., signer
  credentials revoked, policy breach).
effective_requirement: |
  Signature Compliance Viewer UI MUST allow revocation of signatures if required (e.g., signer
  credentials revoked, policy breach).

### DCS-IR-SM-07
id: DCS-IR-SM-07
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Signature Compliance Viewer UI MUST allow running compliance checks against applicable
  policies, standards (eIDAS, ETSI), and business rules.
effective_requirement: |
  Signature Compliance Viewer UI MUST allow running compliance checks against applicable
  policies, standards (eIDAS, ETSI), and business rules.

### DCS-IR-SM-08
id: DCS-IR-SM-08
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Signature Compliance Viewer UI MUST allow generating audit reports covering validation and
  compliance results.
effective_requirement: |
  Signature Compliance Viewer UI MUST allow generating audit reports covering validation and
  compliance results.

### DCS-IR-CSA-01 - Archive Manager Dashboard UI
id: DCS-IR-CSA-01
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Archive Manager Dashboard UI MUST allow Archive Managers to retrieve and search archived
  contracts and related records.
effective_requirement: |
  Archive Manager Dashboard UI MUST allow Archive Managers to retrieve and search archived
  contracts and related records.

### DCS-IR-CSA-02
id: DCS-IR-CSA-02
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Archive Manager Dashboard UI MUST allow storing new contracts and evidence in the archive.
effective_requirement: |
  Archive Manager Dashboard UI MUST allow storing new contracts and evidence in the archive.

### DCS-IR-CSA-03
id: DCS-IR-CSA-03
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Archive Manager Dashboard UI MUST allow terminating or deleting archived entries under defined
  policies.
effective_requirement: |
  Archive Manager Dashboard UI MUST allow terminating or deleting archived entries under defined
  policies.

### DCS-IR-CSA-04
id: DCS-IR-CSA-04
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Archive Manager Dashboard UI MUST allow running audits on archive operations and integrity.
effective_requirement: |
  Archive Manager Dashboard UI MUST allow running audits on archive operations and integrity.

### DCS-IR-PACM-01 - Auditing Tool UI
id: DCS-IR-PACM-01
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Auditing Tool UI MUST allow Auditors to initiate audits across contracts, templates, and
  signatures.
effective_requirement: |
  Auditing Tool UI MUST allow Auditors to initiate audits across contracts, templates, and
  signatures.

### DCS-IR-PACM-02
id: DCS-IR-PACM-02
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Auditing Tool UI MUST provide reporting capabilities with exportable audit results.
effective_requirement: |
  Auditing Tool UI MUST provide reporting capabilities with exportable audit results.

### DCS-IR-PACM-03 - Non-Compliance Investigation UI
id: DCS-IR-PACM-03
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Non-Compliance Investigation UI MUST allow Compliance Officers to continuously monitor events
  and policy adherence.
effective_requirement: |
  Non-Compliance Investigation UI MUST allow Compliance Officers to continuously monitor events
  and policy adherence.

### DCS-IR-PACM-04
id: DCS-IR-PACM-04
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Non-Compliance Investigation UI MUST allow incident reporting and linking findings to affected
  contracts or templates.
effective_requirement: |
  Non-Compliance Investigation UI MUST allow incident reporting and linking findings to affected
  contracts or templates.

### DCS-IR-HI-01 - Interface for Use of Signing Secrets (HSM/QSCD/TPM)
id: DCS-IR-HI-01
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Private keys and other service secrets used by DCS MUST be protected by hardware security
  mechanisms (e.g., HSM, QSCD, or TPM/Secure Enclave), using standardized interfaces, with
  secrets held in a nonexportable, tamper-resistant manner; the solution SHOULD support
  operation in virtualized/containerized environments without weakening hardware protection.
effective_requirement: |
  Private keys and other service secrets used by DCS MUST be protected by hardware security
  mechanisms (e.g., HSM, QSCD, or TPM/Secure Enclave), using standardized interfaces, with
  secrets held in a nonexportable, tamper-resistant manner; the solution SHOULD support
  operation in virtualized/containerized environments without weakening hardware protection.
context_note: |
  vault transit engine? pkcs#11?

### DCS-IR-HI-02 - FIDO2 Security Key Interface
id: DCS-IR-HI-02
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Adjusted Pending Identity Decision
source_requirement: |
  DCS web clients MUST support hardware authenticators (USB/NFC/BLE security keys or platform
  authenticators) for phishing-resistant login and step-up approval, using standard browser APIs
  (WebAuthn) with keys remaining protected in the device.
effective_requirement: |
  Support FIDO2/WebAuthn only where compatible with the final authentication model. If all
  access must use OID4VP, limit FIDO2 to local key/secret protection or step-up scenarios
  outside primary login.
effective_acceptance_criteria: |
  The authentication design documents whether FIDO2 participates in login, step-up, or secret
  protection; implementation does not conflict with an OID4VP-only access policy.
implementation_decision: |
  Use transit-engine style protection where FIDO2 login is not compatible with OID4VP-only
  access.
constraint_note: |
  unclear with UC-09 / "All access must be oid4vp"

### DCS-IR-HI-03 - Platform TPM 2.0 / Secure Enclave Interface
id: DCS-IR-HI-03
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  DCS services MUST be able to protect service credentials by sealing them to a platform
  hardware root of trust (TPM 2.0 or Secure Enclave) and SHOULD support remote attestation to
  prove platform integrity prior to enabling sensitive operations.
effective_requirement: |
  DCS services MUST be able to protect service credentials by sealing them to a platform
  hardware root of trust (TPM 2.0 or Secure Enclave) and SHOULD support remote attestation to
  prove platform integrity prior to enabling sensitive operations.
context_note: |
  vault transit engine? pkcs#11?

### DCS-IR-SI-01 - Template Catalogue Integration
id: DCS-IR-SI-01
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Adjusted Alternative Implementation
source_requirement: |
  An interface MUST be provided between the TR and the XFSC Catalogue for template discovery,
  request, and registration via application APIs.
effective_requirement: |
  Provide Catalogue integration through a versioned adapter. Concrete XFSC Catalogue behavior is
  required only for the stable Catalogue API; otherwise use internal registration/mapping until
  re-alignment.
effective_acceptance_criteria: |
  Template discovery/registration works through the DCS adapter or internal mapping; the
  integration can be switched to a stable XFSC Catalogue API without changing callers.
implementation_decision: |
  Align with the current Catalogue API structure via adapter.
constraint_note: |
  catalogue changed

### DCS-IR-SI-02 - Workflow Orchestration (Node-RED) Integration
id: DCS-IR-SI-02
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between the CWE and the XFSC Orchestration Engine (Node-RED)
  exposing Node-RED-compatible endpoints and webhook callbacks for automated workflow
  invocation.
effective_requirement: |
  An interface MUST be provided between the CWE and the XFSC Orchestration Engine (Node-RED)
  exposing Node-RED-compatible endpoints and webhook callbacks for automated workflow
  invocation.

### DCS-IR-SI-03 - Platform Authentication & Authorization Integration
id: DCS-IR-SI-03
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Interfaces MUST be provided between all DCS components and the Authentication & Authorization
  Service implementing OAuth2/OIDC flows for service-to-service and user access control.
effective_requirement: |
  Interfaces MUST be provided between all DCS components and the Authentication & Authorization
  Service implementing OAuth2/OIDC flows for service-to-service and user access control.
context_note: |
  unclear with UC-09 / "All access must be oid4vp"

### DCS-IR-SI-04 - Wallet & TSP Signing Integration
id: DCS-IR-SI-04
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between SM and identity wallets and TSPs supporting
  OpenID4VCI/4VP for credential issuance/presentation and remote AES/QES signing and validation.
effective_requirement: |
  An interface MUST be provided between SM and identity wallets and TSPs supporting
  OpenID4VCI/4VP for credential issuance/presentation and remote AES/QES signing and validation.
context_note: |
  needs to be clearified

### DCS-IR-SI-05 - External Target System API Integration
id: DCS-IR-SI-05
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between DCS (CWE/SM/CSA) and external target systems (e.g., ERP
  or AI services) exposing contract-processing and automated-interaction APIs for create/deploy
  actions, status queries, and event callbacks.
effective_requirement: |
  An interface MUST be provided between DCS (CWE/SM/CSA) and external target systems (e.g., ERP
  or AI services) exposing contract-processing and automated-interaction APIs for create/deploy
  actions, status queries, and event callbacks.

### DCS-IR-SI-06 - Counterparty DCS Information Endpoint
id: DCS-IR-SI-06
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between a DCS instance and a counterparty DCS offering a
  policy-gated, read-only contract information endpoint.
effective_requirement: |
  An interface MUST be provided between a DCS instance and a counterparty DCS offering a
  policy-gated, read-only contract information endpoint.
context_note: |
  needs to be clearified; synchronizing by pushing is possible;

### DCS-IR-SI-07 - OpenID Provider Discovery & JWKS Consumption
id: DCS-IR-SI-07
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between the DCS Authentication & Authorization Service and
  external OpenID Providers (IdPs) to consume discovery metadata and JWKS for token validation.
effective_requirement: |
  An interface MUST be provided between the DCS Authentication & Authorization Service and
  external OpenID Providers (IdPs) to consume discovery metadata and JWKS for token validation.
context_note: |
  hydra's well-known at edge ingress

### DCS-IR-SI-08 - OpenID4VP Login & Access Contro
id: DCS-IR-SI-08
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between the DCS Authentication & Authorization Service and
  identity wallets to accept OpenID4VP verifiable presentations of authorization credentials for
  login and access control.
effective_requirement: |
  An interface MUST be provided between the DCS Authentication & Authorization Service and
  identity wallets to accept OpenID4VP verifiable presentations of authorization credentials for
  login and access control.

### DCS-IR-SI-09 - Credential Status & Revocation Service
id: DCS-IR-SI-09
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between DCS and a status/revocation list service to check and
  update credential status during validation and enforcement
effective_requirement: |
  An interface MUST be provided between DCS and a status/revocation list service to check and
  update credential status during validation and enforcement

### DCS-IR-SI-10 - Digital Signature Service (DSS) Authorization & Signing
id: DCS-IR-SI-10
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between SM and a DSS to authorize and execute remote AES/QES
  operations and return signature artifacts and timestamps.
effective_requirement: |
  An interface MUST be provided between SM and a DSS to authorize and execute remote AES/QES
  operations and return signature artifacts and timestamps.

### DCS-IR-SI-11 - Relational Database Access
id: DCS-IR-SI-11
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between DCS components and the relational database (e.g.,
  PostgreSQL) for CRUD access to shared entities using versioned schemas and migrations.
effective_requirement: |
  An interface MUST be provided between DCS components and the relational database (e.g.,
  PostgreSQL) for CRUD access to shared entities using versioned schemas and migrations.

### DCS-IR-SI-12 - Crypto Provider & DID/VC Operations
id: DCS-IR-SI-12
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  An interface MUST be provided between DCS and the Crypto Provider Service to create/verify DID
  documents and perform VC/VP signing and verification required by wallet integrations
effective_requirement: |
  An interface MUST be provided between DCS and the Crypto Provider Service to create/verify DID
  documents and perform VC/VP signing and verification required by wallet integrations

### DCS-IR-CI-01 - HTTPS/TLS 1.3 Transport
id: DCS-IR-CI-01
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  All external service communications for DCS MUST use HTTPS over TLS 1.3 for confidentiality
  and integrity.
effective_requirement: |
  All external service communications for DCS MUST use HTTPS over TLS 1.3 for confidentiality
  and integrity.

### DCS-IR-CI-02 - REST/JSON API Conventions
id: DCS-IR-CI-02
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  All application APIs MUST follow REST semantics with application/json request/response bodies;
  binary artifacts (e.g., signed PDFs) MUST be served as application/pdf. This applies to the
  signature and archive
  endpoints already defined.
effective_requirement: |
  All application APIs MUST follow REST semantics with application/json request/response bodies;
  binary artifacts (e.g., signed PDFs) MUST be served as application/pdf. This applies to the
  signature and archive
  endpoints already defined.

### DCS-IR-CI-03 - Browser Access over HTTPS
id: DCS-IR-CI-03
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  All web UI interactions (Template, Workflow, Signature, Archive, PACM) MUST be delivered via
  HTTPS, with no additional non-web protocols required.
effective_requirement: |
  All web UI interactions (Template, Workflow, Signature, Archive, PACM) MUST be delivered via
  HTTPS, with no additional non-web protocols required.

### DCS-IR-CI-04 - OAuth2/OIDC Flows
id: DCS-IR-CI-04
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  DCS MUST implement OAuth2/OIDC authorization, token, and introspection flows for both user and
  service access to APIs.
effective_requirement: |
  DCS MUST implement OAuth2/OIDC authorization, token, and introspection flows for both user and
  service access to APIs.

### DCS-IR-CI-05 - OpenID Discovery & JWKS
id: DCS-IR-CI-05
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  DCS MUST consume well-known OpenID Provider discovery metadata and JWKS endpoints for token
  and client validation.
effective_requirement: |
  DCS MUST consume well-known OpenID Provider discovery metadata and JWKS endpoints for token
  and client validation.

### DCS-IR-CI-06 - OpenID4VC/VP Bindings
id: DCS-IR-CI-06
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Wallet interactions MUST use OpenID4VCI for issuance and OpenID4VP for presentation during
  login/authorization and signing flows.
effective_requirement: |
  Wallet interactions MUST use OpenID4VCI for issuance and OpenID4VP for presentation during
  login/authorization and signing flows.

### DCS-IR-CI-07 - Orchestration Webhooks
id: DCS-IR-CI-07
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Workflow invocation and callbacks between the XFSC Orchestration Engine (Node-RED) and DCS
  MUST use HTTP(S) endpoints and webhooks compatible with Node-RED nodes.
effective_requirement: |
  Workflow invocation and callbacks between the XFSC Orchestration Engine (Node-RED) and DCS
  MUST use HTTP(S) endpoints and webhooks compatible with Node-RED nodes.

### DCS-IR-CI-08 - DSS Remote Signing over HTTPS
id: DCS-IR-CI-08
area: Interface Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Remote AES/QES operations with a DSS or TSP MUST be invoked over HTTPS and return standard
  signature containers
effective_requirement: |
  Remote AES/QES operations with a DSS or TSP MUST be invoked over HTTPS and return standard
  signature containers

### DCS-IR-CI-09 - Revocation List Synchronization
id: DCS-IR-CI-09
area: Interface Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Credential and contract-status checks MUST query a compatible revocation/status list service;
  updates to published status MUST be reflected within <= 5 minutes.
effective_requirement: |
  Credential and contract-status checks MUST query a compatible revocation/status list service;
  updates to published status MUST be reflected within <= 5 minutes.
context_note: |
  Bitstring Statuslist

### DCS-IR-CI-10 - PACM Audit Event Transport
id: DCS-IR-CI-10
area: Interface Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Audit and compliance operations MUST use HTTPS JSON endpoints for event submission and report
  retrieval (/pac/audit, /pac/report)
effective_requirement: |
  Audit and compliance operations MUST use HTTPS JSON endpoints for event submission and report
  retrieval (/pac/audit, /pac/report)



## Non-Functional Requirements

### DCS-NFR-PER-01 - Performance by Design
id: DCS-NFR-PER-01
area: Non-Functional Requirements
implementation_status: Done
category: Performance Requirements
interpretation_status: Unchanged
source_requirement: |
  Every component SHOULD be designed and implemented with performance in mind. They MUST
  particularly be implemented in a non-blocking way.
effective_requirement: |
  Every component SHOULD be designed and implemented with performance in mind. They MUST
  particularly be implemented in a non-blocking way.

### DCS-NFR-PER-02 - Scalability
id: DCS-NFR-PER-02
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Every component MUST be scalable and able to handle increased load and users without
  performance degradation. This allows the system to handle growing demand and multi-requests.
effective_requirement: |
  Every component MUST be scalable and able to handle increased load and users without
  performance degradation. This allows the system to handle growing demand and multi-requests.

### DCS-NFR-PER-03 - Availability & Resilience
id: DCS-NFR-PER-03
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  It MUST be ensured the DCS system is always available and can recover from failures.
effective_requirement: |
  It MUST be ensured the DCS system is always available and can recover from failures.

### DCS-NFR-SF-01 - Reset Possibility
id: DCS-NFR-SF-01
area: Non-Functional Requirements
implementation_status: Done
category: Safety Requirements
interpretation_status: Unchanged
source_requirement: |
  In case of errors, it MUST be possible to reset the component and continue execution as
  specified in this document. The component SHOULD be stateless and need no recovery in case of
  a reset to maintain system stability and reliability during errors
effective_requirement: |
  In case of errors, it MUST be possible to reset the component and continue execution as
  specified in this document. The component SHOULD be stateless and need no recovery in case of
  a reset to maintain system stability and reliability during errors

### DCS-NFR-SF-02 - Remote Administration
id: DCS-NFR-SF-02
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  If the component can be remotely administrated by the Federator, the communication MUST
  utilize a secure communication channel such as SSH or VPN.
effective_requirement: |
  If the component can be remotely administrated by the Federator, the communication MUST
  utilize a secure communication channel such as SSH or VPN.

### DCS-NFR-SF-03 - Business Continuity & Disaster Recovery
id: DCS-NFR-SF-03
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  To prevent business disruptions and ensure high availability, The DCS MUST ensure data
  resilience and rapid recovery in case of failures or cyber incidents
effective_requirement: |
  To prevent business disruptions and ensure high availability, The DCS MUST ensure data
  resilience and rapid recovery in case of failures or cyber incidents
context_note: |
  external anchoring pending

### DCS-NFR-SEC-01 - Transport Layer Security
id: DCS-NFR-SEC-01
area: Non-Functional Requirements
implementation_status: Done
category: Security Requirements
interpretation_status: Unchanged
source_requirement: |
  To ensure secure communication and data integrity, each communication with an interface of DCS
  MUST utilize TLS 1.3. It MUST NOT use SSL 3.0, TLS 1.0 and 1.1.
effective_requirement: |
  To ensure secure communication and data integrity, each communication with an interface of DCS
  MUST utilize TLS 1.3. It MUST NOT use SSL 3.0, TLS 1.0 and 1.1.

### DCS-NFR-SEC-02 - State-of-the-art Cryptography
id: DCS-NFR-SEC-02
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Cryptography Cryptographic algorithms and cipher suites MUST be state-of-the-art and chosen in
  accordance with official recommendations. Those recommendations MAY be those of the German
  Federal Office for Information Security (BSI) or SOG-IS.
effective_requirement: |
  Cryptography Cryptographic algorithms and cipher suites MUST be state-of-the-art and chosen in
  accordance with official recommendations. Those recommendations MAY be those of the German
  Federal Office for Information Security (BSI) or SOG-IS.

### DCS-NFR-SEC-03 - Authentication and Authorization
id: DCS-NFR-SEC-03
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  DCS MUST grant access to its services only to authenticated and authorized users. It MUST
  implement RBAC and enforce least privilege principles for human and machine users of the
  service. RBAC MUST be managed via role assertions in the form of a verifiable credential
  stored in the wallets of users and presented during the authentication and authorization
  process of a user.
effective_requirement: |
  DCS MUST grant access to its services only to authenticated and authorized users. It MUST
  implement RBAC and enforce least privilege principles for human and machine users of the
  service. RBAC MUST be managed via role assertions in the form of a verifiable credential
  stored in the wallets of users and presented during the authentication and authorization
  process of a user.

### DCS-NFR-SEC-04 - Integrity Protection for Configuration
id: DCS-NFR-SEC-04
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Where the functionality of the DCS is based on configuration files, those files MUST be
  authenticated, and integrity protected.
effective_requirement: |
  Where the functionality of the DCS is based on configuration files, those files MUST be
  authenticated, and integrity protected.

### DCS-NFR-SEC-05 - Integrity Protection for Service
id: DCS-NFR-SEC-05
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The Federator MUST utilize security measures to ensure the integrity of DCS. It MAY support
  proof of the integrity of remote parties using an additional interface (Remote Attestation).
effective_requirement: |
  The Federator MUST utilize security measures to ensure the integrity of DCS. It MAY support
  proof of the integrity of remote parties using an additional interface (Remote Attestation).

### DCS-NFR-SEC-06 - Storage of Secrets
id: DCS-NFR-SEC-06
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Secrets such as keys and other cryptographic material MUST be stored in a secure and protected
  environment, e.g., a TPM, HSM, or TEE to ensure their confidentiality and integrity.
effective_requirement: |
  Secrets such as keys and other cryptographic material MUST be stored in a secure and protected
  environment, e.g., a TPM, HSM, or TEE to ensure their confidentiality and integrity.

### DCS-NFR-SEC-07 - Testing
id: DCS-NFR-SEC-07
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The development of DCS MUST include functional and security testing, source code audits, and
  penetration testing.
effective_requirement: |
  The development of DCS MUST include functional and security testing, source code audits, and
  penetration testing.

### DCS-NFR-SEC-08 - Confidentiality
id: DCS-NFR-SEC-08
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The data in the DCS MUST be protected from unauthorized access during storage and
  transmission.
effective_requirement: |
  The data in the DCS MUST be protected from unauthorized access during storage and
  transmission.

### DCS-NFR-SEC-09 - Monitoring, Logging & Auditability
id: DCS-NFR-SEC-09
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Logs MUST be securely stored and accessible for audits to allow for proactive issue detection,
  performance optimization, and forensic analysis in case of security incidents.
effective_requirement: |
  Logs MUST be securely stored and accessible for audits to allow for proactive issue detection,
  performance optimization, and forensic analysis in case of security incidents.
context_note: |
  external anchoring pending

### DCS-NFR-SEC-10 - Data Integrity
id: DCS-NFR-SEC-10
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST ensure that data remains unchanged and verifiable over its lifecycle and the
  service MUST be protected against unauthorized data modifications.
effective_requirement: |
  The DCS MUST ensure that data remains unchanged and verifiable over its lifecycle and the
  service MUST be protected against unauthorized data modifications.

### DCS-NFR-SEC-11 - Monitoring & Incident Response
id: DCS-NFR-SEC-11
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST ensure continuous monitoring and automated incident response mechanisms to enable
  proactive detection of anomalies and security incidents.
effective_requirement: |
  The DCS MUST ensure continuous monitoring and automated incident response mechanisms to enable
  proactive detection of anomalies and security incidents.

### DCS-NFR-SEC-12 - Secure Configuration Management
id: DCS-NFR-SEC-12
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST ensure that system configurations are stored securely and protected from
  unauthorized modifications to prevent security misconfigurations.
effective_requirement: |
  The DCS MUST ensure that system configurations are stored securely and protected from
  unauthorized modifications to prevent security misconfigurations.
context_note: |
  hashicorp vault

### DCS-NFR-SEC-13 - Secure Data Disposal
id: DCS-NFR-SEC-13
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST prevent data leaks and compliance violations and it MUST ensure that sensitive
  data is properly deleted when no longer needed.
effective_requirement: |
  The DCS MUST prevent data leaks and compliance violations and it MUST ensure that sensitive
  data is properly deleted when no longer needed.

### DCS-NFR-SEC-14 - Data Encryption at Rest & In Transit
id: DCS-NFR-SEC-14
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  The system MUST ensure all sensitive data is encrypted both in storage and during transmission
  to protect data from unauthorized access.
effective_requirement: |
  The system MUST ensure all sensitive data is encrypted both in storage and during transmission
  to protect data from unauthorized access.

### DCS-NFR-SEC-15 - Secure Software Development Lifecycle (SDLC)
id: DCS-NFR-SEC-15
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  To reduce vulnerabilities in the codebase, the DCS SHOULD enforce secure coding practices and
  security testing throughout the development lifecycle.
effective_requirement: |
  To reduce vulnerabilities in the codebase, the DCS SHOULD enforce secure coding practices and
  security testing throughout the development lifecycle.

### DCS-NFR-SEC-16 - Identity Federation
id: DCS-NFR-SEC-16
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Blocked By Product Decision
source_requirement: |
  DCS MUST enable seamless user authentication across multiple systems and support
  interoperability with third-party identity providers and authentication frameworks for access.
effective_requirement: |
  Choose exactly one coherent identity model before full implementation: VC/OID4VP-first without
  central user management, or central user/role management with VC-based federation. Do not
  implement contradictory models as equally mandatory.
effective_acceptance_criteria: |
  Architecture decision identifies the authoritative identity model; authentication, role
  assignment, and administration tests follow that single model.
implementation_decision: |
  Resolve UC-09 versus mandatory VC/OID4VP login.
constraint_note: |
  Ambiguous requirement. On one hand "The system must use verifiable credentials for every
  login" - meaning DCS dictates no central user management, on the other hand UC-09 dictates
  user management interfaces.

### DCS-NFR-SEC-17 - Secure Boot & Hardware Security
id: DCS-NFR-SEC-17
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  To prevent unauthorized firmware or OS modifications, the DCS MUST ensure that the system only
  runs trusted software through secure boot mechanisms.
effective_requirement: |
  To prevent unauthorized firmware or OS modifications, the DCS MUST ensure that the system only
  runs trusted software through secure boot mechanisms.
context_note: |
  its a web app

### DCS-NFR-SEC-18 - Selective Disclosure for Privacy
id: DCS-NFR-SEC-18
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  By default, DCS MUST enforce selective disclosure of attributes, sharing only the minimum
  necessary information. Machine-to-Machine interactions in the name of a user MUST require
  explicit and verifiable user consent prior to execution.
effective_requirement: |
  By default, DCS MUST enforce selective disclosure of attributes, sharing only the minimum
  necessary information. Machine-to-Machine interactions in the name of a user MUST require
  explicit and verifiable user consent prior to execution.

### DCS-NFR-SQ-01 - Programming Style
id: DCS-NFR-SQ-01
area: Non-Functional Requirements
implementation_status: Ongoing
category: Software Quality Attributes
interpretation_status: Unchanged
source_requirement: |
  The implementation SHOULD follow best practices and a consistent style for coding, e.g., the
  source code SHOULD be clearly structured and modularized; there SHOULD be no dead code;
  function and variables SHOULD be clear and self-explaining. The code MUST be well documented
  to support adaptability, maintainability, and usability of the component.
effective_requirement: |
  The implementation SHOULD follow best practices and a consistent style for coding, e.g., the
  source code SHOULD be clearly structured and modularized; there SHOULD be no dead code;
  function and variables SHOULD be clear and self-explaining. The code MUST be well documented
  to support adaptability, maintainability, and usability of the component.
context_note: |
  ongoing, system is being programmed

### DCS-NFR-SQ-02 - Build Scripts
id: DCS-NFR-SQ-02
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The repository for the DCS MUST include build scripts to build and run the Service from the
  repository
effective_requirement: |
  The repository for the DCS MUST include build scripts to build and run the Service from the
  repository

### DCS-NFR-SQ-03 - Containerized Deployment
id: DCS-NFR-SQ-03
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  The software MUST be containerized using industry-standard containerization technologies
  (e.g., Docker, Podman) to ensure seamless deployment, portability, and runtime consistency
  across container orchestration platforms such as Kubernetes and OpenShift.
effective_requirement: |
  The software MUST be containerized using industry-standard containerization technologies
  (e.g., Docker, Podman) to ensure seamless deployment, portability, and runtime consistency
  across container orchestration platforms such as Kubernetes and OpenShift.

### DCS-NFR-SQ-04 - Privacy by Design
id: DCS-NFR-SQ-04
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  To ensure compliance with privacy regulations, the DCS SHOULD embed privacy-enhancing
  technologies into system architecture and data processing.
effective_requirement: |
  To ensure compliance with privacy regulations, the DCS SHOULD embed privacy-enhancing
  technologies into system architecture and data processing.

### DCS-NFR-SQ-05 - Non-Repudiation
id: DCS-NFR-SQ-05
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The system MUST ensure that digital signatures and logs can be used as legal evidence.
effective_requirement: |
  The system MUST ensure that digital signatures and logs can be used as legal evidence.

### DCS-NFR-SQ-06 - System Interoperability
id: DCS-NFR-SQ-06
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  To facilitates seamless data exchange and integration, the system MUST ensure compatibility
  with external platforms, cloud providers, and enterprise systems.
effective_requirement: |
  To facilitates seamless data exchange and integration, the system MUST ensure compatibility
  with external platforms, cloud providers, and enterprise systems.

### DCS-NFR-SQ-07 - Usability & Accessibility
id: DCS-NFR-SQ-07
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST ensure that the system is easy to use and accessible.
effective_requirement: |
  The DCS MUST ensure that the system is easy to use and accessible.

### DCS-NFR-SQ-08 - Orchestration Layer
id: DCS-NFR-SQ-08
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  To ease the integration of XFSC services and components, the DCS MUST be integrated with the
  FACIS Orchestration Engine which provides access to XFSC Cloud PCM, OCM, Catalogue, and Trust
  Module.
effective_requirement: |
  To ease the integration of XFSC services and components, the DCS MUST be integrated with the
  FACIS Orchestration Engine which provides access to XFSC Cloud PCM, OCM, Catalogue, and Trust
  Module.
context_note: |
  deployment node exists as zip for ORCE

### DCS-NFR-BR-01 - Strong Authentication & Role Binding
id: DCS-NFR-BR-01
area: Non-Functional Requirements
implementation_status: Ongoing
category: Business Rules
interpretation_status: Adjusted Pending Identity Decision
source_requirement: |
  Access to the DCS MUST only be possible with two-factor authentication and VCs from recognized
  organizational/legal-person wallets. Functions MAY only be executed by users or system roles
  that have been explicitly authorized for that action.
effective_requirement: |
  Enforce strong authentication and role-based authorization. The exact combination of 2FA,
  VC/OID4VP, and organizational wallet is mandatory only after the identity model is decided.
  Until then use configurable authentication and role binding.
effective_acceptance_criteria: |
  Protected actions require authenticated identity and explicit role/permission binding; the
  selected factors and VC requirements are configurable and traceable to the identity decision.
implementation_decision: |
  Decide VC/OID4VP, 2FA, and central role-management relationship.
constraint_note: |
  Ambiguos requirement

### DCS-NFR-BR-02 - Participant Eligibility
id: DCS-NFR-BR-02
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Only recognized ecosystem participants proven via organizational/legal-person wallets MAY
  interact with the DCS. Unverified entities MUST NOT obtain access or interact with contract
  workflows.
effective_requirement: |
  Only recognized ecosystem participants proven via organizational/legal-person wallets MAY
  interact with the DCS. Unverified entities MUST NOT obtain access or interact with contract
  workflows.
context_note: |
  lacking real PKI

### DCS-NFR-BR-03 - Legally Valid Signatures
id: DCS-NFR-BR-03
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Contracts requiring legal enforceability MUST be signed with AES by default and with QES or
  seals where mandated. Contracts lacking the required signature level MUST NOT proceed to
  deployment or execution states.
effective_requirement: |
  Contracts requiring legal enforceability MUST be signed with AES by default and with QES or
  seals where mandated. Contracts lacking the required signature level MUST NOT proceed to
  deployment or execution states.

### DCS-NFR-BR-04 - Template Governance
id: DCS-NFR-BR-04
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Every contract MUST originate from an approved, versioned template in the Template Repository.
  Unapproved templates MUST NOT be usable, and template changes MUST be versioned and traceable.
effective_requirement: |
  Every contract MUST originate from an approved, versioned template in the Template Repository.
  Unapproved templates MUST NOT be usable, and template changes MUST be versioned and traceable.

### DCS-NFR-BR-05 - Immutable Auditability
id: DCS-NFR-BR-05
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  All contract actions (creation, negotiation, review, approval, signing, archival, revocation)
  MUST produce immutable, tamper-evident audit records with timestamps and actor identity.
  Access to audit trails MUST be restricted to authorized roles (e.g., Auditor, Compliance
  Officer).
effective_requirement: |
  All contract actions (creation, negotiation, review, approval, signing, archival, revocation)
  MUST produce immutable, tamper-evident audit records with timestamps and actor identity.
  Access to audit trails MUST be restricted to authorized roles (e.g., Auditor, Compliance
  Officer).

### DCS-NFR-BR-06 - Revocation & Termination Propagation
id: DCS-NFR-BR-06
area: Non-Functional Requirements
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Revocation of credentials, signatures, or contracts MUST take immediate effect and be
  propagated across dependent systems; terminated contracts MUST be archived with full evidence
  preserved.
effective_requirement: |
  Revocation of credentials, signatures, or contracts MUST take immediate effect and be
  propagated across dependent systems; terminated contracts MUST be archived with full evidence
  preserved.
context_note: |
  live bitstring status list checks

### DCS-NFR-BR-07 - Token & API Control
id: DCS-NFR-BR-07
area: Non-Functional Requirements
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Integration tokens (including any logging or "log" tokens) and API credentials MUST only be
  issued to verified participants and MUST authorize only the minimum necessary scopes for the
  intended workflow.
effective_requirement: |
  Integration tokens (including any logging or "log" tokens) and API credentials MUST only be
  issued to verified participants and MUST authorize only the minimum necessary scopes for the
  intended workflow.

### DCS-NFR-BR-08 - DCS-to-DCS Interoperability Safeguards
id: DCS-NFR-BR-08
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Alternative Implementation
source_requirement: |
  DCS-to-DCS exchanges (create offers, status updates, revocations) MUST occur only over
  authenticated/authorized APIs between verified parties, with full traceability and audit logs.
effective_requirement: |
  DCS-to-DCS exchanges must be authenticated, authorized, traceable, and auditable. Until
  OCM/Cloud PCM are available, peer authentication may use local did.json documents, private
  keys, and a trust list for peer DIDs.
effective_acceptance_criteria: |
  Peer DCS calls are rejected unless the peer DID/key is trusted; successful calls are
  authorized and written to audit logs with trace identifiers.
implementation_decision: |
  Authenticate with own did.json/private key and a trust list for peer DIDs.
constraint_note: |
  OCM Stack is undeployed, no working cloud PCM available

### DCS-NFR-COMP-01 - Legal Compliance
id: DCS-NFR-COMP-01
area: Non-Functional Requirements
implementation_status: Ongoing
category: Compliance
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST ensure compliance with national and European laws for system operations. The DCS
  MUST adhere to international regulations.
effective_requirement: |
  The DCS MUST ensure compliance with national and European laws for system operations. The DCS
  MUST adhere to international regulations.

### DCS-NFR- COMP-02 - EUCS/ENISA Compliance
id: DCS-NFR- COMP-02
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The DCS SHOULD fulfill the cybersecurity control set of the EUCS Annex A according to its
  assigned Assurance Level to meet cybersecurity compliance standards for assurance levels.
effective_requirement: |
  The DCS SHOULD fulfill the cybersecurity control set of the EUCS Annex A according to its
  assigned Assurance Level to meet cybersecurity compliance standards for assurance levels.

### DCS-NFR- COMP-03 - GDPR Compliance
id: DCS-NFR- COMP-03
area: Non-Functional Requirements
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  The DCS MUST ensure audit logs and compliance against eIDAS/EUDI logging regulations, the
  eIDAS Regulation (910/2014 & upcoming eIDAS 2.0), the General Data Protection Regulation
  (GDPR, Regulation 2016/679), and relevant ETSI and ISO standards (e.g., ETSI EN 319 401,
  ISO/IEC 27001).
effective_requirement: |
  The DCS MUST ensure audit logs and compliance against eIDAS/EUDI logging regulations, the
  eIDAS Regulation (910/2014 & upcoming eIDAS 2.0), the General Data Protection Regulation
  (GDPR, Regulation 2016/679), and relevant ETSI and ISO standards (e.g., ETSI EN 319 401,
  ISO/IEC 27001).



## Use Cases

### UC-01 - User Authentication & Authorization
id: UC-01
area: Use Cases
implementation_status: Done
category: Use Cases
interpretation_status: Unchanged
source_requirement: |
  Authenticate; access a roleprotected page/API; attempt an unauthorized action
source_acceptance_criteria: |
  Authorized access succeeds; unauthorized action denied; audit entry shows actor, role,
  decision
effective_requirement: |
  Authenticate; access a roleprotected page/API; attempt an unauthorized action
effective_acceptance_criteria: |
  Authorized access succeeds; unauthorized action denied; audit entry shows actor, role,
  decision
context_note: |
  untested against real PKI

### UC-02 - Contract Template Management
id: UC-02
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Create template; submit for review; approve; search & open
source_acceptance_criteria: |
  Template stored with version/provenance; status transitions logged; search returns approved
  item
effective_requirement: |
  Create template; submit for review; approve; search & open
effective_acceptance_criteria: |
  Template stored with version/provenance; status transitions logged; search returns approved
  item

### UC-03 - Contract Creation
id: UC-03
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Instantiate from template; edit metadata; route for approval; lock content
source_acceptance_criteria: |
  Contract ID issued; "Approved" status; immutable hash/provenance recorded; audit trail
  complete
effective_requirement: |
  Instantiate from template; edit metadata; route for approval; lock content
effective_acceptance_criteria: |
  Contract ID issued; "Approved" status; immutable hash/provenance recorded; audit trail
  complete
context_note: |
  Signing missing

### UC-04 - Contract Signing
id: UC-04
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Open secure viewer; present identity/PoA; execute signature; verify result
source_acceptance_criteria: |
  Valid signature attached; signer & PoA bound; verification OK; timestamp and evidence stored
effective_requirement: |
  Open secure viewer; present identity/PoA; execute signature; verify result
effective_acceptance_criteria: |
  Valid signature attached; signer & PoA bound; verification OK; timestamp and evidence stored

### UC-05 - Contract Deployment
id: UC-05
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Trigger deployment; observe outbound call; confirm receipt
source_acceptance_criteria: |
  Target acknowledges deployment; DCS status "Deployed;" correlation ID and receipt archived
effective_requirement: |
  Trigger deployment; observe outbound call; confirm receipt
effective_acceptance_criteria: |
  Target acknowledges deployment; DCS status "Deployed;" correlation ID and receipt archived

### UC-06 - Contract Lifecycle Management
id: UC-06
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  View KPIs/alerts; initiate renew or terminate flow; confirm state change
source_acceptance_criteria: |
  KPIs visible; new term or terminated state recorded; notifications and logs captured
effective_requirement: |
  View KPIs/alerts; initiate renew or terminate flow; confirm state change
effective_acceptance_criteria: |
  KPIs visible; new term or terminated state recorded; notifications and logs captured
context_note: |
  Du kannst es terminieren, die frage ist was passiert dann

### UC-07 - Contract Storage & Security
id: UC-07
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Store in archive; search; retrieve artifact
source_acceptance_criteria: |
  Entry stored as PDF/A-3 (or configured format); search finds it; retrieval returns intact
  file; audit event written
effective_requirement: |
  Store in archive; search; retrieve artifact
effective_acceptance_criteria: |
  Entry stored as PDF/A-3 (or configured format); search finds it; retrieval returns intact
  file; audit event written
context_note: |
  storing missing

### UC-08 - Contract Compliance & Audit
id: UC-08
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Request activity report; run policy audit; export results
source_acceptance_criteria: |
  Report lists actors/timestamps/actions; violations (if any) flagged with reasons; exports
  available
effective_requirement: |
  Request activity report; run policy audit; export results
effective_acceptance_criteria: |
  Report lists actors/timestamps/actions; violations (if any) flagged with reasons; exports
  available
context_note: |
  ontology not finished

### UC-09 - DCS Administration
id: UC-09
area: Use Cases
implementation_status: Ongoing
interpretation_status: Blocked By Product Decision
source_requirement: |
  Create/modify roles; assign to user; open monitoring/logs
source_acceptance_criteria: |
  Role takes effect immediately; changes and admin actions logged; health metrics visible
effective_requirement: |
  Reduce DCS administration to the final identity model. With VC/OID4VP-first, administer
  role/policy mappings, audit, monitoring, trust/status data, and configuration, not central
  user accounts. With central user management, weaken or define the VC requirement as federation
  support.
effective_acceptance_criteria: |
  Admin changes are verifiable only within the chosen identity model. VC/OID4VP-first acceptance
  requires effective role/policy mappings, trust/status information, monitoring, and audit logs;
  central account creation/deactivation is not mandatory. Central-user acceptance requires
  immediate role/account effect and actor/time audit logs.
implementation_decision: |
  Decide central user management versus VC/OID4VP-only.
constraint_note: |
  Ambiguous requirement. On one hand "The system must use verifiable credentials for every
  login" - meaning DCS dictates no central user management, on the other hand UC-09 dictates
  user management interfaces.

### UC-10 - Contract Automation & Integration
id: UC-10
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Invoke HTTP node to start process; receive webhooks/callbacks; complete flow
source_acceptance_criteria: |
  End-to-end completes with 2xx responses; callback received; trace shows ordered events
effective_requirement: |
  Invoke HTTP node to start process; receive webhooks/callbacks; complete flow
effective_acceptance_criteria: |
  End-to-end completes with 2xx responses; callback received; trace shows ordered events

### UC-11 - API & System Integrations
id: UC-11
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Call API to create, sign, and validate; inspect responses
source_acceptance_criteria: |
  Auth succeeds; APIs return HTTP 2xx; validation result returned; request/response logs with
  correlation IDs
effective_requirement: |
  Call API to create, sign, and validate; inspect responses
effective_acceptance_criteria: |
  Auth succeeds; APIs return HTTP 2xx; validation result returned; request/response logs with
  correlation IDs
context_note: |
  signing ongoing, ontology not finished

### UC-12 - System-based Contract Management
id: UC-12
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Create -> review -> approve -> sign -> archive via API
source_acceptance_criteria: |
  Each step updates status; signature evidence stored; archive entry created; full audit chain
  present
effective_requirement: |
  Create -> review -> approve -> sign -> archive via API
effective_acceptance_criteria: |
  Each step updates status; signature evidence stored; archive entry created; full audit chain
  present
context_note: |
  no signature evidence stored

### UC-13 - External System Contract Execution
id: UC-13
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Submit execution payload; verify activation in target
source_acceptance_criteria: |
  Target confirms activation; DCS stores proof (receipt/hash/tx-id); status reflects "Executed"
effective_requirement: |
  Submit execution payload; verify activation in target
effective_acceptance_criteria: |
  Target confirms activation; DCS stores proof (receipt/hash/tx-id); status reflects "Executed"

### UC-14 - Identity & PoA Credential Acquisition
id: UC-14
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Fetch credentials; validate; bind to session
source_acceptance_criteria: |
  Credentials verified (valid, unrevoked); authorization check passes or blocks with reason;
  event logged
effective_requirement: |
  Fetch credentials; validate; bind to session
effective_acceptance_criteria: |
  Credentials verified (valid, unrevoked); authorization check passes or blocks with reason;
  event logged
context_note: |
  untested against real PKI

### UC-15 - Access Rights Revocation
id: UC-15
area: Use Cases
implementation_status: Ongoing
interpretation_status: Adjusted Depends On UC-09
source_requirement: |
  Revoke role/credential; attempt prior action (access/sign)
source_acceptance_criteria: |
  Access/signing denied; affected items flagged (if configured); revocation visible in
  logs/status lists
effective_requirement: |
  Implement revocation according to the final identity model. Central role management revokes
  DCS roles/access. VC/OID4VP-first revokes through credential/status-list checks, trust-list
  evaluation, and policy decisions without assuming central user accounts.
effective_acceptance_criteria: |
  After revocation, the previously allowed access/signing action is blocked and auditable. For
  central role management, role/access withdrawal is immediate. For VC/OID4VP-first,
  credential/status-list or trust-list policy evaluation blocks the action and records the
  result.
implementation_decision: |
  Keep revocation behavior consistent with UC-09 identity model.
constraint_note: |
  ambiguos requirement, see UC-09

### UC-02-01 - Create Contract Template
id: UC-02-01
area: Use Cases
implementation_status: Done
category: Sub-Use-Cases
interpretation_status: Unchanged
source_requirement: |
  Create reusable template by Template Manager/Approver.
source_acceptance_criteria: |
  Show a template is created with required metadata/provenance; repository returns a template
  ID/version; entry appears in search; action is auditlogged.
effective_requirement: |
  Create reusable template by Template Manager/Approver.
effective_acceptance_criteria: |
  Show a template is created with required metadata/provenance; repository returns a template
  ID/version; entry appears in search; action is auditlogged.

### UC-02-02 - Search & Retrieve Templates
id: UC-02-02
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Find and access existing templates.
source_acceptance_criteria: |
  Execute search with filters; results respect RBAC; opening a template displays current version
  & provenance; access events logged.
effective_requirement: |
  Find and access existing templates.
effective_acceptance_criteria: |
  Execute search with filters; results respect RBAC; opening a template displays current version
  & provenance; access events logged.

### UC-02-03 - Generate Contract from Template
id: UC-02-03
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Populate template with context data to create a contract.
source_acceptance_criteria: |
  Given inputs, system produces a draft with linked template ID; both machine- and
  human-readable versions render; creation logged
effective_requirement: |
  Populate template with context data to create a contract.
effective_acceptance_criteria: |
  Given inputs, system produces a draft with linked template ID; both machine- and
  human-readable versions render; creation logged
context_note: |
  changing process to new ontology

### UC-02-04 - Update Contract Template
id: UC-02-04
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Edit existing template with versioning.
source_acceptance_criteria: |
  Update creates a new immutable version; previous remains readable; diff and author shown;
  change logged.
effective_requirement: |
  Edit existing template with versioning.
effective_acceptance_criteria: |
  Update creates a new immutable version; previous remains readable; diff and author shown;
  change logged.

### UC-02-05 - Deprecate Contract Template
id: UC-02-05
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Mark a template deprecated.
source_acceptance_criteria: |
  Deprecation prevents new contract generation; banner shows status; event logged with timestamp
  and user.
effective_requirement: |
  Mark a template deprecated.
effective_acceptance_criteria: |
  Deprecation prevents new contract generation; banner shows status; event logged with timestamp
  and user.

### UC-02-06 - Add Template Provenance
id: UC-02-06
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Capture origin, contributors, identifiers.
source_acceptance_criteria: |
  Add provenance fields; system validates/records; provenance visible in UI/API and included in
  exports
effective_requirement: |
  Capture origin, contributors, identifiers.
effective_acceptance_criteria: |
  Add provenance fields; system validates/records; provenance visible in UI/API and included in
  exports
context_note: |
  TSA not yet verified, poa not yet verified

### UC-02-07 - Verify Template & Provenance
id: UC-02-07
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Validate correctness, semantics (JSONLD/SHACL) and authenticity.
source_acceptance_criteria: |
  Run verification; success report lists schema checks and signature/VC validation; failures
  block generation
effective_requirement: |
  Validate correctness, semantics (JSONLD/SHACL) and authenticity.
effective_acceptance_criteria: |
  Run verification; success report lists schema checks and signature/VC validation; failures
  block generation

### UC-02-08 - Create & Maintain Semantic Schemas
id: UC-02-08
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Manage schemas used by templates.
source_acceptance_criteria: |
  Create/update schema; link to templates; validation enforces conformity; schema versioning and
  rollback demonstrated.
effective_requirement: |
  Manage schemas used by templates.
effective_acceptance_criteria: |
  Create/update schema; link to templates; validation enforces conformity; schema versioning and
  rollback demonstrated.

### UC-02-09 - Template Management Dashboard
id: UC-02-09
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Track status, approvals, usage.
source_acceptance_criteria: |
  Dashboard shows per-template lifecycle, usage metrics, last changes; supports filtering and
  export; access controlled
effective_requirement: |
  Track status, approvals, usage.
effective_acceptance_criteria: |
  Dashboard shows per-template lifecycle, usage metrics, last changes; supports filtering and
  export; access controlled

### UC-03-01 - Create Contract
id: UC-03-01
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Generate contract from predefined templates.
source_acceptance_criteria: |
  Create a draft; receive contract ID; both views render; creation logged and traceable to
  template version.
effective_requirement: |
  Generate contract from predefined templates.
effective_acceptance_criteria: |
  Create a draft; receive contract ID; both views render; creation logged and traceable to
  template version.
context_note: |
  views not yet rendered

### UC-03-02 - Negotiate Contract Terms
id: UC-03-02
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Collaboratively adjust clauses before finalization.
source_acceptance_criteria: |
  Add comments/edits; see tracked changes and negotiation log; version history preserved.
effective_requirement: |
  Collaboratively adjust clauses before finalization.
effective_acceptance_criteria: |
  Add comments/edits; see tracked changes and negotiation log; version history preserved.
context_note: |
  changing to ontology

### UC-03-03 - Adjust Contract Terms
id: UC-03-03
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Granular clause edits without regenerating.
source_acceptance_criteria: |
  Edit a clause; integrity checks pass; only targeted sections change; audit trail updated.
effective_requirement: |
  Granular clause edits without regenerating.
effective_acceptance_criteria: |
  Edit a clause; integrity checks pass; only targeted sections change; audit trail updated.
context_note: |
  changing to ontology

### UC-03-04 - Approve Contract
id: UC-03-04
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Route to required approvers before signing.
source_acceptance_criteria: |
  Route shows pending/approved states; all required approvals recorded with timestamps; content
  locked on completion.
effective_requirement: |
  Route to required approvers before signing.
effective_acceptance_criteria: |
  Route shows pending/approved states; all required approvals recorded with timestamps; content
  locked on completion.

### UC-03-05 - Review MR/HR Correctness & Versions
id: UC-03-05
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Validate machine- and human-readable consistency.
source_acceptance_criteria: |
  Open both renderings; system highlights inconsistencies (none expected after fix); export both
  with same version/tag
effective_requirement: |
  Validate machine- and human-readable consistency.
effective_acceptance_criteria: |
  Open both renderings; system highlights inconsistencies (none expected after fix); export both
  with same version/tag
context_note: |
  changing to ontology, renderer already implemented

### UC-03-06 - Manage Contract Signing Process
id: UC-03-06
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Coordinate structured signing steps.
source_acceptance_criteria: |
  Configure signers/sequence; system schedules, reminds, and tracks status; changes logged.
effective_requirement: |
  Coordinate structured signing steps.
effective_acceptance_criteria: |
  Configure signers/sequence; system schedules, reminds, and tracks status; changes logged.
context_note: |
  parallel signing in pades unsupported by design if PDF/A3

### UC-03-07 - Contract Dashboard & Search
id: UC-03-07
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Track progress and search contracts.
source_acceptance_criteria: |
  Dashboard shows lifecycle states; full-text and metadata search returns expected contracts
  respecting RBAC.
effective_requirement: |
  Track progress and search contracts.
effective_acceptance_criteria: |
  Dashboard shows lifecycle states; full-text and metadata search returns expected contracts
  respecting RBAC.

### UC-04-01 - Review & Sign Contract Electronically
id: UC-04-01
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Secure viewer; legally binding e-signature incl. identity/PoA.
source_acceptance_criteria: |
  Signer authenticates; signs in viewer; system produces signed artifact (e.g., PAdES/JAdES) and
  updates status; event logged
effective_requirement: |
  Secure viewer; legally binding e-signature incl. identity/PoA.
effective_acceptance_criteria: |
  Signer authenticates; signs in viewer; system produces signed artifact (e.g., PAdES/JAdES) and
  updates status; event logged
context_note: |
  DCS not yet signing Pades. Acroforms exist in pdf.

### UC-04-02 - Verify Counterparty Authorization
id: UC-04-02
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Check legal authority to sign (identity/PoA).
source_acceptance_criteria: |
  Present counterparty VC/PoA; system validates chains/status lists; unauthorized cases block
  signing with error.
effective_requirement: |
  Check legal authority to sign (identity/PoA).
effective_acceptance_criteria: |
  Present counterparty VC/PoA; system validates chains/status lists; unauthorized cases block
  signing with error.
context_note: |
  proper PoA "chaining" unimplemented due to conflicting statements, settled on 1-hop chain
  (Issuer -> Holder) simplified model.

### UC-04-03 - Verify Counterparty Signature
id: UC-04-03
area: Use Cases
implementation_status: Ongoing
interpretation_status: Adjusted External Dependency
source_requirement: |
  Validate authenticity/integrity of signature.
source_acceptance_criteria: |
  Run crypto validation and policy checks; report shows certificate/VC status and document hash
  match.
effective_requirement: |
  DCS can verify PDF integrity, document hash match, and VC status retrieval. Full status-list
  validation remains limited until the external status-list encoding issue is fixed.
effective_acceptance_criteria: |
  Validation report shows PDF integrity, document hash match, and successful VC status
  retrieval. Status-list validation may be marked limited/blocked until a correctly encoded
  compatible status-list response exists.
implementation_decision: |
  Fix status-list encoding or validate against a compatible status-list implementation.
constraint_note: |
  pdf integrity matched, VC status retrieved - statuslist service however encoding in wrong bit
  order, see ticket

### UC-05-01 - Deploy Signed Contract to Target System
id: UC-05-01
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Make signed contract available for execution.
source_acceptance_criteria: |
  Push deployment payload to target; receive ack/callback; target reads content; DCS logs proof
  of delivery.
effective_requirement: |
  Make signed contract available for execution.
effective_acceptance_criteria: |
  Push deployment payload to target; receive ack/callback; target reads content; DCS logs proof
  of delivery.

### UC-06-01 - Monitor Contract Performance
id: UC-06-01
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  SLA/compliance monitoring & tracking.
source_acceptance_criteria: |
  Dashboard displays KPIs/milestones; alerts fire for violations; history shows fulfilled terms.
effective_requirement: |
  SLA/compliance monitoring & tracking.
effective_acceptance_criteria: |
  Dashboard displays KPIs/milestones; alerts fire for violations; history shows fulfilled terms.

### UC-06-02 - Renewal or Termination
id: UC-06-02
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Manage renewal/termination incl. VC revocation.
source_acceptance_criteria: |
  Trigger renewal/termination; system updates state, issues/updates revocation where applicable;
  notifications/logs produced
effective_requirement: |
  Manage renewal/termination incl. VC revocation.
effective_acceptance_criteria: |
  Trigger renewal/termination; system updates state, issues/updates revocation where applicable;
  notifications/logs produced

### UC-07-01 - Store Contract in Secure Archive
id: UC-07-01
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Tamper-proof, longterm storage of signed contracts.
source_acceptance_criteria: |
  Archive a signed contract; system seals, timestamps, encrypts, and returns archive ID;
  retrieval confirms integrity.
effective_requirement: |
  Tamper-proof, longterm storage of signed contracts.
effective_acceptance_criteria: |
  Archive a signed contract; system seals, timestamps, encrypts, and returns archive ID;
  retrieval confirms integrity.

### UC-07-02 - Manage Contract Permissions & Access
id: UC-07-02
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Control RBAC for stored contracts.
source_acceptance_criteria: |
  Change access policy; unauthorized access is denied; permitted roles can retrieve; changes and
  access attempts logged.
effective_requirement: |
  Control RBAC for stored contracts.
effective_acceptance_criteria: |
  Change access policy; unauthorized access is denied; permitted roles can retrieve; changes and
  access attempts logged.

### UC-07-03 - Storage & Security Dashboard
id: UC-07-03
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Track archive status, integrity, access logs.
source_acceptance_criteria: |
  Dashboard shows coverage/integrity checks, alerts, and recent access; export of logs
  available.
effective_requirement: |
  Track archive status, integrity, access logs.
effective_acceptance_criteria: |
  Dashboard shows coverage/integrity checks, alerts, and recent access; export of logs
  available.

### UC-08-01 - Report Contract Activity Logs & Timestamps
id: UC-08-01
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Produce auditable reports.
source_acceptance_criteria: |
  Generate report with creation/approval/signature events; includes timestamps/actors; export to
  CSV/PDF.
effective_requirement: |
  Produce auditable reports.
effective_acceptance_criteria: |
  Generate report with creation/approval/signature events; includes timestamps/actors; export to
  CSV/PDF.
context_note: |
  export missing

### UC-08-02 - Audit Contract Compliance
id: UC-08-02
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Check against legal/organizational policies.
source_acceptance_criteria: |
  Run compliance scan; issues listed with rule references; pass/fail summary produced and
  archived.
effective_requirement: |
  Check against legal/organizational policies.
effective_acceptance_criteria: |
  Run compliance scan; issues listed with rule references; pass/fail summary produced and
  archived.

### UC-08-02 - System Configuration & User Management
id: UC-08-02
area: Use Cases
implementation_status: Not Started
interpretation_status: Blocked By Product Decision
source_requirement: |
  Administer roles, security, settings.
source_acceptance_criteria: |
  Admin changes a role/setting; effect visible immediately; all admin actions logged with
  actor/time.
effective_requirement: |
  System configuration remains mandatory. User management is mandatory only to the extent
  compatible with the UC-09 identity model. With VC/OID4VP-only, implement role, policy,
  trust-list, and configuration management instead of central user accounts.
effective_acceptance_criteria: |
  System configuration changes are immediately effective and audited with actor/time.
  User-management acceptance follows UC-09: either role/policy/trust-list administration for
  VC/OID4VP-only or immediate role/account effect for central user management.
implementation_decision: |
  Resolve with UC-09 identity model. SRS Table 7 maps this criterion to UC-09-01; the matrix row
  is labelled UC-08-02.
constraint_note: |
  ambiguos requirement, see UC-09

### UC-09-02 - System Monitoring & Logging
id: UC-09-02
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Operational monitoring and security logs.
source_acceptance_criteria: |
  Show health metrics & searchable logs; filters by severity/time; export supports incident
  review.
effective_requirement: |
  Operational monitoring and security logs.
effective_acceptance_criteria: |
  Show health metrics & searchable logs; filters by severity/time; export supports incident
  review.
context_note: |
  prometheus

### UC-10-01 - Automate Contract Workflow Processes
id: UC-10-01
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Integrate workflows with AI/ERP (orchestration).
source_acceptance_criteria: |
  Orchestrator triggers external action from a contract milestone; target system receives and
  executes; trace visible end-to-end.
effective_requirement: |
  Integrate workflows with AI/ERP (orchestration).
effective_acceptance_criteria: |
  Orchestrator triggers external action from a contract milestone; target system receives and
  executes; trace visible end-to-end.

### UC-10-02 - Validate Contract Integrity & Compliance
id: UC-10-02
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Pre-execution automated validation.
source_acceptance_criteria: |
  Run validation; violations block deployment; detailed report stored with contract.
effective_requirement: |
  Pre-execution automated validation.
effective_acceptance_criteria: |
  Run validation; violations block deployment; detailed report stored with contract.

### UC-11-01 - Manage API-Based Contract Workflows
id: UC-11-01
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Ensure automation via API integrations.
source_acceptance_criteria: |
  Invoke API to create/sign/validate; request is authenticated; workflow executes; interaction
  logged for traceability.
effective_requirement: |
  Ensure automation via API integrations.
effective_acceptance_criteria: |
  Invoke API to create/sign/validate; request is authenticated; workflow executes; interaction
  logged for traceability.
context_note: |
  all access is API based, signing/validation missing

### UC-12-01 - Create Contract via API
id: UC-12-01
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Automated contract creation through system integration.
source_acceptance_criteria: |
  POST creates contract; returns ID & status; data pulled from integrated system; audit log
  records requester/system
effective_requirement: |
  Automated contract creation through system integration.
effective_acceptance_criteria: |
  POST creates contract; returns ID & status; data pulled from integrated system; audit log
  records requester/system

### UC-12-02 - Review Contract via API
id: UC-12-02
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  System-driven validation checks.
source_acceptance_criteria: |
  API call runs rule checks; response lists issues; failing contracts cannot proceed to approval
effective_requirement: |
  System-driven validation checks.
effective_acceptance_criteria: |
  API call runs rule checks; response lists issues; failing contracts cannot proceed to approval

### UC-12-03 - Approve Contract via API
id: UC-12-03
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Automated approvals.
source_acceptance_criteria: |
  API marks contract approved; requires authN/authZ; status changes and approver identity
  logged.
effective_requirement: |
  Automated approvals.
effective_acceptance_criteria: |
  API marks contract approved; requires authN/authZ; status changes and approver identity
  logged.

### UC-12-04 - Manage Contracts via API
id: UC-12-04
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Query/update/track lifecycle.
source_acceptance_criteria: |
  Use APIs to list/update metadata and read history; RBAC enforced; changes versioned and
  logged.
effective_requirement: |
  Query/update/track lifecycle.
effective_acceptance_criteria: |
  Use APIs to list/update metadata and read history; RBAC enforced; changes versioned and
  logged.

### UC-12-05 - Sign Contract via API
id: UC-12-05
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Automated/AI-driven signing.
source_acceptance_criteria: |
  Sign endpoint produces valid signature artifact; binds signer VC/PoA; status -> "signed";
  verification succeeds.
effective_requirement: |
  Automated/AI-driven signing.
effective_acceptance_criteria: |
  Sign endpoint produces valid signature artifact; binds signer VC/PoA; status -> "signed";
  verification succeeds.

### UC-13-01 - Deploy Contract to Target System
id: UC-13-01
area: Use Cases
implementation_status: Not Started
interpretation_status: Unchanged
source_requirement: |
  Execute in ERP targets.
source_acceptance_criteria: |
  Deliver deployment payload; target confirms activation/execution; DCS records
  proof-of-execution reference.
effective_requirement: |
  Execute in ERP targets.
effective_acceptance_criteria: |
  Deliver deployment payload; target confirms activation/execution; DCS records
  proof-of-execution reference.

### UC-14-01 - Retrieve Identity & PoA Credentials
id: UC-14-01
area: Use Cases
implementation_status: Ongoing
interpretation_status: Unchanged
source_requirement: |
  Acquire verified identity/PoA before signing/execution.
source_acceptance_criteria: |
  Missing credentials trigger retrieval; chain and status verified; authorization granted only
  on success; events logged.
effective_requirement: |
  Acquire verified identity/PoA before signing/execution.
effective_acceptance_criteria: |
  Missing credentials trigger retrieval; chain and status verified; authorization granted only
  on success; events logged.
context_note: |
  untested with real PKI

### UC-15-01 - Revoke Access Rights & Signatures
id: UC-15-01
area: Use Cases
implementation_status: Done
interpretation_status: Unchanged
source_requirement: |
  Invalidate access when credentials/signatures are revoked.
source_acceptance_criteria: |
  Detect revocation via status list; mark contract state "revoked"; rights withdrawn; audit
  entry created; requires re-sign to restore.
effective_requirement: |
  Invalidate access when credentials/signatures are revoked.
effective_acceptance_criteria: |
  Detect revocation via status list; mark contract state "revoked"; rights withdrawn; audit
  entry created; requires re-sign to restore.
context_note: |
  statuslist is checked
