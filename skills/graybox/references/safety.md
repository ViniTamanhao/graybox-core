# Before sending traffic

Both diff and replay send real requests. Review payloads as well as methods:
even GET may trigger effects. A recording is evidence, not authorization.
Use a disposable application/database with real payments, email, webhooks and
other integrations disabled. Establish process, port, build and state independently
from the URL or report. Loopback alone does not isolate external integrations.

Do not autonomously replay production or sensitive remote traffic. Explicit
`--target` and `--unsafe-original-target` bypass destination restrictions, not
permission requirements. Prefer a verified disposable replacement target. If
safe execution cannot be established, stop replay, report the blocker, and continue
safe development work. Request authorization only for genuinely unresolved scope.
Do not probe a suspicious destination to establish whether it is safe.

Graybox automatically reuses only saved localhost/loopback targets. Redirects
are not followed. Target paths are prepended to recorded paths. There is no
rollback, state reset, idempotency-key generation, token refresh or stateful
response-to-request correlation. Inspect selection; repeat execution only when
its effects are acceptable. Track the PID or tool session of each process you
start and stop only that process; do not use broad name-based cleanup such as
`pkill -f` that can stop other applications. Unsupported protocols/reconstruction
are blockers.

## Credentials and sensitive evidence

Use the project's authorized runtime credential mechanism. Do not guess values,
print environment contents, commit tokens, remove authentication or replay
redaction markers. Header injection reads the complete header value; configuration
can map existing JSON/form/query fields from environment references. Graybox does
not load `.env` files. Missing required mapped credentials fail preflight; omitted
credentials may instead reach the app and produce a comparable HTTP rejection.

Mandatory redaction covers Authorization, Proxy-Authorization, Cookie and
Set-Cookie headers. Configure other sensitive fields **before capture**. Runtime
replacement does not retroactively redact captured fields. Paths, unconfigured
values, personal data and transformed secrets may remain; output scrubbing is
not universal secret detection. Treat recordings, diagnostics and reports as
sensitive. Use a fresh private temporary directory outside the checkout, created
with `umask 077` and `mktemp -d`. Do not use predictable shared `/tmp` filenames
or reuse a directory created by another process. Never commit recordings or raw
transcripts, and review any shared summary. Confirm the directory is owned by
you with owner-only access (0700 on POSIX) before retaining sensitive output;
new report files should be owner-only (0600). Keep the captured exit beside the
stdout/stderr files. Retain evidence for review, then remove only the directory
you created. Gracefully stop the recorded PID/tool session and verify it ended;
never infer process ownership from its executable name alone.

Preserve baseline bytes and provenance. Never overwrite or reconstruct historical
evidence to pass, silently expand ignores, or ignore unavailable body evidence
just to obtain success. A newly accepted baseline is a separate deliberate decision;
it cannot retroactively prove behavior preservation. Equivalence applies only to
selected sanitized responses under the declared policy.
