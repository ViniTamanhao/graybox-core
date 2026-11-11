# Agent behavioral verification workflow

Use Graybox when a task changes observable HTTP behavior and relevant, trustworthy
recorded traffic can be executed safely. Ordinary tests still run. Documentation
edits and unrelated utilities usually need no replay. This guide uses the existing
[authenticated order API](../examples/server/README.md); for parsing and safety
requirements see the [contract](agent-verification-contract.md).

## A — A trusted baseline already exists

1. Understand the requested behavior and what is intended to remain unchanged.
   Identify who captured the baseline, its reference application state, and the
   requests relevant to the task. Preserve its provenance and, where practical,
   a digest. Do not silently regenerate it or change comparison policy to pass.
2. Inspect before execution:

   ```bash
   ./graybox version --json
   ./graybox ls order.graybox --json
   ./graybox show order.graybox 1 --json
   ```

   Check request methods/payloads, response status, redaction and body completion.
   An empty recording or an unrelated exchange is not verification evidence.
   Inspection does not contact the target. A list filter does not carry over into
   diff; use `--id 1` explicitly when only that exchange is appropriate.
3. Confirm the target process, port, build and state. Assess side effects even for
   GET requests. Use a disposable local application with real integrations disabled;
   do not treat loopback or an explicit target flag as permission to affect production.
   Obtain authorized runtime credentials. Missing credentials are a blocker, not
   an invitation to guess or remove authentication requirements.
4. Modify the application following its development workflow, including meaningful
   failing tests for bugs/new behavior. Run ordinary tests. Start the intended build
   and reset disposable application state as needed before replay.
5. Compare once; `diff` already replays, so a preceding `replay` is unnecessary and
   can duplicate side effects. For this demo the known synthetic credential is:

   ```bash
   export API_AUTH='Bearer demo-token'
   umask 077
   report_dir="$(mktemp -d /tmp/graybox-reports.XXXXXXXXXX)" || exit 1
   # Existing local demo baseline, one relevant request:
   if ./graybox diff order.graybox --id 1 \
     --target http://127.0.0.1:8080 \
     --secret-header Authorization=API_AUTH --json \
     > "$report_dir/comparison.json" \
     2> "$report_dir/comparison.stderr"; then
     result=0
   else
     result=$?
   fi
   printf 'Graybox exit: %s\n' "$result"
   printf 'Reports stored in: %s\n' "$report_dir"
   ```

   The conditional preserves nonzero outcomes even in shells using `set -e`.
   `mktemp` creates a fresh user-only directory under `/tmp`, outside the checkout
   even if `TMPDIR` points into it; `umask 077` also restricts new report files.
   Reports may contain sensitive data despite redaction: never commit them, and
   inspect them before quoting values in PR descriptions or final reports. Keep
   them for inspection, then remove the temporary directory when no longer needed.
   Private storage does not replace safe recording, authentication or target selection.
   For real credentials, use the project's authorized secret provisioning mechanism,
   never literals in task reports. Configuration can supply body/query credentials;
   use `--config FILE` when the working directory is not the intended config location.
6. Interpret the exit **and** JSON together. For diff, 0 with relevant nonempty
   equivalent results verifies the selected sanitized behavior under the policy.
   Exit 1 reports differences; decide from task requirements whether they are
   expected changes or unexpected regressions. Exit 3 means at least one failed
   comparison; preserve any other useful results but report the aggregate as
   inconclusive. Exit 2/4/5 is a command failure, with text diagnostics rather than
   a JSON error object. Do not claim success from empty or invalid output.
7. Investigate unexpected differences by exchange ID and response location.
   Complete 401/500 responses are comparable behavior; check authentication and
   application state before treating a difference as a code regression. Fix the
   application and rerun ordinary tests/comparison where appropriate. Do not
   overwrite the baseline or silently widen ignores to obtain equivalence.
8. Report reviewed evidence: version/commit, baseline provenance/digest, target
   environment/build, selection, active policy, exit and counts, exact changed
   locations, expected/unexpected classification and unresolved limits. Distinguish
   unexecuted verification from a comparison that ran and failed. Confirm the
   baseline remains unchanged. Review all quoted values for sensitive information.

## B — No baseline exists

Capture before modifying the application only if the current reference state is
known or explicitly accepted, the application can run, the requests are relevant,
and traffic/credentials are authorized. If these conditions are unavailable,
report “Graybox comparison not executed: no trustworthy baseline” and run the
other appropriate tests. A recording captured after the change describes the new
behavior; it cannot prove preservation of the previous behavior. Do not reconstruct
historical evidence from expectations just to produce a passing diff.

For the included demo, build at the reference state and start the normal API in
terminal 1 (the known demo has no real payment/notification integration):

```bash
go build -o graybox ./cmd/graybox
go run ./examples/server
```

Start capture in terminal 2. Choose a fresh output filename; existing files are
refused. Configure any additional redaction before capture:

```bash
./graybox record --target http://127.0.0.1:8080 --output order.graybox
```

In terminal 3, capture the accepted reference request:

```bash
export API_AUTH='Bearer demo-token'
curl --fail-with-body http://127.0.0.1:9000/orders/42 -H "Authorization: $API_AUTH"
```

Check `total_cents` is 4500. Stop the recorder gracefully with Ctrl+C in terminal 2
and check its final exit/diagnostics; startup alone proves no successful capture.
Inspect with `ls` and `show` as above: the Authorization header should be
`<REDACTED>`, and both bodies should be complete and not truncated. Preserve this
baseline and its reference state. Now follow scenario A.

For an intentional demo change, stop the API and restart terminal 1 with:

```bash
go run ./examples/server --regressed
```

Run scenario A's diff command. The following JSON is derived from an executed
source-build demo; the target port is normalized to the documented default.
Durations are observations from that run and will vary:

```json
{
  "recording": "order.graybox",
  "target": "http://127.0.0.1:8080",
  "body_capture_limit": 10485760,
  "ignored": ["response.headers.date", "response.headers.content-length"],
  "summary": {"total": 1, "equivalent": 0, "changed": 1, "failed": 0},
  "results": [{
    "exchange_id": 1,
    "method": "GET",
    "path": "/orders/42",
    "outcome": "changed",
    "baseline_duration_ms": 1.192388,
    "current_duration_ms": 0.781284,
    "differences": [{
      "kind": "value_changed",
      "location": {"component": "response.body", "path": "/total_cents"},
      "before": {"present": true, "value": 4500},
      "after": {"present": true, "value": 5000}
    }]
  }]
}
```

Exit is 1. Restart the API without `--regressed` and repeat: exit 0,
`summary` is `{"total":1,"equivalent":1,"changed":0,"failed":0}` and the result
has `"outcome":"equivalent","differences":[]`. The baseline remains unchanged.
For this task, the changed total is an intentional demo regression; in a real
feature task only the requirements establish whether the change is acceptable.

Example final report: “Compared exchange 1, GET /orders/42, against the disposable
local build using the preserved pre-change baseline. Exit 1: 0 equivalent,
1 changed, 0 failed. response.body#/total_cents changed from 4500 to 5000, matching
the deliberate demo regression. Restored build: exit 0, 1 equivalent. Default
Date/Content-Length ignores applied; credentials were redacted. This verifies
only the selected response under that policy, not other endpoints or side effects.”
Attach version/build and baseline provenance to that report in actual use. Never
include secret values. This executed CLI example is not an experiment validating
an autonomous coding-agent integration. See the separate
[skill evaluations](agent-skill-evaluation.md) and
[portability trials](agent-skill-portability.md) for that evidence.
