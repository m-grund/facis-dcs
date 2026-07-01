.PHONY: all build gen test bdd test-all ontology clean start-pdf docker

VENV_PATH := .venv
VENV_PYTHON        := $(VENV_PATH)/bin/python3
BEHAVE              := $(VENV_PATH)/bin/behave

# Default: ontology + build
all: docs build

docs: 
	make -C ../docs/semantic-ontology/linkml/ clean all
	cp -r ../docs docs

# Start pdf-core server (for development)
start-pdf: all
	air

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

# ── Docker ───────────────────────────────────────────────────────────
docker: docs
	docker build -t dcspdfcore .

# ── Housekeeping ─────────────────────────────────────────────────────
clean:
	rm -rf $(ONTOL_DIR) pdf-core docs/
