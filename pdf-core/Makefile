.PHONY: all build gen test bdd test-all ontology clean

SCHEMA    := ontology/dcs-pdf-core.yaml
ONTOL_DIR := ontology/generated

# Default: ontology + build
all: ontology build

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

gen: ontology

# ── Tests ────────────────────────────────────────────────────────────
# Unit + integration tests (fast, no server required)
test:
	go test ./...

# BDD tests (starts a real server, requires Python + behave)
bdd:
	behave features/

# Both suites
test-all: test bdd

# ── Go build ─────────────────────────────────────────────────────────
build:
	go build -o pdf-core .

# ── Housekeeping ─────────────────────────────────────────────────────
clean:
	rm -rf $(ONTOL_DIR) pdf-core
