package design

import (
	. "goa.design/goa/v3/dsl"
)

var _ = Service("DIDService", func() {
	Description("Returns the DID document for contracts, templates, and the service itself")

	Method("GetServiceDID", func() {
		Description("Returns the service's own DID document (domain-level, no path)")

		Result(Any)
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/.well-known/did.json")
			GET("/api/.well-known/did.json")

			Response(StatusOK, func() {
				ContentType("application/did+json")
			})
			Response("internal_error", StatusInternalServerError)
		})
	})
})
