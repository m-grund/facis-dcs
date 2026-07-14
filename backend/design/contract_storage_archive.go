package design

import (
	. "goa.design/goa/v3/dsl"
)

var ArchiveRetrieveRequest = Type("ArchiveRetrieveRequest", func() {
	Description("Archive retrieve request")

	Token("token", String, "JWT token")
})
var ArchiveRetrieveResponse = Type("ArchiveRetrieveResponse", func() {
	Description("Result for retrieving the archive")

	Attribute("contracts", ArrayOf(ContractItem), "A list of contracts")
})

var ArchiveSearchRequest = Type("ArchiveSearchRequest", func() {
	Description("Archive search request")

	Token("token", String, "JWT token")

	Attribute("did", String, "Decentralized Identifier of the contract")
	Attribute("contract_version", Int, "The version number of the contract")
	Attribute("state", String, "The state of the contract")
	Attribute("name", String, "The name of the contract")
	Attribute("description", String, "A description for that contract")
	Attribute("contract_data", String, "Search value for full text search in contract data")
	Attribute("tag", String, "Return only archive entries carrying this annotation tag (DCS-FR-CSA-11)")
})

var ArchiveAnnotationResponse = Type("ArchiveAnnotationResponse", func() {
	Description("The archive entry annotation after an annotate call (DCS-FR-CSA-11)")

	Attribute("did", String, "Decentralized Identifier of the annotated contract")
	Attribute("summary", String, "The stored summary (caller-provided, or system-generated from the contract metadata when none was supplied)")
	Attribute("tags", ArrayOf(String), "The stored tag set")

	Required("did", "summary")
})

// Contract Storage & Archive Service  (/archive/...)
var _ = Service("ContractStorageArchive", func() {
	Description("Contract Storage & Archive APIs (/archive/...)")

	Method("retrieve", func() {
		Description("retrieve archived items.")
		Meta("dcs:requirements", "DCS-IR-CSA-01", "DCS-IR-CSA-05")
		Meta("dcs:ui", "Archive Manager Dashboard", "Archive Access")
		Meta("dcs:csa:components", "Signed Contract Archive")

		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
		})

		Payload(ArchiveRetrieveRequest)
		Result(ArchiveRetrieveResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/retrieve")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("search", func() {
		Description("search archived records. search records by criteria.")
		Meta("dcs:requirements", "DCS-IR-CSA-01", "DCS-IR-CSA-05")
		Meta("dcs:ui", "Archive Manager Dashboard", "Archive Access")
		Meta("dcs:csa:components", "Signed Contract Archive")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
			Scope("Contract Observer")
		})
		Payload(ArchiveSearchRequest)
		Result(ArrayOfRequired(ContractItem))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/search")
			Param("did")
			Param("contract_version")
			Param("state")
			Param("name")
			Param("description")
			Param("contract_data")
			Param("tag")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("store", func() {
		Description("store new contract or evidence.")
		Meta("dcs:requirements", "DCS-IR-CSA-02", "DCS-IR-CSA-06")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "Signed Contract Archive")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			POST("/archive/store")
			Response(StatusOK)
		})
		Result(String)
	})

	Method("delete", func() {
		Description("Permanently delete an archived contract entry (DCS-FR-CSA-17). This is a soft delete: the archive entry is marked deleted_at/deleted_by/deletion_reason rather than physically removed, so evidence remains discoverable for compliance/dispute resolution, and requires a justification that is logged with the deletion's audit event.")
		Meta("dcs:requirements", "DCS-IR-CSA-03", "DCS-IR-CSA-06", "DCS-FR-CSA-17")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "Signed Contract Archive", "Automated Alerts")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract to delete")
			Attribute("justification", String, "Justification for the deletion (DCS-FR-CSA-17); logged with the deletion audit event")
			Required("did", "justification")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			DELETE("/archive/delete")
			Param("did")
			Param("justification")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
		Result(Int)
	})

	Method("annotate", func() {
		Description("Annotate an archived contract with a summary and tags (DCS-FR-CSA-11). The summary may be supplied by the caller or, when omitted, is generated from the archived contract's metadata; tags replace the entry's tag set when provided. Only the annotation is mutable — the archive entry's snapshot and evidence stay immutable.")
		Meta("dcs:requirements", "DCS-FR-CSA-11")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "Signed Contract Archive")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("did", String, "Decentralized Identifier of the archived contract to annotate")
			Attribute("summary", String, "Manual summary; when omitted (and none is stored yet) a summary is generated from the contract metadata")
			Attribute("tags", ArrayOf(String), "Tags for thematic categorization and discovery; replaces the entry's tag set")
			Required("did")
		})
		Result(ArchiveAnnotationResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/archive/annotate")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("audit", func() {
		Description("Retrieve the archive audit log: actor, timestamp, operation, and contract DID for every recorded archive-affecting event (store/retrieve/search/delete) — DCS-IR-CSA-04, UC-07-03.")
		Meta("dcs:requirements", "DCS-IR-CSA-04")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/archive/audit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
		Result(ArrayOfRequired(ContractAuditResponse))
	})

})
