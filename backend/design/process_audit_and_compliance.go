package design

import (
	. "goa.design/goa/v3/dsl"
)

// Process Audit & Compliance Management Service  (/pac/...)
var _ = Service("ProcessAuditAndCompliance", func() {
	Description("Process Audit & Compliance Management APIs (/pac/...)")

	Method("audit", func() {
		Description("trigger an audit on selected scope.")
		Meta("dcs:requirements", "DCS-IR-PACM-01")
		Meta("dcs:ui", "Auditing Tool")
		Meta("dcs:pacm:components", "")
		Security(JWTAuth, func() {
			Scope("Auditor")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		HTTP(func() {
			POST("/pac/audit")
			Response(StatusOK)
		})
		Result(String)
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
			GET("/pac/report")
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
			GET("/pac/monitor")
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
			POST("/pac/report")
			Response(StatusOK)
		})
		Result(Any)
	})
})
