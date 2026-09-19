package urlpath

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalid identifies a URL outside the accepted domain of this package.
var ErrInvalid = errors.New("invalid URL")

// ParseSourceBase validates an absolute http or https base URL that a client
// appends relative route paths to, and returns it with exactly one trailing
// slash.
//
// The whole mount prefix is preserved, so "https://host/bench/a/simulator"
// and "https://host/bench/a/simulator/" both become
// "https://host/bench/a/simulator/" and appending "receptions/history"
// addresses the mounted route rather than the server root.
//
// Userinfo, queries, fragments (including empty ones), invalid ports, empty
// path segments, dot segments, control bytes, backslashes, and percent-encoded
// path separators or dot segments are rejected. Each of them would make the
// appended route resolve somewhere the caller did not name.
func ParseSourceBase(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("%w: base URL is empty", ErrInvalid)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: parse base URL %q: %w", ErrInvalid, raw, err)
	}
	switch {
	case parsed.Scheme != "http" && parsed.Scheme != "https":
		return "", fmt.Errorf("%w: base URL %q must use http or https", ErrInvalid, raw)
	case parsed.Host == "":
		return "", fmt.Errorf("%w: base URL %q has no host", ErrInvalid, raw)
	case parsed.User != nil:
		return "", fmt.Errorf("%w: base URL %q must not carry userinfo", ErrInvalid, raw)
	case parsed.Opaque != "":
		return "", fmt.Errorf("%w: base URL %q is not hierarchical", ErrInvalid, raw)
	}
	if err := validPort(raw, parsed); err != nil {
		return "", err
	}
	if err := validPathText(raw, parsed.EscapedPath()); err != nil {
		return "", err
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/"
	parsed.RawPath = ""
	return parsed.String(), nil
}

// validPort rejects an empty or out-of-range explicit port. A missing port is
// accepted and means the scheme default.
func validPort(raw string, parsed *url.URL) error {
	if strings.HasSuffix(parsed.Host, ":") {
		return fmt.Errorf("%w: base URL %q has an empty port", ErrInvalid, raw)
	}
	port := parsed.Port()
	if port == "" {
		return nil
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("%w: base URL %q has an invalid port", ErrInvalid, raw)
	}
	return nil
}
