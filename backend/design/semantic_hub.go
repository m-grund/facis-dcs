package design

import (
	. "goa.design/goa/v3/dsl"
)

var SemanticSchemaItem = Type("SemanticSchemaItem", func() {
	Description("One stored, versioned Semantic Hub schema entry (DCS-FR-TR-03)")

	Attribute("name", String, "Schema name")
	Attribute("version", Int, "Schema version (monotonic per name+kind)")
	Attribute("kind", String, "Schema kind: context (JSON-LD), shapes (SHACL), or profile (validation profile)")
	Attribute("media_type", String, "Media type of the content (e.g. application/ld+json, text/turtle)")
	Attribute("content", String, "The schema document, verbatim")
	Attribute("active", Boolean, "Whether this is the active version documents are produced against")
	Attribute("created_by", String, "Who registered this version")
	Attribute("created_at", String, "When this version was registered")

	Required("name", "version", "kind", "media_type", "content", "active", "created_by", "created_at")
})

var ClauseCatalogProperty = Type("ClauseCatalogProperty", func() {
	Description("One typed clause property (a SHACL sh:property on a clause NodeShape), pre-digested for form generation")

	Attribute("path", String, "Property path (compact local name, e.g. \"amount\")")
	Attribute("datatype", String, "XSD datatype local name (e.g. \"integer\", \"string\"), empty if unconstrained")
	Attribute("in", ArrayOf(String), "Allowed value set (sh:in), empty if unconstrained")
	Attribute("min_count", Int, "sh:minCount, 0 if unconstrained")
	Attribute("max_count", Int, "sh:maxCount, 0 if unconstrained")
	Attribute("min_inclusive", Float64, "sh:minInclusive, present only if declared")
	Attribute("max_inclusive", Float64, "sh:maxInclusive, present only if declared")

	Required("path")
})

var ClauseCatalogType = Type("ClauseCatalogType", func() {
	Description("One typed clause (a SHACL NodeShape targeting a clause class), pre-digested for the template builder's palette")

	Attribute("type", String, "The clause's dcs:type (e.g. \"dcs:PaymentClause\")")
	Attribute("label", String, "Human-readable label (rdfs:label on the shape, falls back to the type's local name)")
	Attribute("properties", ArrayOf(ClauseCatalogProperty), "The clause's typed properties")

	Required("type", "label", "properties")
})

var ClauseCatalogResponse = Type("ClauseCatalogResponse", func() {
	Description("The active clause catalog: a pre-digested JSON form-schema plus the raw SHACL shapes it was derived from")

	Attribute("version", Int, "The clause-catalog hub version this catalog was read from")
	Attribute("clauses", ArrayOf(ClauseCatalogType), "Pre-digested clause type form-schemas")
	Attribute("shapes", String, "The raw SHACL Turtle the catalog was derived from")

	Required("version", "clauses", "shapes")
})

var SemanticSchemaRegisterResponse = Type("SemanticSchemaRegisterResponse", func() {
	Description("Result of registering a Semantic Hub schema version")

	Attribute("name", String, "Schema name")
	Attribute("version", Int, "The assigned version")
	Attribute("kind", String, "Schema kind")
	Attribute("active", Boolean, "Whether the new version was activated")

	Required("name", "version", "kind", "active")
})

// Semantic Hub Service (/semantic/...) — DCS-FR-TR-03, UC-02-08: versioned
// storage for the JSON-LD contexts, SHACL shapes, and validation profiles
// every DCS document is produced against. Reads are public (like
// /.well-known/did.json): produced artifacts carry hub-served schemaRefs
// that external verifiers must be able to resolve without a DCS login.
var _ = Service("SemanticHub", func() {
	Description("Semantic Hub APIs (/semantic/...): versioned JSON-LD context, SHACL shape, and validation-profile storage (DCS-FR-TR-03, UC-02-08)")

	Method("register", func() {
		Description("Register a new version of a schema (context/shapes/profile). Versions are immutable and monotonic per name+kind; when activate is set the new version becomes the one all newly produced documents anchor to.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		Meta("dcs:ui", "Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile")
			})
			Attribute("media_type", String, "Media type of the content")
			Attribute("content", String, "The schema document, verbatim")
			Attribute("activate", Boolean, "Make the new version active immediately")
			Required("name", "kind", "media_type", "content")
		})
		Result(SemanticSchemaRegisterResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/semantic/schema/register")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("rollback", func() {
		Description("Make a previously registered version the active one again (UC-02-08: schema versioning and rollback).")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		Meta("dcs:ui", "Template Management Dashboard")

		Security(JWTAuth, func() {
			Scope("Template Manager")
		})

		Payload(func() {
			Token("token", String, "JWT token")
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile")
			})
			Attribute("version", Int, "The version to activate")
			Required("name", "kind", "version")
		})
		Result(SemanticSchemaRegisterResponse)

		Error("bad_request", ErrorResult, "Bad request")
		Error("not_found", ErrorResult, "No such schema version")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			POST("/semantic/schema/rollback")
			Response(StatusOK)
			Response("bad_request", StatusBadRequest)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("retrieve", func() {
		Description("Retrieve a schema version (the active one when version is omitted). Public, like /.well-known/did.json — produced artifacts carry hub schemaRefs external verifiers resolve without a DCS login.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile")
			})
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name", "kind")
		})
		Result(SemanticSchemaItem)

		Error("not_found", ErrorResult, "No such schema")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/schema/retrieve")
			Param("name")
			Param("kind")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("versions", func() {
		Description("List every stored version of a schema, oldest first (UC-02-08).")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Schema name")
			Attribute("kind", String, "Schema kind", func() {
				Enum("context", "shapes", "profile")
			})
			Required("name", "kind")
		})
		Result(ArrayOf(SemanticSchemaItem))

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/schema/versions")
			Param("name")
			Param("kind")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("resolve_context", func() {
		Description("Serve a registered JSON-LD context document itself (the resolvable anchor produced documents' schemaRefs.jsonLdContext points at). Returns the parsed JSON-LD document as the response body.")
		Meta("dcs:requirements", "DCS-FR-TR-03")
		NoSecurity()

		Payload(func() {
			Attribute("name", String, "Context schema name")
			Attribute("version", Int, "Specific version; active version when omitted")
			Required("name")
		})
		Result(Any)

		Error("not_found", ErrorResult, "No such context")
		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/context/{name}")
			Param("version")
			Response(StatusOK)
			Response("not_found", StatusNotFound)
			Response("internal_error", StatusInternalServerError)
		})
	})

	Method("clauses", func() {
		Description("List the active clause catalog (DCS-FR-TR-03/TR-04, Phase 3, ADR-10): typed clause NodeShapes the template builder's palette generates a form from, pre-digested into a JSON form-schema server-side (each clause type's target class, label, and properties with their datatype/sh:in/min-max constraints) plus the raw SHACL Turtle for a future shacl-form-style client. Public, like resolve_context: a produced contract's typed clauses are validated against these same shapes (validateAgainstHubShapes), and an external verifier resolving dcs:schemaRefs needs to read them too.")
		Meta("dcs:requirements", "DCS-FR-TR-03", "DCS-FR-TR-04")
		Meta("dcs:ui", "Template Management Dashboard")
		NoSecurity()

		Result(ClauseCatalogResponse)

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/semantic/clauses")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
