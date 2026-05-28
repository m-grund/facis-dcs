package design

import (
	. "goa.design/goa/v3/dsl"
)

var PDFVerifyResult = Type("PDFVerifyResult", func() {
	Description("Result of MR/HR hash consistency verification (DCS-FR-CWE-04, DCS-FR-CWE-05)")

	Attribute("match", Boolean, "True when the stored PDF was generated from the embedded JSON-LD without alteration")
	Attribute("jsonld_hash", String, "SHA-256 hex of the extracted JSON-LD attachment")
	Attribute("base_pdf_hash", String, "SHA-256 hex of the re-generated base PDF from the same JSON-LD")
	Attribute("stored_base_pdf_hash", String, "SHA-256 hex of the stored PDF base layer (before any C2PA incremental updates)")

	Required("match", "jsonld_hash", "base_pdf_hash", "stored_base_pdf_hash")
})

// PDFGeneration Service  (/pdf/...)
var _ = Service("PDFGeneration", func() {
	Error("not_found", ErrorResult, "Contract or template not found")
	Error("internal_error", ErrorResult, "Internal server error")

	HTTP(func() {
		Response("not_found", StatusNotFound)
		Response("internal_error", StatusInternalServerError)
	})

	Description("PDF export and MR/HR hash verification for contracts and templates (DCS-FR-CWE-04, DCS-FR-CWE-05, DCS-OR-C2PA-001)")

	// export_contract_pdf — GET /pdf/export/contract/{did}
	Method("export_contract_pdf", func() {
		Description("Export a contract as a PDF/A-3 document with embedded JSON-LD and accumulated C2PA lifecycle assertions.")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-SM-27", "DCS-OR-C2PA-001")
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Contract Reviewer")
			Scope("Contract Creator")
			Scope("Contract Approver")
			Scope("Contract Observer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract")
			Required("did")
		})
		HTTP(func() {
			GET("/pdf/export/contract/{did}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK)
		})
	})

	// export_template_pdf — GET /pdf/export/template/{did}
	Method("export_template_pdf", func() {
		Description("Export a contract template as a PDF/A-3 document with embedded JSON-LD.")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-SM-27")
		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Template Reviewer")
			Scope("Template Creator")
			Scope("Template Approver")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract template")
			Required("did")
		})
		HTTP(func() {
			GET("/pdf/export/template/{did}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK)
		})
	})

	// verify_contract_pdf — GET /pdf/verify/contract/{did}
	Method("verify_contract_pdf", func() {
		Description("Verify MR/HR hash consistency for a contract: re-generates the base PDF from the embedded JSON-LD and compares SHA-256 hashes. (DCS-FR-CWE-04, DCS-FR-CWE-05)")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-CWE-05", "DCS-FR-CSA-06")
		Security(JWTAuth, func() {
			Scope("Contract Manager")
			Scope("Contract Reviewer")
			Scope("Contract Approver")
			Scope("Contract Observer")
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract")
			Required("did")
		})
		Result(PDFVerifyResult)
		HTTP(func() {
			GET("/pdf/verify/contract/{did}")
			Response(StatusOK)
		})
	})

	// verify_template_pdf — GET /pdf/verify/template/{did}
	Method("verify_template_pdf", func() {
		Description("Verify MR/HR hash consistency for a contract template.")
		Meta("dcs:requirements", "DCS-FR-CWE-04", "DCS-FR-CWE-05")
		Security(JWTAuth, func() {
			Scope("Template Manager")
			Scope("Template Reviewer")
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "DID of the contract template")
			Required("did")
		})
		Result(PDFVerifyResult)
		HTTP(func() {
			GET("/pdf/verify/template/{did}")
			Response(StatusOK)
		})
	})
})
