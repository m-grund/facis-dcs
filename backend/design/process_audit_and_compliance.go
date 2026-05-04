package design

import (
	. "goa.design/goa/v3/dsl"
)

var PACAuditRequest = Type("PACAuditRequest", func() {
	Description("Process audit request")

	Token("token", String, "JWT token")

	Attribute("scope", String, "Scope that should be audited")

	Required("scope")
})

var PACResourceAuditTrailEntry = Type("PACResourceAuditTrailEntry", func() {
	Description("Resource audit trails entry")

	Attribute("id", Int64, "Identifier for the outbox event")
	Attribute("component", String, "Name of the component")
	Attribute("event_type", String, "Type of the event")
	Attribute("event_data", Any, "Data of the event")
	Attribute("did", String, "Decentralized Identifier of the resource")
	Attribute("created_at", String, "The creation date of the event")
	Attribute("res_log_pred_cid", String, "Resource audit trail predecessor on the IPFS chain")
	Attribute("global_log_pred_cid", String, "Global audit trail predecessor on the IPFS chain")

	Required("id", "component", "event_type", "event_data", "created_at")
})

var PACAuditResponse = Type("PACAuditResponse", func() {
	Description("Resource audit trail")

	Attribute("did", String, "Decentralized Identifier of the resource")
	Attribute("component", String, "Name of the component")
	Attribute("created_at", String, "Creation date of the audit response")
	Attribute("audit_trail", ArrayOfRequired(PACResourceAuditTrailEntry), "Resource audit trails entries")

	Required("did", "component", "created_at", "audit_trail")
})

// Process Audit & Compliance Management Service  (/processauditandcompliance/...)
var _ = Service("ProcessAuditAndCompliance", func() {
	Description("Process Audit & Compliance Management APIs (/processauditandcompliance/...)")

	Method("audit", func() {
		Description("trigger an audit on selected scope.")
		Meta("dcs:requirements", "DCS-IR-PACM-01")
		Meta("dcs:ui", "Auditing Tool")
		Meta("dcs:pacm:components", "")

		Security(JWTAuth, func() {
			Scope("Auditor")
		})

		Payload(PACAuditRequest)
		Result(ArrayOfRequired(PACAuditResponse))

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/processauditandcompliance/audit")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("audit_report", func() {
		Description("generate and retrieve audit reports.")
		Meta("dcs:requirements", "DCS-IR-PACM-02")
		Meta("dcs:ui", "Auditing Tool")
		Meta("dcs:pacm:components", "")
		Security(JWTAuth, func() {
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			GET("/processauditandcompliance/report")
			Response(StatusOK)
		})
		Result(Any)
	})

	Method("monitor", func() {
		Description("continuous monitoring and event retrieval.")
		Meta("dcs:requirements", "DCS-IR-PACM-03")
		Meta("dcs:ui", "Non-Compliance Investigation")
		Meta("dcs:pacm:components", "")
		Security(JWTAuth, func() {
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			GET("/processauditandcompliance/monitor")
			Response(StatusOK)
		})
		Result(Any)
	})

	Method("incident_report", func() {
		Description("submit non-compliance findings as case records.")
		Meta("dcs:requirements", "DCS-IR-PACM-04")
		Meta("dcs:ui", "Non-Compliance Investigation")
		Meta("dcs:pacm:components", "")
		Security(JWTAuth, func() {
			Scope("Compliance Officer")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			POST("/processauditandcompliance/report")
			Response(StatusOK)
		})
		Result(Any)
	})
})
