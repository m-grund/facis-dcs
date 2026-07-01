.PHONY: all build gen test bdd test-all ontology clean

VENV_PATH := .venv
VENV_PYTHON        := $(VENV_PATH)/bin/python3
BEHAVE              := $(VENV_PATH)/bin/behave

# Default: ontology + build
all: docs build

docs: 
	cp -r ../docs docs

# ── Tests ────────────────────────────────────────────────────────────
# Unit + integration tests (fast, no server required)
test:
	go test ./...

# BDD tests (starts a real server, requires Python + behave)
bdd:
	$(BEHAVE) features/

# Both suites
test-all: test bdd

# ── Go build ─────────────────────────────────────────────────────────
build:
	go build -o pdf-core .

# ── Housekeeping ─────────────────────────────────────────────────────
clean:
	rm -rf $(ONTOL_DIR) pdf-core
