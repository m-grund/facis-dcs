"""Fixture builders for the ODRL-soundness BDD scenarios
(features/18_odrl_soundness).

These build canonical `dcs:documentStructure`-enveloped contract documents
(see backend/internal/base/validation/documentdata.go `isCanonicalEnvelope`)
carrying either:

  - the legacy bare-Duty `dcs:policies` shape (flat array of
    `odrl:Duty`/`odrl:Permission`/`odrl:Prohibition` nodes, each with only an
    `odrl:constraint` — no `odrl:action`, no enclosing policy node, no
    parties/target), or
  - the canonical ODRL 2.2 shape (docs/adr-6-odrl-profile-enforcement.md):
    one enclosing `odrl:Agreement` (`uid` == the contract DID, `odrl:profile`
    declared),
    whose rules each carry exactly one `odrl:action` plus
    `odrl:assigner`/`odrl:assignee`/`odrl:target`.

Both shapes constrain the SAME field (`urn:uuid:field-provider-country`, a
string, or `urn:uuid:field-provider-coverage`, a number) so the same fixture
family can drive the structure, enforcement, operator-matrix, and
legacy-shape-rejection scenarios.
"""

FIELD_COUNTRY = "urn:uuid:field-provider-country"
FIELD_COVERAGE = "urn:uuid:field-provider-coverage"

_FIELD_BY_NAME = {
    "country": (FIELD_COUNTRY, "country", "string"),
    "coverage": (FIELD_COVERAGE, "coverage", "number"),
}


def _semantic_value(field_name: str, actual_value):
    field_id, parameter_name, _ = _FIELD_BY_NAME[field_name]
    return {
        "blockId": "block-clause-1",
        "conditionId": "provider",
        "parameterName": parameter_name,
        "parameterValue": actual_value,
    }


def _requirement_field(field_name: str) -> dict:
    field_id, parameter_name, value_type = _FIELD_BY_NAME[field_name]
    return {
        "@id": field_id,
        "@type": "dcs:RequirementField",
        "dcs:parameterName": parameter_name,
        "dcs:valueType": value_type,
        "dcs:required": True,
    }


def legacy_bare_duty_policies(field_name: str, operator: str, right_operand) -> list:
    """The shape the codebase emits/accepts TODAY (no action/parties/target,
    no enclosing Set) — bare `odrl:Duty` nodes each holding one constraint.
    """
    field_id, _, _ = _FIELD_BY_NAME[field_name]
    return [
        {
            "@id": f"urn:uuid:policy-{field_name}-0",
            "@type": "odrl:Duty",
            "odrl:constraint": {
                "@type": "odrl:Constraint",
                "odrl:leftOperand": {"@id": field_id},
                "odrl:operator": {"@id": f"odrl:{operator}"},
                "odrl:rightOperand": right_operand,
            },
        }
    ]


def odrl_set_policies(
    contract_did: str,
    field_name: str,
    operator: str,
    right_operand,
    rule_type: str = "odrl:Duty",
) -> dict:
    """The canonical ODRL 2.2 shape: one enclosing `odrl:Agreement` (`uid` ==
    contract DID), `odrl:profile` declared, rule carries exactly one
    `odrl:action` plus assigner/assignee/target.
    """
    field_id, _, _ = _FIELD_BY_NAME[field_name]
    rule_bucket = {
        "odrl:Duty": "odrl:obligation",
        "odrl:Permission": "odrl:permission",
        "odrl:Prohibition": "odrl:prohibition",
    }[rule_type]
    rule = {
        "@id": f"urn:uuid:policy-{field_name}-0",
        "@type": rule_type,
        "odrl:action": {"@id": "dcs:provideCompliantValue"},
        "odrl:assigner": {"@id": "did:web:example.org%3A9001:bdd-provider-org"},
        "odrl:assignee": {"@id": "did:web:example.org%3A9002:bdd-customer-org"},
        "odrl:target": {"@id": contract_did},
        "odrl:constraint": {
            "@type": "odrl:Constraint",
            "odrl:leftOperand": {"@id": field_id},
            "odrl:operator": {"@id": f"odrl:{operator}"},
            "odrl:rightOperand": right_operand,
        },
    }
    return {
        "@id": "urn:uuid:policy-set-1",
        "@type": "odrl:Agreement",
        "uid": contract_did,
        "odrl:profile": {"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
        rule_bucket: [rule],
    }


def build_contract_document(contract_did: str, field_name: str, policies, actual_value) -> dict:
    """A full canonical `dcs:Contract` document (documentStructure +
    contractData + semanticConditionValues + policies) suitable for a full
    replacement PUT to /contract/update while the contract is in DRAFT.
    """
    field_id, _, _ = _FIELD_BY_NAME[field_name]
    return {
        "@context": {
            "dcs": "https://w3id.org/facis/dcs/ontology/v1#",
            "odrl": "http://www.w3.org/ns/odrl/2/",
        },
        "@type": "dcs:Contract",
        "dcs:metadata": {
            "@type": "dcs:ContractMetadata",
            "dcs:title": "BDD ODRL Soundness Contract",
        },
        "dcs:documentStructure": {
            "@type": "dcs:DocumentStructure",
            "dcs:blocks": {
                "@list": [
                    {
                        "@id": "urn:uuid:block-clause-1",
                        "@type": "dcs:Clause",
                        "dcs:content": {
                            "@list": [
                                f"Provider {field_name}: ",
                                {
                                    "@type": "dcs:Placeholder",
                                    "dcs:token": f"{{{{provider.{field_name}}}}}",
                                    "dcs:bindsTo": {"@id": field_id},
                                },
                            ]
                        },
                    }
                ]
            },
            "dcs:layout": [
                {
                    "@id": "urn:uuid:block-root",
                    "dcs:isRoot": True,
                    "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]},
                }
            ],
        },
        "dcs:contractData": [
            {
                "@id": "urn:uuid:requirement-provider",
                "@type": "dcs:DataRequirement",
                "dcs:conditionId": "provider",
                "dcs:name": "Provider",
                "dcs:schemaVersion": "v1",
                "dcs:entityType": "CompanyParty",
                "dcs:entityRole": "provider",
                "dcs:fields": [_requirement_field(field_name)],
            }
        ],
        "semanticConditionValues": [_semantic_value(field_name, actual_value)],
        "dcs:policies": policies,
    }


def extract_policy_rules(policies) -> list:
    """Flattens either policy shape into a plain list of rule dicts.

    - Legacy shape: `policies` IS already the flat list of rules.
    - Target policy shape: `policies` is a single dict; rules live under
      `odrl:permission` / `odrl:prohibition` / `odrl:obligation`
      array properties.
    """
    if isinstance(policies, list):
        return [item for item in policies if isinstance(item, dict)]
    if isinstance(policies, dict):
        rules = []
        for key in ("odrl:permission", "odrl:prohibition", "odrl:obligation"):
            bucket = policies.get(key)
            if isinstance(bucket, list):
                rules.extend(item for item in bucket if isinstance(item, dict))
            elif isinstance(bucket, dict):
                rules.append(bucket)
        return rules
    return []
