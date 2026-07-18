#!/usr/bin/env python3
"""Dev preflight: create the status-list tenants used locally via NATS when the
DB is empty.

Uses dev NodePorts from values.dev.yml (HTTP 30821, NATS 30422). Seeds both the
default tenant and the tenant the BDD harness issues its credentials against
(BDD_CREDENTIAL_TENANT) — otherwise a local BDD run's OID4VP verification 401s
on a status-list check for an unseeded tenant.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

WALLET_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(WALLET_ROOT))

from dcs_wallet.status_list import (
    BDD_CREDENTIAL_TENANT,
    DEFAULT_NATS_URL,
    DEFAULT_SERVICE_BASE,
    DEFAULT_TENANT,
    ensure_status_list_initialized,
)


def main() -> int:
    service_base = os.getenv("STATUSLIST_SERVICE_URL", DEFAULT_SERVICE_BASE)
    nats_url = os.getenv("NATS_URL", DEFAULT_NATS_URL)
    for tenant in dict.fromkeys([DEFAULT_TENANT, BDD_CREDENTIAL_TENANT]):
        ensure_status_list_initialized(service_base=service_base, nats_url=nats_url, tenant_id=tenant)
        print(f"statuslist ready for dev: {service_base.rstrip('/')}/v1/tenants/{tenant}/status/1")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
