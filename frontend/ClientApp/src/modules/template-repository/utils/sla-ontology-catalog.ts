/**
 * Single source for ODRL 2.2 action IRIs used when the editor emits ODRL
 * rules (see docs/adr-6-odrl-profile-enforcement.md).
 *
 * Every `OdrlRule` built by dcsDraftStore carries exactly one `odrl:action`.
 * For field-value constraints generated from semantic conditions/parameters
 * (the common case in the template/contract editor), the DCS ODRL profile
 * action `dcs:provideCompliantValue` applies (see
 * docs/adr-6-odrl-profile-enforcement.md). Where a clause instead
 * expresses a standard ODRL usage/spatial/temporal permission directly, the
 * corresponding standard-vocabulary action IRI should be preferred over the
 * DCS-specific one.
 *
 * This file intentionally does not yet catalogue SLA metric IRIs/operators/
 * units (see CLAUDE.md) — only the ODRL action-IRI slice needed by the
 * editor has been populated so far; extending it into the full planned
 * SLA/ODRL vocabulary catalogue is an open follow-up.
 */

/** The DCS ODRL profile action for "the bound contract-data field value complies with the declared constraint". */
export const DCS_ODRL_ACTION_PROVIDE_COMPLIANT_VALUE = 'dcs:provideCompliantValue' as const

/** Standard ODRL 2.2 actions preferred over the DCS-specific one where they apply directly. */
export const ODRL_ACTION_USE = 'odrl:use' as const
export const ODRL_ACTION_SPATIAL = 'odrl:spatial' as const // NOTE: odrl:spatial is a left-operand term, retained here only as a documented non-default; prefer ODRL_ACTION_USE for spatial permissions.
export const ODRL_ACTION_DATETIME = 'odrl:dateTime' as const // NOTE: same caveat as ODRL_ACTION_SPATIAL — retained for documentation purposes.

/** The action IRI dcsDraftStore attaches to every generated field-constraint ODRL rule. */
export const DEFAULT_FIELD_CONSTRAINT_ACTION = DCS_ODRL_ACTION_PROVIDE_COMPLIANT_VALUE

/** The DCS ODRL profile IRI declared as `odrl:profile` on every enclosing `odrl:Set`. */
export const DCS_ODRL_PROFILE_IRI = 'https://w3id.org/facis/dcs/ontology/v1/odrl-profile' as const
