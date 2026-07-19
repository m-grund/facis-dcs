package design

import (
	. "goa.design/goa/v3/dsl"
)

var DCSToDCSContractPdfRequest = Type("DCSToDCSContractPdfRequest", func() {
	Description("A contract PDF shipped to the counterparty (ADR-13). The PDF is the wire format: it carries the machine-readable JSON-LD, the C2PA provenance chain, and any signatures. A bare PDF is a proposal (offer or negotiation counter); a PDF accompanied by a JAdES is a signature (acceptance).")

	Attribute("secret_value", String, "Secret value")
	Attribute("secret_hash", Bytes, "Secret hash")

	Attribute("from_peer_did", String, "The did of the peer shipping the PDF")
	Attribute("contract_iri", String, "IRI of the contract the PDF represents")
	Attribute("pdf", Bytes, "The contract PDF")
	Attribute("jades_signature", String, "The sender's JAdES over the contract, present only when this ship is a signature (acceptance); empty for a proposal")

	Required("from_peer_did", "contract_iri", "pdf", "secret_value", "secret_hash")
})

var DCSToDCSContractPdfResponse = Type("DCSToDCSContractPdfResponse", func() {
	Description("Result for receiving a contract PDF")

	Attribute("from_peer_did", String, "Decentralized Identifier of the receiving peer")

	Required("from_peer_did")
})

var DCSToDCSSyncProvenanceResponse = Type("DCSToDCSSyncProvenanceResponse", func() {
	Description("The stored JAdES provenance artifact for a contract received from a peer (DCS-FR-SM-02)")

	Attribute("did", String, "IRI of the received contract")
	Attribute("contract_version", Int, "Contract version the signature covers")
	Attribute("from_peer_did", String, "The peer that signed the shipped contract")
	Attribute("jades_signature", String, "The verified JAdES baseline-B compact JWS as received")
	Attribute("received_at", String, "When the signed ship was accepted")

	Required("did", "contract_version", "from_peer_did", "jades_signature", "received_at")
})

var _ = Service("DcsToDcs", func() {
	Description("DCS-to-DCS federation: two DCS instances exchange the contract PDF per lifecycle step and the JAdES after signing (ADR-13). Each instance runs its own workflow/RBAC; no contract state or task ledger crosses the boundary.")

	Method("post_pdf", func() {
		Description("Receive a contract PDF shipped by the counterparty (ADR-13). The receiver verifies the sender via a did:web challenge-response signature (secret_value signed with the sender's private key, verified against its did:web document and eIDAS certificate chain) — not JWT, since there is no shared end-user identity across DCS instances run by different operators — then asks pdf-core to extract the embedded JSON-LD and upserts its own local copy of the contract. A bare PDF is a proposal (the local copy moves to negotiation); a PDF with a JAdES is the counterparty's signature.")
		Meta("dcs:cwe:components", "DCS-to-DCS Synchronization")

		Payload(DCSToDCSContractPdfRequest)
		Result(DCSToDCSContractPdfResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/peer/contracts/pdf")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("get_provenance", func() {
		Description("Return the stored JAdES provenance artifact for a contract this instance received from a peer (DCS-FR-SM-02): the sender's baseline-B compact JWS over the contract, verified at receipt and persisted for independent re-verification. JWT-secured — read by local users inspecting a received contract's cross-instance provenance.")
		Meta("dcs:cwe:components", "DCS-to-DCS Synchronization")

		Security(JWTAuth, func() {
			Scope("Contract Creator")
			Scope("Sys. Contract Creator")
			Scope("Contract Reviewer")
			Scope("Sys. Contract Reviewer")
			Scope("Contract Approver")
			Scope("Sys. Contract Approver")
			Scope("Contract Manager")
			Scope("Sys. Contract Manager")
			Scope("Contract Observer")
			Scope("Auditor")
			Scope("Compliance Officer")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "IRI of the received contract")
			Required("did")
		})
		Result(DCSToDCSSyncProvenanceResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No sync provenance stored for this contract")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/peer/contracts/provenance")
			Param("did")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
