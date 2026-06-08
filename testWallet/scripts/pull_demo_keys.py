#!/usr/bin/env python3
"""Pull demo OID4VP material from Vault into local testWallet paths."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path


def main() -> int:
    script = Path(__file__).resolve().parent / "generate_dev_keys.py"
    return subprocess.call([sys.executable, str(script), "--vault-read"])


if __name__ == "__main__":
    raise SystemExit(main())
