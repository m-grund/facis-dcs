package main

import (
	"context"
	"fmt"
	"net/http"

	_ "embed"

	dcspdfcore "example.com/m/V2/gen/dcspdfcore"
	dcspdfcoresvr "example.com/m/V2/gen/http/dcspdfcore/server"
	goahttp "goa.design/goa/v3/http"
)

//go:embed ontology/generated/dcs-pdf-core-context.jsonld
var ontologyContext []byte

//go:embed ontology/generated/dcs-pdf-core.owl.jsonld
var ontologyOWL []byte

//go:embed ontology/generated/dcs-pdf-core.shacl.ttl
var ontologySHACL []byte

//go:embed gen/http/openapi3.json
var openAPI3JSON []byte

// binaryResponseEncoder wraps goahttp.ResponseEncoder but writes raw bytes for
// binary content types instead of JSON-encoding them as base64 strings.
// It sets the Content-Type header here (before the generated code calls
// WriteHeader) so the correct media type is sent with the response.
func binaryResponseEncoder(ctx context.Context, w http.ResponseWriter) goahttp.Encoder {
	ct, _ := ctx.Value(goahttp.ContentTypeKey).(string)
	switch ct {
	case "application/pdf", "application/ld+json":
		w.Header().Set("Content-Type", ct)
		return &rawEncoder{w}
	}
	return goahttp.ResponseEncoder(ctx, w)
}

type rawEncoder struct{ w http.ResponseWriter }

func (e *rawEncoder) Encode(v any) error {
	b, ok := v.([]byte)
	if !ok {
		return fmt.Errorf("rawEncoder: expected []byte, got %T", v)
	}
	_, err := e.w.Write(b)
	return err
}

func newServer() http.Handler {
	svc := &dcspdfcoreService{}
	endpoints := dcspdfcore.NewEndpoints(svc)

	mux := goahttp.NewMuxer()
	svr := dcspdfcoresvr.New(endpoints, mux, goahttp.RequestDecoder, binaryResponseEncoder, nil, nil)
	dcspdfcoresvr.Mount(mux, svr)

	// Meta-routes not part of the goa design.
	mux.Handle("GET", "/swagger.json", handleSwagger)
	mux.Handle("GET", "/ui/", handleUI)
	mux.Handle("GET", "/index.html", handleUI)
	mux.Handle("GET", "/ontology/dcs-pdf-core.shacl", handleSHACL)
	mux.Handle("GET", "/", handleRoot)

	return mux
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
}

func handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, swaggerHTML)
}

func handleSwagger(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(openAPI3JSON)
}

func handleSHACL(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
	_, _ = w.Write(ontologySHACL)
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
	<title>DCS-PDF-CORE Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    :root { color-scheme: light; }
    body { margin: 0; background: linear-gradient(135deg, #f4efe4, #dce8e4); }
    #swagger-ui { min-height: 100vh; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/swagger.json',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis]
    });
  </script>
</body>
</html>
`
