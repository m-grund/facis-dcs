package design

import . "goa.design/goa/v3/dsl"

var _ = API("dcspdfcore", func() {
	Title("DCS-PDF-CORE semantic ledger engine")
	Description("Deterministic PDF/A-3a compiler and semantic ledger engine")
	Version("0.1.0")
	Server("dcspdfcore", func() {
		Host("localhost", func() {
			URI("http://localhost:8080")
		})
	})
})

var _ = Service("dcspdfcore", func() {
	Description("DCS-PDF-CORE endpoints")

	// Shared errors
	Error("bad_request", ErrorResult, "Malformed or invalid input")
	Error("unsupported_media_type", ErrorResult, "Content-Type not accepted")
	Error("conflict", ErrorResult, "Document unchanged – nothing to amend")
	Error("unprocessable_entity", ErrorResult, "Payload cannot be compiled")

	// POST /download – compile JSON-LD → PDF
	Method("download", func() {
		Description("Compile deterministic PDF/A-3a bytes from a JSON-LD payload")
		Payload(func() {
			Attribute("content_type", String, "Request Content-Type (application/ld+json or application/json)")
			Required("content_type")
		})
		Result(Bytes)
		Error("bad_request")
		Error("unsupported_media_type")
		HTTP(func() {
			POST("/download")
			SkipRequestBodyEncodeDecode()
			Header("content_type:Content-Type")
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
			Response("bad_request", StatusBadRequest)
			Response("unsupported_media_type", StatusUnsupportedMediaType)
		})
	})

	// POST /verify – verify a compiled PDF and append a witness block
	Method("verify", func() {
		Description("Verify that a PDF was compiled from its embedded payload, then append a C2PA witness")
		Payload(func() {
			Attribute("content_type", String, "Request Content-Type (application/pdf)")
			Required("content_type")
		})
		Result(Bytes)
		Error("bad_request")
		Error("unsupported_media_type")
		Error("conflict")
		Error("unprocessable_entity")
		HTTP(func() {
			POST("/verify")
			SkipRequestBodyEncodeDecode()
			Header("content_type:Content-Type")
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
			Response("bad_request", StatusBadRequest)
			Response("unsupported_media_type", StatusUnsupportedMediaType)
			Response("conflict", StatusConflict)
			Response("unprocessable_entity", StatusUnprocessableEntity)
		})
	})

	// POST /update – amend a PDF with a new JSON-LD payload (multipart/form-data)
	Method("update", func() {
		Description("Amend an existing PDF with a new JSON-LD payload via multipart/form-data (fields: pdf, payload)")
		Payload(func() {
			Attribute("content_type", String, "Request Content-Type (multipart/form-data)")
			Required("content_type")
		})
		Result(Bytes)
		Error("bad_request")
		Error("unsupported_media_type")
		Error("conflict")
		HTTP(func() {
			POST("/update")
			SkipRequestBodyEncodeDecode()
			Header("content_type:Content-Type")
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
			Response("bad_request", StatusBadRequest)
			Response("unsupported_media_type", StatusUnsupportedMediaType)
			Response("conflict", StatusConflict)
		})
	})

	// POST /claim – bind an external JSON-LD claim to a PDF that lacks embedded metadata
	Method("claim", func() {
		Description("Verify that a supplied JSON-LD payload produces the same page content as the submitted PDF (which need not contain embedded metadata). Returns the canonical compiled PDF — with the JSON-LD embedded and a C2PA verification witness — as evidence of the match.")
		Payload(func() {
			Attribute("content_type", String, "Request Content-Type (multipart/form-data with boundary; fields: pdf, payload)")
			Required("content_type")
		})
		Result(Bytes)
		Error("bad_request")
		Error("unsupported_media_type")
		Error("conflict")
		HTTP(func() {
			POST("/claim")
			SkipRequestBodyEncodeDecode()
			Header("content_type:Content-Type")
			Response(StatusOK, func() {
				ContentType("application/pdf")
			})
			Response("bad_request", StatusBadRequest)
			Response("unsupported_media_type", StatusUnsupportedMediaType)
			Response("conflict", StatusConflict)
		})
	})

	// GET /ontology/dcs-pdf-core – JSON-LD context
	Method("ontology_context", func() {
		Description("Serve the DCS-PDF-CORE JSON-LD context document")
		Result(Bytes)
		HTTP(func() {
			GET("/ontology/dcs-pdf-core")
			Response(StatusOK, func() {
				ContentType("application/ld+json")
			})
		})
	})

	// GET /ontology/dcs-pdf-core.owl – OWL definition
	Method("ontology_owl", func() {
		Description("Serve the DCS-PDF-CORE OWL ontology definition as JSON-LD")
		Result(Bytes)
		HTTP(func() {
			GET("/ontology/dcs-pdf-core.owl")
			Response(StatusOK, func() {
				ContentType("application/ld+json")
			})
		})
	})
})
