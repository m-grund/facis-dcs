package design

import (
	. "goa.design/goa/v3/dsl"
)

var C2PASignRequest = Type("C2PASignRequest", func() {
	Description("Request to sign COSE Sig_structure bytes with the HSM C2PA key")

	Token("token", String, "JWT token")

	Attribute("sig_structure", String, "Base64-encoded COSE Sig_structure bytes to be signed")

	Required("sig_structure")
})

var C2PASignResponse = Type("C2PASignResponse", func() {
	Description("An ES256 signature over the submitted COSE Sig_structure bytes")

	Attribute("signature", String, "Base64-encoded 64-byte raw r||s ES256 signature")

	Required("signature")
})

// InternalSigning exposes authenticated, non-public signing primitives that
// keep private-key material inside the backend's PKCS#11 token. pdf-core (a
// separate process holding no key material) builds the COSE Sig_structure and
// calls c2paSign to obtain the ES256 signature it embeds into a C2PA manifest
// (DCS-IR-HI-01, Workstream A2.3). Unlike C2PAService (public manifest
// retrieval), these methods require a JWT.
var _ = Service("InternalSigning", func() {
	Description("Authenticated backend-internal signing primitives backed by the PKCS#11 token (DCS-IR-HI-01).")

	Method("c2paSign", func() {
		Description("Signs COSE Sig_structure bytes with the HSM dcs-c2pa key and returns the raw 64-byte r||s ES256 signature.")
		Meta("dcs:requirements", "DCS-OR-C2PA-001")

		// The call is made by pdf-core on behalf of whatever already-authenticated
		// action triggered a PDF (re-)render — most commonly a contract or
		// template export, not a dedicated "sign" action — so the accepted
		// scopes are the union of export_contract_pdf's, export_contract_bundle's,
		// export_template_pdf's, and export_template_bundle's scopes
		// (pdf_generation.go), rather than being restricted to a signing-specific
		// role.
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Contract Reviewer")
			Scope("Contract Creator")
			Scope("Contract Approver")
			Scope("Contract Observer")
			Scope("Contract Signer")
			Scope("Sys. Contract Signer")
			Scope("Template Manager")
			Scope("Template Reviewer")
			Scope("Template Creator")
			Scope("Template Approver")
		})

		Payload(C2PASignRequest)
		Result(C2PASignResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/internal/c2pa/sign")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
