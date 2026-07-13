"""Real-signing-vertical (Workstream B: PAdES + EUDIPLO ceremony + PID
binding, docs/anforderung.md B1-B4/B-acceptance) BDD step definitions.

NOTE (recurring bug in earlier workstreams): this __init__.py MUST actually
import the step module below — an empty __init__.py makes behave silently
fail to discover every step in this package (steps become "undefined" at
runtime with no import error). See steps/peer_trust/__init__.py and
steps/pki_consolidation/__init__.py for the same, correct pattern.
"""

from . import dcs_real_signing_vertical_steps  # noqa: F401
from . import dcs_real_signing_vertical_tamper_steps  # noqa: F401
from . import dcs_real_signing_vertical_orce_steps  # noqa: F401
