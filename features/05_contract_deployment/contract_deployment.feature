# Contract deployment, execution evidence, and KPIs (SRS: DCS-FR-SM-10/-12,
# DCS-FR-CWE-06/-09/-20/-31, DCS-IR-SI-02/-05).
#
# Endpoint surface exercised here:
#   - POST /contract/deploy (manual deploy trigger, UC-05-01)
#   - POST /contract/deployment/callback (target -> DCS ack/status/KPI,
#     protected by a shared-secret header)
#   - GET /contract/retrieve/{did} "kpis" field
#   - archive entries (GET /archive/search) with an
#     evidence.deployment{correlation_id, payload_hash, receipt_hash,
#     tsa_token, activated_at} sub-object
# See steps/contract_deployment/dcs_contract_deployment_steps.py's module
# docstring for the force-set DB test seam and the ORCE contract-target-flow
# wiring.

@DCS-FR-UC-05-1 @DCS-FR-UC-13-1
Feature: Contract deployment, execution evidence, and KPIs

  @DCS-FR-CWE-20
  Scenario: Archive entry is created only when the contract reaches SIGNED, not at APPROVED
    Given contract "Archive Trigger Contract" has reached contract state "APPROVED"
    Then the archive has no entry for contract "Archive Trigger Contract"
    When the counterparty signer applies a signature to contract "Archive Trigger Contract"
    Then get http 200:Success code
    And the archive has an entry for contract "Archive Trigger Contract"

  @DCS-FR-CWE-31 @DCS-FR-CWE-20
  Scenario: An archived contract in state ACTIVE still appears in the live contract list
    Given I am authenticated with roles: "Contract Manager"
    And contract "Live Archived Contract" has reached contract state "SIGNED"
    And contract "Live Archived Contract" is force-set to state "ACTIVE" directly in the database (pre-deploy test seam, bypassing the deployment chain)
    When the contract search endpoint is queried with state filter "ACTIVE"
    Then the search results include contract "Live Archived Contract"
    And the archive has an entry for contract "Live Archived Contract"

  @DCS-FR-SM-12 @UC-05-01
  Scenario: An authorized user deploys a SIGNED contract to the configured Contract Target System
    Given contract "Deploy Signed Contract" has reached contract state "SIGNED"
    When an authorized user deploys contract "Deploy Signed Contract" to the configured contract target
    Then get http 200:Success code
    And the deployment response includes a correlation ID

  @DCS-FR-SM-12 @DCS-IR-SI-05
  Scenario: The deployment payload declares the machine-readable JSON-LD, DID, version, hash, timestamp, and odrl:Set
    Given contract "Deploy Payload Contract" has reached contract state "SIGNED"
    When an authorized user deploys contract "Deploy Payload Contract" to the configured contract target
    Then get http 200:Success code
    And the deployment response declares the contract DID, version, content hash, timestamp, and the odrl:Set policy for "Deploy Payload Contract"

  @DCS-NFR-BR-03 @DCS-FR-SM-12
  Scenario: A contract that is not SIGNED is rejected for deployment
    Given contract "Draft Deploy Rejection Contract" has reached contract state "DRAFT"
    When an authorized user deploys contract "Draft Deploy Rejection Contract" to the configured contract target
    Then the request is denied with a client error

  @DCS-FR-CWE-06
  Scenario: Deployment is triggered automatically once the signing workflow completes
    Given contract "Auto Deploy Contract" has reached contract state "APPROVED"
    When the counterparty signer applies a signature to contract "Auto Deploy Contract"
    Then get http 200:Success code
    And the archive entry for contract "Auto Deploy Contract" records an automatic deployment correlation ID

  @DCS-IR-SI-05
  Scenario: The deployment callback rejects a request without a valid shared secret
    Given contract "Callback Auth Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "Callback Auth Contract" to the configured contract target
    And get http 200:Success code
    When the target sends a deployment callback for contract "Callback Auth Contract" with an invalid shared secret
    Then the callback request is rejected for the missing or invalid shared secret

  @DCS-IR-SI-02 @DCS-IR-SI-05
  Scenario: The shipped ORCE contract-target-flow verifies the content hash and returns a matching ack
    Given contract "ORCE Ack Contract" has reached contract state "SIGNED"
    And the example ORCE contract-target-flow is reachable
    When a deployment payload for contract "ORCE Ack Contract" is posted directly to the ORCE contract-target-flow
    Then the ORCE flow acknowledges with correlation_id, payload_hash, and activated_at matching the sent payload

  @DCS-FR-SM-10
  Scenario: The execution-evidence receipt is TSA-timestamped and appended to the archive entry
    Given contract "TSA Evidence Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "TSA Evidence Contract" to the configured contract target
    And get http 200:Success code
    When the target sends a deployment acknowledgement for contract "TSA Evidence Contract" with the correct shared secret
    Then get http 200:Success code
    And the archive entry for contract "TSA Evidence Contract" contains an RFC-3161 TSA timestamp over the execution-evidence receipt

  @DCS-FR-SM-12
  Scenario: An acknowledged deployment moves the contract from SIGNED to ACTIVE
    Given contract "Ack Activates Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "Ack Activates Contract" to the configured contract target
    And get http 200:Success code
    When the target sends a deployment acknowledgement for contract "Ack Activates Contract" with the correct shared secret
    Then get http 200:Success code
    And the contract "Ack Activates Contract" is in state "ACTIVE"

  @DCS-FR-CWE-31 @DCS-FR-CWE-09
  Scenario: A KPI reported via callback for an ACTIVE contract appears on the contract detail
    Given contract "KPI Dashboard Contract" has reached contract state "SIGNED"
    And an authorized user deploys contract "KPI Dashboard Contract" to the configured contract target
    And get http 200:Success code
    And the target sends a deployment acknowledgement for contract "KPI Dashboard Contract" with the correct shared secret
    And get http 200:Success code
    When the target reports a KPI value "uptime_percent" = "99.5" for contract "KPI Dashboard Contract"
    Then get http 200:Success code
    And the contract detail for "KPI Dashboard Contract" shows KPI "uptime_percent" with value "99.5"

  @DCS-FR-CWE-09
  Scenario: A KPI that violates its contractual SLA threshold sets a violation flag
    Given contract "KPI Violation Contract" is a fresh draft whose ODRL policy constrains field "coverage" using operator "gteq" against "95" while the actual value is "95"
    And contract "KPI Violation Contract" is submitted, reviewed, approved, and signed via the standard workflow
    And an authorized user deploys contract "KPI Violation Contract" to the configured contract target
    And get http 200:Success code
    And the target sends a deployment acknowledgement for contract "KPI Violation Contract" with the correct shared secret
    And get http 200:Success code
    When the target reports a KPI value "coverage" = "80" for contract "KPI Violation Contract"
    Then get http 200:Success code
    And the contract detail for "KPI Violation Contract" shows a KPI violation flag for "coverage"
