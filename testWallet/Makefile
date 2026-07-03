PYTHON ?= python3

.PHONY: install ensure-statuslist test

install:
	$(PYTHON) -m pip install -r requirements.txt

ensure-statuslist:
	$(PYTHON) scripts/ensure_statuslist_for_dev.py

test:
	$(PYTHON) -m unittest discover -s dcs_wallet -p "test_*.py"