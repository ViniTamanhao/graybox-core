# Installation and release verification

## Release status

**v0.3.1 was published November 11, 2026.** See the
[GitHub release](https://github.com/ViniTamanhao/graybox-core/releases/tag/v0.3.1)
and [release notes](releases/v0.3.1.md). It includes the README refresh, portable
Agent Skills guidance and checks, and the Go 1.26.11 toolchain security update.
CLI behavior, module dependencies and schema-1 recordings are unchanged.

The `@latest` module version and GitHub Releases reflect published versions,
which may differ from the development `main` branch. For historical pre-release
verification details, see the [readiness audit](releases/v0.3.1-readiness.md).

## Source

The module minimum is **Go 1.26.11**, as declared in go.mod. Use a patched
toolchain for source builds and `go install`: at least **Go 1.26.11** on the 1.26
line or **Go 1.27.4** on the 1.27 line. The November 11, 2026
[Go advisory GO-2026-6859](https://vuln.go.dev/ID/GO-2026-6859.json) affects older
standard-library versions. The module minimum does not select the newest patched
toolchain automatically. Prebuilt artifacts retain the standard library used
when they were built.
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
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@v0.3.1
graybox version
```

Use `@latest` to select the latest published module instead of a pinned version.
A published module can have an older minimum Go requirement than current main.
Module installs report their module version (for example `v0.3.1`); release builds
embed the tag with `-ldflags="-X main.version=v0.3.1"`. Explicit build injection
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

Example for the published v0.3.1 Linux AMD64 artifact, in a new empty directory:

```bash
curl -fLO https://github.com/ViniTamanhao/graybox-core/releases/download/v0.3.1/graybox_0.3.1_linux_amd64.tar.gz
curl -fLO https://github.com/ViniTamanhao/graybox-core/releases/download/v0.3.1/SHA256SUMS
grep '  graybox_0.3.1_linux_amd64.tar.gz$' SHA256SUMS | sha256sum --check -
tar -xzf graybox_0.3.1_linux_amd64.tar.gz
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

v0.3.1 release binaries were built with Go 1.26.11. This does not rebuild or
remediate older release binaries. Prebuilt CLI binaries require no local Go installation.

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

Install the matching v0.3.1 CLI:

```bash
go install github.com/ViniTamanhao/graybox-core/cmd/graybox@v0.3.1
graybox version --json
```

For prebuilt downloads, select the v0.3.1 archive for your platform. Expect `{"version":"v0.3.1"}`. The skill is distributed in the
matching repository tag, separately from the six CLI archives. Follow the
[external-project installation](agent-skill.md#2-install-only-the-skill-into-your-application).
No separate skill archive or plugin installation is needed for this release.

For maintainers: check local packaging from a v0.3.1 source checkout without
publishing or replacing release artifacts:

```bash
umask 077
release_root="$(mktemp -d /tmp/graybox-release.XXXXXXXXXX)" || exit 1
bash scripts/package-release.sh v0.3.1 "$release_root/dist"
python3 scripts/verify-release.py "$release_root/dist" v0.3.1
# Linux AMD64 example; select your native archive on other hosts.
mkdir "$release_root/native"
tar -xzf "$release_root/dist/graybox_0.3.1_linux_amd64.tar.gz" -C "$release_root/native"
python3 evaluations/agent-skill/fixture.py check --binary "$release_root/native/graybox"
```

This fixture is POSIX-only and builds a disposable example API with Go, but
copies and executes the supplied CLI without rebuilding it. All API commands run
outside the checkout, with synthetic credentials and scoped process cleanup.
It checks redaction, equivalent/change/restored comparisons and baseline integrity.
The ordinary archive verifier works on Linux, macOS and Windows.
