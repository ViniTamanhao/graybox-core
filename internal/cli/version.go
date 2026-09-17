package cli

import (
	"fmt"
	"runtime/debug"
)

var readBuildInfo = debug.ReadBuildInfo

func (a App) buildVersion() string {
	if a.Version != "" {
		return a.Version
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func (a App) runVersion(args []string) (int, error) {
	var jsonOutput, help bool
	usage := func() {
		fmt.Fprint(a.Stdout, `Usage: graybox version [options]

Print the Graybox build version.

Options:
  --json             emit structured JSON
  -h, --help         show this help
`)
	}
	fs := a.newFlagSet("version", usage)
	fs.BoolVar(&jsonOutput, "json", false, "")
	fs.BoolVar(&help, "help", false, "")
	fs.BoolVar(&help, "h", false, "")
	positional, err := parseInterspersed(fs, args, map[string]bool{"json": true, "help": true, "h": true})
	if err != nil {
		return ExitUsage, usageError{err.Error()}
	}
	if help {
		usage()
		return ExitSuccess, nil
	}
	if len(positional) != 0 {
		return ExitUsage, usageError{"version does not accept arguments"}
	}
	version := a.buildVersion()
	if jsonOutput {
		if err := writeJSON(a.Stdout, map[string]string{"version": version}); err != nil {
			return ExitInternal, err
		}
		return ExitSuccess, nil
	}
	fmt.Fprintf(a.Stdout, "graybox %s\n", version)
	return ExitSuccess, nil
}
