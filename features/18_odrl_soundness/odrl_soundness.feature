# Requirement: odrl-soundness
#
# Covers Workstream F ("Machine-readable contract soundness: real ODRL +
# server-side enforcement", docs/anforderung.md) — only the ACs the analyst
# marked Pruefmittel = BDD:
#
#   AC1 — all rules of a document inside ONE enclosing odrl:Set
#         (uid = the contract DID), odrl:profile declared.
#   AC2 — every rule carries exactly one odrl:action.
#   AC3 — every rule carries odrl:assigner/odrl:assignee + odrl:target.
#   AC4 — a contract with a violated constraint cannot be approved.
#   AC5 — a contract with a violated constraint cannot be signed, even via a
#         direct raw API call.
#   AC6 — server-side operator evaluation covers all 8 operators correctly
#         (eq, neq, isAnyOf, isNoneOf, gteq, lteq, gt, lt).
#   AC7 — a contract with SATISFIED constraints is not falsely rejected
#         (positive counter-test to AC4/AC5).
#   AC8 — the legacy bare-Duty policy shape (no action, no enclosing Set) is
#         explicitly rejected by structural validation.
#
# Deliberately OUT of scope for this pack:
#   - AC9  (manueller-Drill)
#   - AC10 (extern-validiert)
#   - AC11 (extern-validiert)
#   - AC12 (Pruefmittel = BDD, but the analyst marked it explicitly
#           "nice-to-have, nur falls Kapazitaet" — DCS-to-DCS info-endpoint /
#           FR-SM-12 deployment-payload verbatim-propagation of the odrl:Set
#           is not exercised here; add it once F1-F4 land and capacity allows,
#           tagged @REQ-odrl-soundness-AC12).
#
# AC1/AC2/AC3/AC4/AC5/AC7 deliberately build their fixtures against the
# TARGET odrl:Set-enclosed shape that Workstream F1 is supposed to introduce
# — NOT the flat-array shape the codebase emits/accepts today. This is
# intentional: `extractContractODRLPolicies`
# (backend/internal/base/validation/contractcontentaudit.go:897) reads
# `dcs:policies` as a flat array today; `validateCanonicalEnvelope`
# (backend/internal/base/validation/documentdata.go:335-338) additionally
# REJECTS anything that isn't an array right now, so every one of these
# scenarios is expected to be RED until Workstream F1 (emit the enclosing
# odrl:Set) and F2 (make extraction/evaluation understand it) land together.
# That is deliberate: testing AC4/AC5/AC7 against the NEW shape (not the old
# flat array) is what would catch the exact regression the architect flagged
# — if the implementer migrates the emitted shape but forgets to migrate the
# extraction in the same change, approve/apply would silently see zero
# policies and let everything through.
#
# AC6/AC8 deliberately use the CURRENT legacy flat-array shape instead:
#   - AC6 is about operator-evaluation correctness. That already works today
#     (server-side `evaluateODRLConstraint`,
#     backend/internal/base/validation/contractcontentaudit.go:1069)
#     independent of the enclosing-Set migration, and is additionally
#     covered end-to-end by the Go unit tests in
#     backend/internal/base/validation/contractcontentaudit_test.go — this
#     Scenario Outline is legitimately expected to be GREEN already.
#   - AC8 is specifically about the legacy shape being REJECTED once F1/F3
#     land; today it is accepted (that is the whole point of the gap), so
#     this scenario is expected to be RED until F3's structural validation
#     update ships.
#
# AC4 covers the regular UI/API entry path only. Per the architect's note,
# the peer-action entry path dispatches through the SAME
# command.Approver.Handle used by the UI/API path
# (backend/internal/service/dcs_to_dcs.go:201-206), so it is architecturally
# covered by this same enforcement point rather than re-tested with a
# separate peer-action scenario here.
#
# AC5's "direct raw API call" scenario attempts a sign call on the contract
# BEFORE it is approved (contract state REVIEWED). This is deliberate: once
# AC4 holds, a constraint-violating contract can never legitimately reach
# APPROVED (the only state /signature/apply accepts,
# backend/internal/signingmanagement/command/apply.go:75), so a raw sign
# attempt against it — at any point before or at approval — must always be
# rejected. If a future change to F1/F2 regresses AC4 (the extraction bug
# above) such that a violating contract DOES reach APPROVED, this AC5
# scenario would need to be re-pointed at that (illegitimately) approved
# contract to keep proving independent enforcement on the signing path; that
# is an open point for the verifier to re-check once F1/F2 land.

@DCS-FR-PACM-03
Feature: Machine-readable ODRL soundness and server-side policy enforcement

  @REQ-odrl-soundness-AC1 @REQ-odrl-soundness-AC2 @REQ-odrl-soundness-AC3 @DCS-FR-PACM-03
  Scenario: A contract's ODRL policies form one enclosing Set with profile, action, parties, and target
    Given a fresh draft contract "ODRL Structure Contract"
    When the policies of contract "ODRL Structure Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Structure Contract" is accepted
    And the stored policies of contract "ODRL Structure Contract" form a single enclosing odrl:Set whose uid equals the contract DID and which declares an odrl:profile
    And every stored policy rule of contract "ODRL Structure Contract" declares exactly one odrl:action
    And every stored policy rule of contract "ODRL Structure Contract" declares an odrl:assigner, odrl:assignee, and odrl:target

  @REQ-odrl-soundness-AC4 @DCS-FR-PACM-03
  Scenario: A contract with a violated ODRL constraint cannot be approved
    Given a fresh draft contract "ODRL Violation Contract"
    When the policies of contract "ODRL Violation Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "FRA"
    Then the policy update for contract "ODRL Violation Contract" is accepted
    When approval is attempted for contract "ODRL Violation Contract"
    Then the approval is rejected because an ODRL constraint is violated

  @REQ-odrl-soundness-AC5 @DCS-FR-PACM-03
  Scenario: A contract with a violated ODRL constraint cannot be signed via a direct raw API call
    Given a fresh draft contract "ODRL Raw Sign Contract"
    When the policies of contract "ODRL Raw Sign Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "FRA"
    Then the policy update for contract "ODRL Raw Sign Contract" is accepted
    When a direct signing API call is attempted against contract "ODRL Raw Sign Contract" before it is approved
    Then the sign attempt for contract "ODRL Raw Sign Contract" is rejected and the contract remains unsigned

  @REQ-odrl-soundness-AC7 @DCS-FR-PACM-03
  Scenario: A contract with satisfied ODRL constraints is approved and signed normally
    Given a fresh draft contract "ODRL Satisfied Contract"
    When the policies of contract "ODRL Satisfied Contract" are updated to a real ODRL 2.2 policy set (rule "Duty", field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Satisfied Contract" is accepted
    When the contract "ODRL Satisfied Contract" is submitted, reviewed, approved, and signed via the standard workflow
    Then the contract "ODRL Satisfied Contract" reaches SIGNED state

  @REQ-odrl-soundness-AC8 @DCS-FR-PACM-03
  Scenario: The legacy bare-Duty policy shape is rejected by structural validation
    Given a fresh draft contract "ODRL Legacy Shape Contract"
    When the policies of contract "ODRL Legacy Shape Contract" are updated to the legacy bare-Duty form (field "country", operator "isAnyOf") requiring "DEU,AUT,CHE" while the actual value is "DEU"
    Then the policy update for contract "ODRL Legacy Shape Contract" is rejected because the legacy bare-Duty form lacks an action and enclosing policy

  @REQ-odrl-soundness-AC6 @DCS-FR-PACM-03
  Scenario Outline: Server-side operator evaluation covers all 8 ODRL operators correctly
    Given contract "<name>" is a fresh draft whose ODRL policy constrains field "<field>" using operator "<operator>" against "<right_operand>" while the actual value is "<actual_value>"
    When approval is attempted for contract "<name>"
    Then the approval outcome for contract "<name>" is "<expect>"

    Examples: string operators
      | name                       | field   | operator | right_operand | actual_value | expect    |
      | ODRL Operator eq sat       | country | eq       | DEU            | DEU          | satisfied |
      | ODRL Operator eq viol      | country | eq       | DEU            | FRA          | violated  |
      | ODRL Operator neq sat      | country | neq      | DEU            | FRA          | satisfied |
      | ODRL Operator neq viol     | country | neq      | DEU            | DEU          | violated  |
      | ODRL Operator isAnyOf sat  | country | isAnyOf  | DEU,AUT,CHE    | AUT          | satisfied |
      | ODRL Operator isAnyOf viol | country | isAnyOf  | DEU,AUT,CHE    | FRA          | violated  |
      | ODRL Operator isNoneOf sat | country | isNoneOf | DEU,AUT,CHE    | FRA          | satisfied |
      | ODRL Operator isNoneOf viol| country | isNoneOf | DEU,AUT,CHE    | DEU          | violated  |

    Examples: numeric operators
      | name                       | field    | operator | right_operand | actual_value | expect    |
      | ODRL Operator gt sat       | coverage | gt       | 95             | 99           | satisfied |
      | ODRL Operator gt viol      | coverage | gt       | 95             | 90           | violated  |
      | ODRL Operator gteq sat     | coverage | gteq     | 95             | 95           | satisfied |
      | ODRL Operator gteq viol    | coverage | gteq     | 95             | 90           | violated  |
      | ODRL Operator lt sat       | coverage | lt       | 5              | 2            | satisfied |
      | ODRL Operator lt viol      | coverage | lt       | 5              | 8            | violated  |
      | ODRL Operator lteq sat     | coverage | lteq     | 5              | 5            | satisfied |
      | ODRL Operator lteq viol    | coverage | lteq     | 5              | 8            | violated  |
