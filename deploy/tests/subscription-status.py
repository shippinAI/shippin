"""Exercise the actual image status script without provider accounts or credentials."""
import json
import os
from pathlib import Path
import subprocess
import tempfile


script = Path(__file__).resolve().parents[1] / "image/ai/shippin-ai-status"
with tempfile.TemporaryDirectory(prefix="subscription-status-") as directory:
    root = Path(directory)
    probes = root / "probes"
    report = root / "status.json"
    probes.write_text("alpha sh true\nbravo sh false\ncharlie sh exit 2\ndelta sh -\nmissing no-such-cli-3849 true\n")
    env = dict(os.environ, HOME=str(root), SHIPPIN_AI_PROBES=str(probes),
               SHIPPIN_AI_STATUS=str(report), SHIPPIN_AI_PROBE_TIMEOUT="2")
    run = subprocess.run(["bash", str(script)], env=env, capture_output=True, text=True)
    assert run.returncode == 0, run.stderr
    assert run.stdout == "", "probe output must not escape"
    status = json.loads(report.read_text())
    assert status["version"] == 2
    got = {p["provider"]: (p["installed"], p["signed_in"], p["auth_known"]) for p in status["providers"]}
    assert got == {"alpha": (True, True, True), "bravo": (True, False, True),
                   "charlie": (True, False, False), "delta": (True, False, False),
                   "missing": (False, False, False)}, got
    print("PASS: positive, signed-out, failed, unsupported and absent CLI probes; no credentials used")
