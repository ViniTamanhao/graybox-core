# Troubleshooting without hiding uncertainty

| Observation | Safe next step |
| --- | --- |
| Executable missing | Check project tooling/PATH and official installation instructions; report unexecuted comparison until a trusted binary exists |
| No recording | Capture accepted pre-change state if available and authorized; otherwise report no trustworthy baseline |
| Invalid/unsupported file or missing ID (2) | Inspect provenance and `ls`; obtain valid evidence, do not edit SQLite or invent an exchange |
| Missing mapped credential (4) | Use authorized provisioning; do not guess, echo values or disable authentication |
| Complete 401/403 | Inspect auth expectations and runtime mapping; this is comparable rejection, not automatically `failed` or a code regression |
| Target unavailable / failed result (3) | Confirm intended process/build/port independently; retry only after resolving cause and reassessing effects |
| Incomplete/truncated request or response | Inspect `show` body flags/proxy error; recapture only from accepted reference state when safe; report unavailable evidence, do not hide it with ignores |
| Unsupported protocol/reconstruction | Report limitation; use other tests, do not invent flags or automatic repair |
| Unexpected differences (1) | Locate exchange/field, compare requirements and state, fix actual regression then rerun unchanged policy/baseline |
| Empty success (0) | No coverage; find relevant recorded exchanges or report unverified |
| Missing/malformed JSON or output failure (5) | Preserve stderr/status privately; inspect cause, assume requests might have run; never infer equivalence |

No machine-readable error categories exist. Do not classify errors by fragile
prose matching. Command failures differ from per-exchange failures. See
[verification](verification.md) for aggregate counts and exit precedence.
