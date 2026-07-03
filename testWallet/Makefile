VENV_DIR ?= .venv
PYTHON := $(VENV_DIR)/bin/python
PIP := $(VENV_DIR)/bin/pip
VENV_STAMP := $(VENV_DIR)/.install-stamp

.PHONY: install ensure-statuslist test

$(PYTHON):
	python3 -m venv $(VENV_DIR)

$(VENV_STAMP): $(PYTHON) requirements.txt
	$(PIP) install -r requirements.txt
	touch $@

install: $(VENV_STAMP)

ensure-statuslist: $(VENV_STAMP)
	$(PYTHON) scripts/ensure_statuslist_for_dev.py

test: $(VENV_STAMP)
	$(PYTHON) -m unittest discover -s dcs_wallet -p "test_*.py"