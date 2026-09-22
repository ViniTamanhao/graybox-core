package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/ViniTamanhao/graybox-core/internal/storage"
)

type metadataReader interface {
	Metadata(
		context.Context,
		string,
	) (string, error)
}

func resolveExecutionTarget(
	ctx context.Context,
	metadata metadataReader,
	targetValue string,
	unsafeOriginalTarget bool,
) (*url.URL, error) {
	usingOriginal := targetValue == ""

	if usingOriginal {
		savedTarget, err := metadata.Metadata(
			ctx,
			"target_url",
		)
		if errors.Is(
			err,
			storage.ErrNotFound,
		) {
			return nil, usageError{
				"recording has no original target; provide --target",
			}
		}
		if err != nil {
			return nil, fmt.Errorf(
				"read recording target: %w",
				err,
			)
		}

		targetValue = savedTarget
	}

	target, err := parseTarget(
		targetValue,
	)
	if err != nil {
		return nil, err
	}

	if usingOriginal &&
		!unsafeOriginalTarget &&
		!isLoopbackTarget(target) {
		return nil, usageError{
			fmt.Sprintf(
				"recording target %q is not loopback; "+
					"provide --target or explicitly allow "+
					"--unsafe-original-target",
				target.String(),
			),
		}
	}

	return target, nil
}

func parseTarget(
	value string,
) (*url.URL, error) {
	if value == "" {
		return nil, usageError{
			"--target is required",
		}
	}

	if !strings.Contains(
		value,
		"://",
	) {
		return nil, usageError{
			fmt.Sprintf(
				"target %q is missing a URL scheme; try http://%s",
				value,
				value,
			),
		}
	}

	parsed, err := url.Parse(
		value,
	)
	if err != nil {
		return nil, usageError{
			fmt.Sprintf(
				"invalid target %q: %v",
				value,
				err,
			),
		}
	}

	parsed.Scheme = strings.ToLower(
		parsed.Scheme,
	)

	if (parsed.Scheme != "http" &&
		parsed.Scheme != "https") ||
		parsed.Host == "" {
		return nil, usageError{
			fmt.Sprintf(
				"target %q must be an absolute HTTP or HTTPS URL",
				value,
			),
		}
	}

	if parsed.User != nil ||
		parsed.Fragment != "" ||
		parsed.RawQuery != "" {
		return nil, usageError{
			fmt.Sprintf(
				"target %q must not contain credentials, a query, or a fragment",
				value,
			),
		}
	}

	return parsed, nil
}

func isLoopbackTarget(
	target *url.URL,
) bool {
	host := strings.TrimSuffix(
		strings.ToLower(
			target.Hostname(),
		),
		".",
	)

	if host == "localhost" ||
		strings.HasSuffix(
			host,
			".localhost",
		) {
		return true
	}

	ip := net.ParseIP(
		host,
	)

	return ip != nil &&
		ip.IsLoopback()
}
