#!/usr/bin/env python3
"""BDD preflight: tenant list 1 is reachable at STATUSLIST_SERVICE_URL.

List creation is done by the Helm hook (bdd-list-init-job.yaml).
BDD credentials are issued at login time by auth_service (not credentials/*.jwt).
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

SCRIPT_ROOT = Path(__file__).resolve().parent
PROJECT_ROOT = SCRIPT_ROOT.parents[2]
sys.path.insert(0, str(PROJECT_ROOT / "testWallet"))

from dcs_wallet.status_list import fetch_status_list_payload, status_list_uri


def main() -> int:
    service_base = os.getenv("STATUSLIST_SERVICE_URL", "").strip()
    if not service_base:
        print("STATUSLIST_SERVICE_URL is required for BDD statuslist preflight", file=sys.stderr)
        return 1

    uri = status_list_uri(service_base)
    fetch_status_list_payload(uri)
    print(f"statuslist ready for BDD: GET {uri}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
