#!/usr/bin/env python3
"""Dev preflight: create tenant list 1 via NATS when the DB is empty.

Uses dev NodePorts from values.dev.yml (HTTP 30821, NATS 30422).
Run from dev-stack.sh after Helm deploy; not used for BDD.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.status_list import (
    DEFAULT_NATS_URL,
    DEFAULT_SERVICE_BASE,
    ensure_status_list_initialized,
)


def main() -> int:
    service_base = os.getenv("STATUSLIST_SERVICE_URL", DEFAULT_SERVICE_BASE)
    nats_url = os.getenv("NATS_URL", DEFAULT_NATS_URL)
    ensure_status_list_initialized(service_base=service_base, nats_url=nats_url)
    print(f"statuslist ready for dev: {service_base.rstrip('/')}/v1/tenants/default/status/1")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
