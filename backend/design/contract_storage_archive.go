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
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			GET("/archive/search")
			Response(StatusOK)
		})
		Result(ArrayOf(Any))
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

	Method("terminate", func() {
		Description("terminate contract/archive entry.")
		Meta("dcs:requirements", "DCS-IR-CSA-03", "DCS-IR-CSA-06")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "Automated Alerts")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			POST("/archive/terminate")
			Response(StatusOK)
		})
		Result(Int)
	})

	Method("delete", func() {
		Description("permanently delete entry.")
		Meta("dcs:requirements", "DCS-IR-CSA-03", "DCS-IR-CSA-06")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "Signed Contract Archive", "Automated Alerts")
		Security(JWTAuth, func() {
			Scope("Archive Manager")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			DELETE("/archive/delete")
			Response(StatusOK)
		})
		Result(Int)
	})

	Method("audit", func() {
		Description("retrieve audit logs.")
		Meta("dcs:requirements", "DCS-IR-CSA-04")
		Meta("dcs:ui", "Archive Manager Dashboard")
		Meta("dcs:csa:components", "")
		Security(JWTAuth, func() {
			Scope("Auditor")
			Scope("Compliance Officer")
			Scope("System Administrator")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			GET("/archive/audit")
			Response(StatusOK)
		})
		Result(ArrayOf(String))
	})

})
