# Security

## Recordings are sensitive

Graybox observes application traffic. A `.graybox` recording may contain authentication tokens, cookies, passwords, personal data, API keys, private identifiers, internal URLs, proprietary payloads, and other secrets.

V0 automatically replaces values in these headers before writing them:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`

Header matching is case-insensitive and the stored marker is `<REDACTED>`. Replay omits redacted headers rather than sending the marker.

This policy is not exhaustive. In particular, Graybox V0 does not inspect or redact request/response bodies, URL paths or queries, other potentially sensitive headers, or recorded proxy-error text. Treat every raw `.graybox` file as potentially sensitive. Review a recording before sharing it, use restrictive filesystem permissions, and avoid committing recordings to source control.

Body capture is bounded (10 MiB per request or response by default), but that bound is a resource limit, not a privacy control. Each body stores its captured byte prefix plus `original_size`, `captured_size`, and `truncated`; secrets can occur within the retained prefix. A proxy transport failure is stored as a Graybox-generated 502 with a non-empty `proxy_error`, while an upstream 502 has no proxy error.

Graybox is local-first and never uploads recordings automatically.

The recorder binds to `127.0.0.1:9000` by default. A wildcard address such as `:9000` or `0.0.0.0:9000` can expose the proxy to the local network; use one only when that exposure is intended and protected by appropriate host controls.

Replay can send recorded payloads to a server. Graybox automatically reuses a saved target only for `localhost`, `*.localhost`, and loopback IP addresses. Remote replay requires an explicit `--target` or `--unsafe-original-target`. Review the destination and recording before allowing it. Redirects are reported without being followed, and truncated request bodies are refused rather than sent partially.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub's security-advisory feature for this repository. Include the affected version, impact, and a minimal reproduction when possible. Do not include real credentials or private production recordings.
