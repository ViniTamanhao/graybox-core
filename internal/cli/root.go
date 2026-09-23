// Package cli implements Graybox's human and machine-readable command surface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

const (
	ExitSuccess = 0

	// ExitReplayFailed retains the established replay contract for a
	// multi-request replay where one or more requests failed.
	ExitReplayFailed = 1

	// ExitBehaviorChanged means diff completed successfully and observed one or
	// more behavioral differences.
	//
	// It intentionally shares numeric code 1 with the existing replay
	// multi-request failure contract. Exit-code interpretation is
	// command-specific.
	ExitBehaviorChanged = 1

	ExitInvalidRecording = 2

	ExitNetwork = 3
	// ExitComparisonFailed means one or more diff comparisons could not be
	// completed. It shares code 3 with a single replay execution failure.
	ExitComparisonFailed = 3

	ExitUsage    = 4
	ExitInternal = 5
)

// App is a testable Graybox command-line application.
type App struct {
	Stdout  io.Writer
	Stderr  io.Writer
	Version string

	createRecording createRecordingFunc
	listen          listenFunc
}

func (a App) Run(
	ctx context.Context,
	args []string,
) int {
	if len(args) == 0 {
		a.printRootHelp(
			a.Stdout,
		)
		return ExitSuccess
	}

	var err error

	code := ExitInternal

	switch args[0] {
	case "record":
		code, err = a.runRecord(
			ctx,
			args[1:],
		)

	case "ls":
		code, err = a.runList(
			ctx,
			args[1:],
		)

	case "show":
		code, err = a.runShow(
			ctx,
			args[1:],
		)

	case "replay":
		code, err = a.runReplay(
			ctx,
			args[1:],
		)

	case "diff":
		code, err = a.runDiff(
			ctx,
			args[1:],
		)

	case "version":
		code, err = a.runVersion(
			args[1:],
		)

	case "help", "-h", "--help":
		if len(args) > 1 {
			return a.helpCommand(
				args[1],
			)
		}

		a.printRootHelp(
			a.Stdout,
		)
		return ExitSuccess

	default:
		fmt.Fprintf(
			a.Stderr,
			"graybox: unknown command %q\n\n",
			args[0],
		)

		a.printRootHelp(
			a.Stderr,
		)

		return ExitUsage
	}

	if err != nil {
		fmt.Fprintf(
			a.Stderr,
			"graybox: %v\n",
			err,
		)
	}

	return code
}

func (a App) printRootHelp(
	w io.Writer,
) {
	fmt.Fprint(
		w,
		`Graybox — a local-first API flight recorder and behavioral debugger

Usage:
  graybox <command> [options]

Commands:
  record    proxy and record HTTP traffic
  ls        list recorded exchanges
  show      inspect one exchange
  replay    replay recorded requests
  diff      replay and compare behavior with a recording
  version   print the Graybox version
  help      show command help

Run "graybox help <command>" for examples and command options.
`,
	)
}

func (a App) helpCommand(
	command string,
) int {
	switch command {
	case "record":
		a.runRecord(
			context.Background(),
			[]string{"--help"},
		)

	case "ls":
		a.runList(
			context.Background(),
			[]string{"--help"},
		)

	case "show":
		a.runShow(
			context.Background(),
			[]string{"--help"},
		)

	case "replay":
		a.runReplay(
			context.Background(),
			[]string{"--help"},
		)

	case "diff":
		a.runDiff(
			context.Background(),
			[]string{"--help"},
		)

	case "version":
		a.runVersion(
			[]string{"--help"},
		)

	default:
		fmt.Fprintf(
			a.Stderr,
			"graybox: unknown command %q\n",
			command,
		)

		return ExitUsage
	}

	return ExitSuccess
}

func (a App) newFlagSet(
	name string,
	usage func(),
) *flag.FlagSet {
	fs := flag.NewFlagSet(
		name,
		flag.ContinueOnError,
	)

	fs.SetOutput(
		io.Discard,
	)

	fs.Usage = usage

	return fs
}

func parseInterspersed(
	fs *flag.FlagSet,
	args []string,
	boolean map[string]bool,
) ([]string, error) {
	var flags []string
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			positional = append(
				positional,
				args[i+1:]...,
			)

			break
		}

		if !strings.HasPrefix(
			arg,
			"-",
		) || arg == "-" {
			positional = append(
				positional,
				arg,
			)

			continue
		}

		flags = append(
			flags,
			arg,
		)

		name := strings.TrimLeft(
			strings.SplitN(
				arg,
				"=",
				2,
			)[0],
			"-",
		)

		if strings.Contains(
			arg,
			"=",
		) ||
			boolean[name] ||
			name == "h" ||
			name == "help" {
			continue
		}

		if i+1 >= len(args) {
			return nil, fmt.Errorf(
				"option %s requires a value",
				arg,
			)
		}

		i++

		flags = append(
			flags,
			args[i],
		)
	}

	if err := fs.Parse(
		flags,
	); err != nil {
		return nil, err
	}

	return positional, nil
}

type usageError struct {
	message string
}

func (e usageError) Error() string {
	return e.message
}

func classifyError(
	err error,
) int {
	var usage usageError

	if errors.As(
		err,
		&usage,
	) ||
		errors.Is(
			err,
			flag.ErrHelp,
		) {
		return ExitUsage
	}

	if errors.Is(
		err,
		storage.ErrInvalidRecording,
	) ||
		errors.Is(
			err,
			storage.ErrUnsupportedSchema,
		) {
		return ExitInvalidRecording
	}

	return ExitInternal
}

func parseID(
	value string,
) (int64, error) {
	id, err := strconv.ParseInt(
		value,
		10,
		64,
	)

	if err != nil ||
		id <= 0 {
		return 0, usageError{
			fmt.Sprintf(
				"exchange ID %q must be a positive integer",
				value,
			),
		}
	}

	return id, nil
}

func flagWasSet(
	fs *flag.FlagSet,
	name string,
) bool {
	found := false

	fs.Visit(
		func(item *flag.Flag) {
			if item.Name == name {
				found = true
			}
		},
	)

	return found
}
