PYTHON      ?= python3
VENV_PATH   ?= .venv
OUTPUT_PATH ?= output
SCHEMA      ?= linkml.yaml

VENV_PYTHON := $(VENV_PATH)/bin/python
VENV_PIP    := $(VENV_PATH)/bin/pip
GEN_OWL     := $(VENV_PATH)/bin/gen-owl
GEN_SHACL   := $(VENV_PATH)/bin/gen-shacl
GEN_JSONLD_CONTEXT   := $(VENV_PATH)/bin/gen-jsonld-context
PYSHACL     := $(VENV_PATH)/bin/pyshacl

EXAMPLE     ?= example-jsonld.jsonld

# Deny Deprecation-Hints
GEN_OWL_FLAGS := --skip-vacuous-min-zero-cardinality-axioms \
                  --skip-vacuous-local-range-axioms \
                  --consolidate-cardinality-axioms


.PHONY: setup_venv generate clean

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

generate: $(OUTPUT_PATH)/$(SCHEMA).owl.ttl $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl $(OUTPUT_PATH)/$(SCHEMA).context.jsonld

validate: $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl $(OUTPUT_PATH)/$(SCHEMA).owl.ttl
	$(PYSHACL) \
	  -s $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl \
	  -e $(OUTPUT_PATH)/$(SCHEMA).owl.ttl \
	  -d $(EXAMPLE) \
	  --data-file-format json-ld \
	  -i rdfs \
	  -f human

clean:
	rm -rf $(OUTPUT_PATH)