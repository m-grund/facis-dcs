@FR-UC-01-1 @FR-UC-01-3 @FR-UC-01-4
Feature: User Authentication & Authorization
  Users authenticate securely and are authorized based on roles and credentials.

  Scenario: Authorization denied for expired credential
    Given I hold an expired credential with roles: "Template Creator"
    When I try to create a template "Standard NDA" in category "Legal"
    Then the request is denied because of credential expiration
  #  And the attempt is logged for audit
 
  Scenario: Role enforcement prevents unauthorized actions
    Given I am authenticated with roles: "Contract Creator"
    When I try to create a template "Standard NDA" in category "Legal"
    Then the request is denied with an authorization error
  #  And the denial is logged with timestamp and actor identity

  @skip
  Scenario: PoA credential validation for signing
    Given I am authenticated with roles: "Contract Signer"
    And I hold a valid PoA credential for organization "Example Corp"
    When I initiate a signing process
    Then the PoA credential is validated
    And signing proceeds if authorized

  @skip
  Scenario: Revoked credential blocks access
    Given my credential has been revoked via XFSC Revocation List
    When I attempt to access the DCS system
    Then the request is denied
    And access rights are invalidated until re-credentialing

  # FR-UC-01-4: Multiple invalid credential attempts trigger lockout
  @skip
  Scenario: Multiple failed credential attempts trigger account lockout
    Given I hold an invalid verifiable credential
    When I fail authentication 5 consecutive times
    Then my account is locked
    And I receive error "Account locked due to multiple failed attempts"
    And the lockout event is logged with timestamp and actor identity

  @skip
  Scenario: Locked account cannot authenticate even with valid credential
    Given my account has been locked due to failed attempts
    When I hold a valid verifiable credential
    And I attempt to access the DCS system
    Then the request is denied with error "Account locked"
    And I am instructed to contact an administrator

  @skip
  Scenario: Administrator can unlock a locked account
    Given user "locked.user@example.com" has a locked account
    And I am authenticated with roles: "System Administrator"
    When I unlock the account for user "locked.user@example.com"
    Then the account is unlocked
    And the user can authenticate with valid credentials
    And the unlock action is logged with timestamp and actor identity

  @skip
  Scenario: Wallet integration failure allows retry
    Given I am authenticated with roles: "Contract Signer"
    And my wallet integration encounters a temporary failure
    When I attempt to sign a contract
    Then the system notifies me of the wallet failure
    And I am offered the option to retry
    And the failure is logged with timestamp and actor identity