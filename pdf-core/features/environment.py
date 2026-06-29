import os
import subprocess
import time
import urllib.error
import urllib.request


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def _load_dev_env(env):
    path = os.path.join(ROOT, ".dev.env")
    if not os.path.exists(path):
        return
    with open(path, "r", encoding="utf-8") as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            env[key.strip()] = value.strip()

def before_all(context):
    env = os.environ.copy()
    _load_dev_env(env)
    env.setdefault("GO111MODULE", "on")

    # Keep BDD runtime stable and deterministic on the documented loopback port.
    subprocess.run(["fuser", "-k", "8080/tcp"], check=False, capture_output=True)
    server_addr = "127.0.0.1:8080"
    server_url = f"http://{server_addr}"
    env["DCS_PDF_CORE_ADDR"] = server_addr
    # Force ontology base URL to match the server address so dcsCoreIRI in the
    # compiler aligns with the 127.0.0.1 IRIs used in BDD test payloads.
    env["DCS_PDF_CORE_ONTOLOGY_BASE_URL"] = server_url
    context.server = subprocess.Popen(
        ["go", "run", "."],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        env=env,
    )
    deadline = time.time() + 20
    last_error = None
    while time.time() < deadline:
        if context.server.poll() is not None:
            break
        try:
            with urllib.request.urlopen(f"{server_url}/swagger.json", timeout=1) as response:
                if response.status == 200:
                    context.server_url = server_url
                    return
        except Exception as exc:
            last_error = exc
            time.sleep(0.25)
    output = ""
    if context.server.stdout is not None:
        try:
            output = context.server.stdout.read()
        except Exception:
            output = ""
    raise RuntimeError(f"server failed to start: {last_error}\n{output}")


def after_all(context):
    server = getattr(context, "server", None)
    if server is None:
        return
    server.terminate()
    try:
        server.wait(timeout=5)
    except subprocess.TimeoutExpired:
        server.kill()
