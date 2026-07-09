"""Load all step definitions from subpackages."""

# This file ensures that step definitions in subpackages are loaded by behave
from core import *
from template_management import *
from frontend import *
from pdf_generation import *
from peer_trust import *
from odrl import *
from pki_consolidation import *
from real_signing_vertical import *
