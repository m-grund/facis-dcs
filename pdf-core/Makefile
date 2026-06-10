.PHONY: all build test bdd test-all gen ontology clean

SCHEMA    := ontology/dcs-pdf-core.yaml
ONTOL_DIR := ontology/generated

# Default: codegen + build
all: gen ontology build

# ── Goa codegen ──────────────────────────────────────────────────────
# Regenerates gen/ on every build.
gen:
	go run goa.design/goa/v3/cmd/goa gen example.com/m/V2/design
	go tool swaggerpatch gen/http/openapi3.yaml

# ── Ontology codegen ─────────────────────────────────────────────────
$(ONTOL_DIR)/dcs-pdf-core-context.jsonld: $(SCHEMA) | $(ONTOL_DIR)
	gen-jsonld-context $< > $@

$(ONTOL_DIR)/dcs-pdf-core.owl.ttl: $(SCHEMA) | $(ONTOL_DIR)
	gen-owl $< > $@

$(ONTOL_DIR)/dcs-pdf-core.owl.jsonld: $(ONTOL_DIR)/dcs-pdf-core.owl.ttl | $(ONTOL_DIR)
	python3 -c "from rdflib import Graph; g=Graph(); g.parse('$<'); g.serialize('$@', format='json-ld')"

$(ONTOL_DIR)/dcs-pdf-core.shacl.ttl: $(SCHEMA) | $(ONTOL_DIR)
	gen-shacl $< > $@

$(ONTOL_DIR):
	mkdir -p $(ONTOL_DIR)

ontology: $(ONTOL_DIR)/dcs-pdf-core-context.jsonld \
          $(ONTOL_DIR)/dcs-pdf-core.owl.ttl \
	      $(ONTOL_DIR)/dcs-pdf-core.owl.jsonld \
	      $(ONTOL_DIR)/dcs-pdf-core.shacl.ttl

# ── Tests ────────────────────────────────────────────────────────────
# Unit + integration tests (fast, no server required)
test: gen
	go test ./...

# BDD tests (starts a real server, requires Python + behave)
bdd: gen
	behave features/

# Both suites
test-all: test bdd

# ── Go build ─────────────────────────────────────────────────────────
build: gen
	go build ./...

# ── Housekeeping ─────────────────────────────────────────────────────
clean:
	rm -rf $(ONTOL_DIR) gen
