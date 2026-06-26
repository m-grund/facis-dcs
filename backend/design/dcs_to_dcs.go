package design

import (
	. "goa.design/goa/v3/dsl"
)

var DCSToDCSContractItem = Type("DCSToDCSContractItem", func() {
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "Current state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "The description of the contract")
	Attribute("created_by", String, "Identifier of who created the contract")
	Attribute("created_at", String, "Created at")
	Attribute("updated_at", String, "Updated at")
	Attribute("template_did", Any, "The DID of the used template")
	Attribute("template_version", Any, "The version of the used template")
	Attribute("start_date", String, "The timestamp when the contract starts")
	Attribute("exp_date", String, "The timestamp when the contract expired")
	Attribute("exp_policy", String, "The policy what should happen if the contract is expired")
	Attribute("exp_notice_period", Int, "The notice period before contract expiration (in days)")
	Attribute("responsible", Any, "Responsible for this contract, including the creator, approvers, reviewers, and negotiators")
	Attribute("contract_data", Any, "The data for the contract")
	Attribute("origin", String, "The did of the dcs where the contract was created")

	Required("did", "contract_version", "state", "created_by", "created_at", "updated_at",
		"template_did", "template_version", "responsible", "origin",
		"contract_version", "template_did", "contract_data",
	)
})

var DCSToDCSContractReviewTaskItem = Type("DCSToDCSContractReviewTaskItem", func() {
	Attribute("id", String, "ID of the review task")
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the review task")
	Attribute("reviewer", String, "The reviewer of the contract")
	Attribute("created_at", String, "Created at")
	Attribute("created_by", String, "Identifier of who created the review task")

	Required("id", "did", "state", "reviewer", "created_at", "contract_version", "created_by")
})

var DCSToDCSContractApprovalTaskItem = Type("DCSToDCSContractApprovalTaskItem", func() {
	Attribute("id", String, "ID of the approval task")
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "The version of the contract")
	Attribute("state", String, "State of the approval task")
	Attribute("approver", String, "The approver for the contract")
	Attribute("created_at", String, "Created at")
	Attribute("created_by", String, "Identifier of who created the approval task")

	Required("id", "did", "state", "approver", "created_at", "contract_version", "created_by")
})

var DCSToDCSContractNegotiationTaskItem = Type("DCSToDCSContractNegotiationTaskItem", func() {
	Attribute("id", String, "ID of the review task")
	Attribute("did", String, "DID of the contract")
	Attribute("state", String, "State of the approval task")
	Attribute("negotiator", String, "The negotiator for the contract")
	Attribute("created_at", String, "Created at")
	Attribute("created_by", String, "Identifier of who created the negotiation task")

	Required("id", "did", "state", "negotiator", "created_at", "created_by")
})

var DCSToDCSContractNegotiationItem = Type("DCSToDCSContractNegotiationItem", func() {
	Attribute("id", String, "ID of the review task")
	Attribute("did", String, "DID of the contract")
	Attribute("contract_version", Int, "Version of the contract")
	Attribute("change_request", Any, "The change request for the contract")
	Attribute("created_at", String, "Created at")
	Attribute("created_by", String, "Identifier of who created the negotiation task")

	Required("id", "did", "contract_version", "created_at", "created_by")
})

var DCSToDCSContractNegotiationDecisionItem = Type("DCSToDCSContractNegotiationDecisionItem", func() {
	Attribute("id", String, "ID of the review task")
	Attribute("negotiation_id", String, "The id of the negotiation")
	Attribute("negotiator", String, "The negotiator who made that decision")
	Attribute("decision", String, "The decision what was made")
	Attribute("rejection_reason", String, "The reason for the rejection")

	Required("id", "negotiation_id", "negotiator")
})

var DCSToDCSContractCreateRequest = Type("DCSToDCSContractCreateRequest", func() {
	Description("Contract create request")

	Attribute("contract", DCSToDCSContractItem, "The contract")
	Attribute("review_tasks", ArrayOf(DCSToDCSContractReviewTaskItem), "The review tasks for that contract")
	Attribute("approval_tasks", ArrayOf(DCSToDCSContractApprovalTaskItem), "The approval tasks for that contract")
	Attribute("negotiation_tasks", ArrayOf(DCSToDCSContractNegotiationTaskItem), "The negotiation tasks for that contract")
	Attribute("negotiation_items", ArrayOf(DCSToDCSContractNegotiationItem), "The negotiations for that contract")
	Attribute("negotiation_decisions", ArrayOf(DCSToDCSContractNegotiationDecisionItem), "The decisions for the change requests")

	Required("contract", "review_tasks", "approval_tasks", "negotiation_tasks")
})

var DCSToDCSContractCreateResponse = Type("DCSToDCSContractCreateResponse", func() {
	Description("Result for creating a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var DCSToDCSContractUpdateRequest = Type("DCSToDCSContractUpdateRequest", func() {
	Description("Contract update request")

	Attribute("contract", DCSToDCSContractItem, "The contract")
	Attribute("review_tasks", ArrayOf(DCSToDCSContractReviewTaskItem), "The review tasks for that contract")
	Attribute("approval_tasks", ArrayOf(DCSToDCSContractApprovalTaskItem), "The approval tasks for that contract")
	Attribute("negotiation_tasks", ArrayOf(DCSToDCSContractNegotiationTaskItem), "The negotiation tasks for that contract")
	Attribute("negotiation_items", ArrayOf(DCSToDCSContractNegotiationItem), "The negotiations for that contract")
	Attribute("negotiation_decisions", ArrayOf(DCSToDCSContractNegotiationDecisionItem), "The decisions for the change requests")

	Required("contract", "review_tasks", "approval_tasks", "negotiation_tasks")
})

var DCSToDCSContractUpdateResponse = Type("DCSToDCSContractUpdateResponse", func() {
	Description("Result for creating a contract")

	Attribute("did", String, "Decentralized Identifier of the contract")

	Required("did")
})

var DCSToDCSContractStatusRequest = Type("DCSToDCSContractStatusRequest", func() {
	Description("Request the status of the contract on this peer")
})

var DCSToDCSContractStatusResponse = Type("DCSToDCSContractStatusResponse", func() {
	Description("Result for the status")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("status", Boolean, "Whether the contract exists (true) or not (false)")

	Required("did", "status")
})

var _ = Service("DcsToDcs", func() {
	Description("DCS supports direct interoperability between two or more DCS instances, enabling automated contract lifecycle operations across organizational boundaries.")

	Method("create", func() {

		Payload(DCSToDCSContractCreateRequest)
		Result(DCSToDCSContractCreateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/peer/create")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("update", func() {
		Description("Offer a policy-gated, read-only contract information endpoint between a DCS instance and a counterparty DCS")
		Meta("dcs:requirements", "DCS-IR-SI-06")

		Payload(DCSToDCSContractUpdateRequest)
		Result(DCSToDCSContractUpdateResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/peer/update")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("status", func() {
		Description("Offer a policy-gated, read-only contract information endpoint between a DCS instance and a counterparty DCS")
		Meta("dcs:requirements", "DCS-IR-SI-06")

		Payload(DCSToDCSContractStatusRequest)
		Result(DCSToDCSContractStatusResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/peer/status")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
