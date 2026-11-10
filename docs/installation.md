# Installation and release verification

## Release status

As verified on 2026-11-10, the latest published release is
[v0.2.3](https://github.com/ViniTamanhao/graybox-core/releases/tag/v0.2.3).
**v0.3.0 is the upcoming release candidate, not a published release.** It includes
the merged stabilization fixes and agent milestones 1–4. The earlier proposed
v0.2.4 was not published; those changes are included in the v0.3.0 scope.
See the [candidate notes](releases/v0.3.0.md) and [readiness evidence](releases/v0.3.0-readiness.md).

`@latest` and release downloads select published versions, not current `main`.
Before publication, use a reviewed candidate checkout and the local packaging
commands below. Do not expect `go install ...@v0.3.0` or tag downloads to work yet.

## Source

The current main branch requires **Go 1.26.10 or newer**, as declared in go.mod.
Go may download the required toolchain automatically; set `GOTOOLCHAIN=local` if
you want an older local toolchain to fail instead. CI reads the version directly
from go.mod. No C compiler is needed for normal builds (SQLite is pure Go).

```bash
git clone https://github.com/ViniTamanhao/graybox-core.git
cd graybox-core
go build -o graybox ./cmd/graybox
./graybox version
```

A checkout build reports `graybox dev`. On Windows use `-o graybox.exe` and
`.\graybox.exe`. For an installed released version:

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@v0.2.3
graybox version
```

Use `@latest` to select the latest published module instead of a pinned version.
A published module can have an older minimum Go requirement than current main.
Module installs report the module version (for example `v0.2.3`); release builds
embed the tag with `-ldflags="-X main.version=v0.2.3"`. Explicit build injection
wins over module build info; unversioned checkout builds fall back to `dev`.

Go installs into `go env GOBIN`, or `$(go env GOPATH)/bin` when GOBIN is empty.
Add that directory to PATH, or invoke the binary by its full path. Shell PATH
changes may require opening a new terminal. On PowerShell use `$env:PATH` and
`Get-Command graybox` to inspect resolution; on Unix use `command -v graybox`.

## Prebuilt binaries

Download from [GitHub Releases](https://github.com/ViniTamanhao/graybox-core/releases).
Select the OS and architecture of the machine running Graybox:
`linux`, `darwin` (macOS), or `windows`; `amd64` (x86-64) or `arm64`.
The archive naming convention is `graybox_VERSION_OS_ARCH.tar.gz` on Unix and
`.zip` on Windows, with VERSION excluding the leading `v`. Each contains one
root-level executable, `graybox` or `graybox.exe`.

Example for the published v0.2.3 Linux AMD64 artifact, in a new empty directory:

```bash
curl -fLO https://github.com/ViniTamanhao/graybox-core/releases/download/v0.2.3/graybox_0.2.3_linux_amd64.tar.gz
curl -fLO https://github.com/ViniTamanhao/graybox-core/releases/download/v0.2.3/SHA256SUMS
grep '  graybox_0.2.3_linux_amd64.tar.gz$' SHA256SUMS | sha256sum --check -
tar -xzf graybox_0.2.3_linux_amd64.tar.gz
chmod +x graybox
./graybox version
./graybox help diff
```

Place the verified binary in a directory on PATH (for example `~/.local/bin` on
Linux). On macOS use `shasum -a 256 ARCHIVE` and compare with the matching
SHA256SUMS line before extracting. On Windows use `Get-FileHash ARCHIVE -Algorithm
SHA256`, compare with SHA256SUMS, then `Expand-Archive ARCHIVE DESTINATION` and
run `DESTINATION\graybox.exe version`. No chmod is needed on Windows. Binaries
are not currently signed/notarized; checksum verification detects corruption,
not compromise of the source of both archive and manifest.

v0.2.3 predates main's Go 1.26.10 security update. The v0.3 preparation does not
create a release; use a current source build if that update is required.

## Maintainer packaging checks (no publishing)

From the repository root, with Go, Bash, tar and Python 3 installed:

```bash
bash scripts/package-release.sh v0.0.0-smoke /tmp/graybox-release-check
python3 scripts/verify-release.py /tmp/graybox-release-check v0.0.0-smoke
```

Choose a fresh output directory for each run. This builds all six targets with
CGO disabled and generates SHA256SUMS. The verifier checks the exact archive
set, checksums, root executable name, nonempty payload and Unix executable mode.
It extracts only the native executable to a temporary directory and tests startup,
text/JSON version, root help and every command's help. It never executes a foreign
architecture. To test one native target (as CI does):

```bash
bash scripts/package-release.sh v0.0.0-smoke /tmp/graybox-native-check "$(go env GOOS)/$(go env GOARCH)"
python3 scripts/verify-release.py --native-only /tmp/graybox-native-check v0.0.0-smoke
```

Normal CI runs native packaging checks on Linux, macOS and Windows. The tag
workflow retains the existing `vMAJOR.MINOR.PATCH[-PRERELEASE]` interface and main
ancestry check. It builds all six archives on Linux, checks them, then tests the
same archives on macOS and Windows before publication. Hosted runner native
architectures determine execution coverage (currently Linux AMD64, macOS ARM64,
Windows AMD64); other artifacts receive structure/checksum and cross-build checks,
not runtime tests. All smoke jobs must pass before release creation. Never push
a tag merely to test this workflow.

## Matching CLI and skill versions

After v0.3.0 is published, install the matching CLI:

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@v0.3.0
graybox version --json
```

For prebuilt downloads, substitute `v0.3.0` and `0.3.0` in the verified archive
example above. Expect `{"version":"v0.3.0"}`. The skill is distributed in the
matching repository tag, separately from the six CLI archives. Follow the
[external-project installation](agent-skill.md#2-install-only-the-skill-into-your-application).
No separate skill archive or plugin installation is needed for this release.

To verify the precise candidate locally without creating a tag:

```bash
umask 077
release_root="$(mktemp -d /tmp/graybox-release.XXXXXXXXXX)" || exit 1
bash scripts/package-release.sh v0.3.0 "$release_root/dist"
python3 scripts/verify-release.py "$release_root/dist" v0.3.0
# Linux AMD64 example; select your native archive on other hosts.
mkdir "$release_root/native"
tar -xzf "$release_root/dist/graybox_0.3.0_linux_amd64.tar.gz" -C "$release_root/native"
python3 evaluations/agent-skill/fixture.py check --binary "$release_root/native/graybox"
```

The final fixture is POSIX-only and builds a disposable example API with Go, but
copies and executes the supplied CLI without rebuilding it. All API commands run
outside the checkout, with synthetic credentials and scoped process cleanup.
It checks redaction, equivalent/change/restored comparisons and baseline integrity.
The ordinary archive verifier works on Linux, macOS and Windows.
