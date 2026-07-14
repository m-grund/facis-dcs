"""Behave environment hooks for DCS BDD tests."""

import os
import socket
import sys
from pathlib import Path
import psycopg2


SKIP_TAGS = {"skip", "skipped"}


def _install_localhost_resolver_fallback():
    """RFC 6761 reserves *.localhost for loopback, but not every resolver
    actually implements that (e.g. plain /etc/nsswitch.conf 'dns' without
    'files' or a wildcard stub). The two-instance BDD suite's did:web
    hostnames (dcs-a.localhost, dcs-b.localhost) need to resolve host-side
    without editing /etc/hosts or using sudo (explicit harness constraint) —
    so wrap socket.getaddrinfo: only for hostnames ending in '.localhost'
    that the real resolver fails to resolve, synthesize a loopback (127.0.0.1)
    result instead of raising. A no-op wherever the system resolver already
    handles it (e.g. GitHub runners via systemd-resolved), since the real
    resolver is always tried first.
    """
    real_getaddrinfo = socket.getaddrinfo

    def _getaddrinfo(host, port, *args, **kwargs):
        try:
            return real_getaddrinfo(host, port, *args, **kwargs)
        except socket.gaierror:
            if isinstance(host, str) and host.endswith(".localhost"):
                return real_getaddrinfo("127.0.0.1", port, *args, **kwargs)
            raise

    socket.getaddrinfo = _getaddrinfo


def _normalize_tag(tag):
	value = str(tag).strip().lower()
	if value.startswith("@"):
		value = value[1:]
	return value


def _has_skip_tag(tags):
	return any(_normalize_tag(tag) in SKIP_TAGS for tag in (tags or ()))


def _iter_feature_scenarios(feature):
	# Includes scenarios generated from outlines when supported by Behave.
	if hasattr(feature, "walk_scenarios"):
		yield from feature.walk_scenarios()
	else:
		yield from feature.scenarios


def before_feature(context, feature):
	if not _has_skip_tag(getattr(feature, "tags", ())):
		return
	for scenario in _iter_feature_scenarios(feature):
		scenario.skip('Skipped by feature tag "@skip"')


def _scenario_has_skip_tag(scenario):
	tags = []
	tags.extend(getattr(scenario, "effective_tags", ()) or ())
	tags.extend(getattr(scenario, "tags", ()) or ())
	feature = getattr(scenario, "feature", None)
	if feature is not None:
		tags.extend(getattr(feature, "tags", ()) or ())
	return _has_skip_tag(tags)


def cleanup_database(context):
	cursor = context.db.cursor()

	try:
		_cleanup_database(cursor)
	except Exception:
		# Leave the shared connection usable for the rest of the suite; the
		# scenario itself still fails with the original error.
		context.db.rollback()
		cursor.close()
		raise

	context.db.commit()
	cursor.close()


def _cleanup_database(cursor):
	cursor.execute("DELETE FROM access_attempts")
	cursor.execute("DELETE FROM ip_lockouts")

	cursor.execute("DELETE FROM contract_negotiation_task")
	cursor.execute("DELETE FROM contract_approval_task")
	cursor.execute("DELETE FROM contract_review_task")
	cursor.execute("DELETE FROM contract_negotiations")
	cursor.execute("TRUNCATE contract_archive_entry_events, contract_archive_entries")
	cursor.execute("DELETE FROM contract_kpis")
	cursor.execute("DELETE FROM contract_deployments")
	cursor.execute("DELETE FROM contract_signatures")
	cursor.execute("DELETE FROM signature_ceremonies")
	cursor.execute("DELETE FROM contracts")

	cursor.execute("DELETE FROM contract_templates_approval_task")
	cursor.execute("DELETE FROM contract_templates_review_task")
	cursor.execute("DELETE FROM template_provenance_credentials")
	cursor.execute("DELETE FROM contract_templates")



def before_scenario(context, scenario):
	if _scenario_has_skip_tag(scenario):
		scenario.skip('Skipped by scenario tag "@skip"')

	if "clean_db" in scenario.tags:
		cleanup_database(context)


def before_all(context):
	_install_localhost_resolver_fallback()

	steps_dir = Path(__file__).resolve().parent / "steps"
	steps_dir_str = str(steps_dir)
	if steps_dir_str not in sys.path:
		sys.path.insert(0, steps_dir_str)

	# Shared request defaults for step definitions.
	# Default to the Vite dev-server proxy (:5173), not the backend port
	# directly (:8991): Hydra has no fixed URLS_SELF_PUBLIC configured, so it
	# derives its OAuth redirect target dynamically from the Host header of
	# whichever caller reaches it first in the login chain. Requests that hit
	# the backend port directly leak that host into Hydra's redirect_to
	# response, which the backend then can't serve (404 on /oauth2/auth) —
	# the whole login flow only works end-to-end when everything consistently
	# goes through the same origin the dev stack's Hydra client is registered
	# against (localhost:5173, see deployment/helm/values.dev.yml).
	context.base_url = os.getenv("BDD_DCS_BASE_URL", "http://localhost:5173/api").rstrip("/")
	# 60s: component-wide audit reads (POST /pac/audit) walk every per-DID
	# hash chain over IPFS; mid-suite that legitimately exceeds 20s on slower
	# runners without being wrong.
	context.http_timeout_seconds = float(os.getenv("BDD_HTTP_TIMEOUT_SECONDS", "60"))
	context.aliases = {}

	try:
		context.db = psycopg2.connect(
			os.getenv("DATABASE_URL", "host=localhost port=30432 user=dcs password=dcs dbname=dcs sslmode=disable")
		)
		context.db.cursor().execute("SELECT 1")
		print("DB connection successful")
	except psycopg2.OperationalError as e:
		raise RuntimeError(f"Could not connect to database: {e}")


def after_all(context):
    context.db.close()
