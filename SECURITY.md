# Security

## Recordings are sensitive

Graybox observes application traffic. A `.graybox` recording may contain authentication tokens, cookies, passwords, personal data, API keys, private identifiers, internal URLs, proprietary payloads, and other secrets.

V0 automatically replaces values in these headers before writing them:

- `Authorization`
- `Proxy-Authorization`
- `Cookie`
- `Set-Cookie`

Header matching is case-insensitive and the stored marker is `<REDACTED>`. Replay omits redacted headers rather than sending the marker.

This policy is not exhaustive. In particular, Graybox V0 does not inspect or redact request/response bodies, URL paths or queries, or other potentially sensitive headers. Treat every raw `.graybox` file as potentially sensitive. Review a recording before sharing it, use restrictive filesystem permissions, and avoid committing recordings to source control.

Graybox is local-first and never uploads recordings automatically.

## Reporting a vulnerability

Please report suspected vulnerabilities privately through GitHub's security-advisory feature for this repository. Include the affected version, impact, and a minimal reproduction when possible. Do not include real credentials or private production recordings.
