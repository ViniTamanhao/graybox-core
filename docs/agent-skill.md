# Agent Skills integration

The reusable [skill](../skills/graybox/SKILL.md) helps compatible coding agents consider recorded HTTP
behavior during API refactors, serialization/authentication/middleware changes,
business-logic updates and API fixes. It supplements ordinary tests. It skips
unrelated documentation, styling and utility tasks; it reports blockers when
baseline provenance, credentials or replay safety cannot be established.

One canonical package under `skills/graybox` supplies workflow guidance; the CLI
remains the execution engine and works independently of any agent. The portable
package is included in **v0.3.1**, separately from the CLI archives. The client
table below distinguishes live evidence from structural compatibility. See the
[portability report](agent-skill-portability.md) for sources and current checks,
and [historical Codex evidence](agent-skill-evaluation.md).

| Client | Project location | Personal location | Current package evidence |
| --- | --- | --- | --- |
| Codex | `.agents/skills/graybox` | `~/.agents/skills/graybox` | Validated: fresh bounded CLI trials; see report |
| Claude Code | `.claude/skills/graybox` | `~/.claude/skills/graybox` | Structurally compatible; live blocked |
| Cursor | `.agents/skills/graybox` or `.cursor/skills/graybox` | `~/.agents/skills/graybox` or `~/.cursor/skills/graybox` | Structurally compatible; live blocked |
| Copilot CLI / VS Code agent mode | `.agents/skills/graybox` or `.github/skills/graybox` | `~/.agents/skills/graybox` or `~/.copilot/skills/graybox` (surface dependent) | Structurally compatible; live blocked |

Prefer one `.agents` copy shared by Codex, Cursor and compatible Copilot surfaces;
use Claude Code's documented `.claude` location. Avoid duplicate names per client.
These are official discovery conventions, not runtime guarantees.

## 1. Install the CLI

Use the [installation guide](installation.md) to install the released v0.3.1 CLI:

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@v0.3.1
```

Alternatively, download its [prebuilt binary](https://github.com/ViniTamanhao/graybox-core/releases/tag/v0.3.1).
Use the matching v0.3.1 source tag for the skill instructions below.

Put Go's binary directory on PATH and verify `graybox version --json`. Record the
source revision for a `dev` version. The skill has no AI service dependency;
using Codex itself requires your normal Codex installation/authentication.

## 2. Install only the skill into your application

### Published v0.3.1 package

Obtain the skill from the published v0.3.1 tag, rather than the moving `main`
branch. Run this from any directory:

```bash
umask 077
skill_source_root="$(mktemp -d /tmp/graybox-skill.XXXXXXXXXX)" || exit 1
graybox_source="$skill_source_root/source"
git clone --branch v0.3.1 --depth 1 https://github.com/ViniTamanhao/graybox-core.git "$graybox_source"
```

Then change to your application's repository (for example `cd ~/src/my-api`):

```bash
mkdir -p .agents/skills
test ! -e .agents/skills/graybox && test ! -L .agents/skills/graybox || { printf 'Review existing skill before updating\n'; exit 1; }
cp -R "$graybox_source/skills/graybox" .agents/skills/graybox
```

### Copy and discover

The commands use POSIX shell tools. Substitute `.claude/skills` for Claude Code,
`.cursor/skills` for Cursor-only use, or `.github/skills` for Copilot-only use.
Copy the entire package. Verify `SKILL.md`, three reference Markdown files and
`examples/authenticated-api.md` exist. Windows users need equivalent native copy
and existence/link checks; native Windows installation was not tested. No
installer, privileged operation or downloaded shell execution is needed.

After copying, you can remove the temporary checkout. The application needs
only this small directory, not the Graybox source tree or
its development `agents/` instructions. Teams can check in the skill package.
Launch Codex from the application repository and confirm `graybox` appears in
`/skills` (or type `$`). Restart if discovery has not refreshed. Ask an ordinary
API task, or explicitly invoke `$graybox` to review behavioral verification.
The description permits implicit selection; it does not guarantee selection on
every task. No `agents/openai.yaml` or extra Codex configuration is required.

For personal availability across projects, the documented user location is
`~/.agents/skills/graybox`; prefer one scope to avoid duplicate names. Project
skills are discovered from CWD up to the repository root. These conventions are
from [official Codex skill documentation](https://learn.chatgpt.com/docs/build-skills).
The evaluated installation path is project-local `.agents/skills/graybox`.

Update by reviewing the new package diff, then replacing the installed directory
with the reviewed version; preserve any local customizations deliberately.
Remove that installed `graybox` directory to uninstall and restart if necessary.
Neither operation changes the CLI, recordings or application credentials.

## 3. Provide application context

Keep a private recording with provenance: accepted reference revision/build,
capture time/owner, environment, relevant requests, protection configuration and
preferably a digest. Inspect complete bodies and expected statuses. A recording
captured after a change cannot verify historical preservation. When no accepted
reference exists, Codex should report that limitation instead of inventing one.

Identify the safe development process/build and disable real payment, email and
other integrations. Loopback alone is insufficient. Replay and diff execute real
requests, including writes. The skill never grants production permission.

Provision runtime credentials with your project's normal authorized mechanism.
For an Authorization header, supply its complete value in `API_AUTH`; the command
uses `--secret-header Authorization=API_AUTH`. Do not paste real values into task
prompts or commit them. Body/form/query replacements use supported environment
references in [configuration](configuration.md); Graybox does not load `.env`.
Missing credentials must remain a blocker rather than prompting authentication
removal. See [security](SECURITY.md).

## Review the result

Expect evidence of normal tests and an actual `diff --json`, with baseline
identity, independently verified target/build, IDs/counts, active policy and exit.
Exit 1 means changed behavior, which may be intended; exit 3 means comparison
failure. Empty exit 0 is no coverage. Command errors and malformed JSON are
unverified. Review affected locations against requirements, and require a rerun
after fixing regressions. Protected values and unrecorded paths remain outside
the claim. Never silently widen ignores or replace a baseline to pass.

Before accepting a verification claim, match it to an executed command and its
observed output/status. A skipped comparison has no runtime error or equivalence
result. The skill separates executed observations from not-executed checks and
requires a final evidence review; see the [retest results](agent-skill-evaluation.md#round-2-verification-and-acceptance)
for the bounded validation and remaining limits.

Reports and recordings remain potentially sensitive despite redaction. Keep them
in private temporary storage outside version control; publish only reviewed
summaries. Try the [synthetic authenticated exercise](../skills/graybox/examples/authenticated-api.md)
or follow the [full workflow](agent-verification-workflow.md).

## Other clients and troubleshooting

Claude Code offers `/graybox`; Cursor offers `/` skill search and its skill
settings. Copilot CLI documents `/skills reload`, `/skills info graybox` and
`/graybox`. VS Code requires agent mode and a version supporting skills. These
checks are documented, not fresh runtime evidence. Automatic activation depends
on the description, task, configuration and model; it is not guaranteed.

If discovery fails, check scope, exact filename, duplicate names and client policy,
then reload/restart. Check CLI PATH separately. Missing trusted baseline,
credentials or target authorization blocks replay: continue ordinary development
and report the limitation. Never weaken authentication or recapture changed
behavior to pass. Remote agents cannot assume access to local recordings,
credentials or loopback services; prepare an authorized disposable runtime there.

For an authenticated handler refactor, inspect the accepted baseline and target,
provision runtime credentials, change the code, run ordinary tests, diff selected
IDs, investigate differences and reverify corrections. Use the packaged synthetic
exercise for a concrete example. Sensitive evidence stays private outside Git.
