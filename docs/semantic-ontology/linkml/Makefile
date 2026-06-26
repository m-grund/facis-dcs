PYTHON      ?= python3
VENV_PATH   ?= .venv
OUTPUT_PATH ?= output
SCHEMA      ?= linkml.yaml

VENV_PYTHON := $(VENV_PATH)/bin/python
VENV_PIP    := $(VENV_PATH)/bin/pip
GEN_OWL            := gen-owl
GEN_SHACL          := gen-shacl
GEN_JSONLD_CONTEXT := gen-jsonld-context
PYSHACL            := $(VENV_PATH)/bin/pyshacl

SHAPES_EXTRA  ?= shapes-content.ttl
SHAPES_MERGED := $(OUTPUT_PATH)/$(SCHEMA).shacl.merged.ttl

ORDERED_LIST_PROPS := blocks content children

VALID_TESTS   := $(wildcard tests/valid/*.jsonld)
INVALID_TESTS := $(wildcard tests/invalid/*.jsonld)

GEN_OWL_FLAGS := --skip-vacuous-min-zero-cardinality-axioms \
                  --skip-vacuous-local-range-axioms \
                  --consolidate-cardinality-axioms

.PHONY: all setup_venv validate clean

all: $(OUTPUT_PATH)/$(SCHEMA).owl.ttl $(SHAPES_MERGED) $(OUTPUT_PATH)/$(SCHEMA).context.jsonld

setup_venv: $(VENV_PATH)/bin/activate

$(VENV_PATH)/bin/activate:
	$(PYTHON) -m venv $(VENV_PATH)
	$(VENV_PIP) install --upgrade pip
	$(VENV_PIP) install linkml pyshacl

$(OUTPUT_PATH):
	mkdir -p $(OUTPUT_PATH)

$(OUTPUT_PATH)/$(SCHEMA).owl.ttl: setup_venv $(OUTPUT_PATH)
	$(GEN_OWL) $(GEN_OWL_FLAGS) $(SCHEMA) > $(OUTPUT_PATH)/$(SCHEMA).owl.ttl

$(OUTPUT_PATH)/$(SCHEMA).shacl.ttl: setup_venv $(OUTPUT_PATH)
	$(GEN_SHACL) --non-closed $(SCHEMA) > $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl

$(OUTPUT_PATH)/$(SCHEMA).context.jsonld: setup_venv $(OUTPUT_PATH)
	$(GEN_JSONLD_CONTEXT) $(SCHEMA) > $(OUTPUT_PATH)/$(SCHEMA).context.jsonld
	$(VENV_PYTHON) -c "\
import json; \
path = '$(OUTPUT_PATH)/$(SCHEMA).context.jsonld'; \
ctx = json.load(open(path)); \
[ctx['@context'][p].update({'@container': '@list'}) for p in '$(ORDERED_LIST_PROPS)'.split() if p in ctx['@context']]; \
json.dump(ctx, open(path, 'w'), indent=3)"

$(SHAPES_MERGED): $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl $(SHAPES_EXTRA)
	cat $^ > $@

validate: setup_venv $(SHAPES_MERGED)
	@failed=0; \
	for f in $(VALID_TESTS); do \
	    out=$$($(PYSHACL) -s $(SHAPES_MERGED) -df json-ld $$f 2>&1); \
	    if echo "$$out" | grep -q "Conforms: True"; then \
	        echo "PASS  $$f"; \
	    else \
	        echo "FAIL  $$f  (expected valid)"; \
	        echo "$$out"; failed=1; \
	    fi; \
	done; \
	for f in $(INVALID_TESTS); do \
	    out=$$($(PYSHACL) -s $(SHAPES_MERGED) -df json-ld $$f 2>&1); \
	    if ! echo "$$out" | grep -q "Conforms: True"; then \
	        echo "PASS  $$f  (correctly rejected)"; \
	        echo "$$out" | grep -E "Constraint Violation|Message:|Result Path:" | sed 's/^/      /'; \
	    else \
	        echo "FAIL  $$f  (expected rejection)"; failed=1; \
	    fi; \
	done; \
	exit $$failed

clean:
	rm -rf $(OUTPUT_PATH)
