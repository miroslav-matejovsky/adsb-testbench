package urlpath_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
)

func TestParseMountPrefixNormalizesTrailingSlash(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{raw: "/", want: "/"},
		{raw: "/bench", want: "/bench/"},
		{raw: "/bench/a", want: "/bench/a/"},
		{raw: "/bench/a/", want: "/bench/a/"},
		{raw: "/bench-1/a_b.c/", want: "/bench-1/a_b.c/"},
		{raw: "/bench%20a/", want: "/bench%20a/"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()

			got, err := urlpath.ParseMountPrefix(tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.want+"api/simulator/truth", got+"api/simulator/truth")
		})
	}
}

// rejectedPathVectors are ambiguous path forms rejected by every path parser.
var rejectedPathVectors = []struct {
	name string
	raw  string
}{
	{name: "empty", raw: ""},
	{name: "relative", raw: "bench/a/"},
	{name: "protocol relative", raw: "//host/bench/"},
	{name: "duplicate slash", raw: "/bench//a/"},
	{name: "double trailing slash", raw: "/bench/a//"},
	{name: "dot segment", raw: "/bench/./a/"},
	{name: "parent segment", raw: "/bench/../a/"},
	{name: "trailing parent", raw: "/bench/.."},
	{name: "encoded dot", raw: "/bench/%2e/a/"},
	{name: "encoded parent", raw: "/bench/%2E%2e/a/"},
	{name: "mixed encoded parent", raw: "/bench/.%2E/a/"},
	{name: "encoded slash", raw: "/bench%2Fa/"},
	{name: "encoded backslash", raw: "/bench%5Ca/"},
	{name: "literal backslash", raw: `/bench\a/`},
	{name: "leading backslash", raw: `/\host/`},
	{name: "query", raw: "/bench/?a=1"},
	{name: "empty query", raw: "/bench/?"},
	{name: "fragment", raw: "/bench/#a"},
	{name: "empty fragment", raw: "/bench/#"},
	{name: "control byte", raw: "/bench\n/"},
	{name: "tab", raw: "/bench\t/"},
	{name: "delete byte", raw: "/bench\x7f/"},
	{name: "encoded control byte", raw: "/bench%0D%0A/"},
	{name: "malformed escape", raw: "/bench%zz/"},
}

func TestParseMountPrefixRejectsAmbiguousPaths(t *testing.T) {
	t.Parallel()

	extra := []struct {
		name string
		raw  string
	}{
		{name: "absolute URL", raw: "http://host/bench/"},
		{name: "scheme only", raw: "javascript:alert(1)"},
	}
	for _, tc := range append(extra, rejectedPathVectors...) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := urlpath.ParseMountPrefix(tc.raw)
			require.ErrorIs(t, err, urlpath.ErrInvalid)
		})
	}
}

func TestParseBrowserBaseAcceptsRootRelativeAndAbsolute(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{raw: "/api/simulator", want: "/api/simulator/"},
		{raw: "/bench/a/api/simulator/", want: "/bench/a/api/simulator/"},
		{raw: "http://127.0.0.1:8080/bench/a/assets", want: "http://127.0.0.1:8080/bench/a/assets/"},
		{raw: "https://host/", want: "https://host/"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()

			got, err := urlpath.ParseBrowserBase(tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseBrowserBaseRejectsAmbiguousURLs(t *testing.T) {
	t.Parallel()

	extra := []struct {
		name string
		raw  string
	}{
		{name: "javascript scheme", raw: "javascript:alert(1)"},
		{name: "data scheme", raw: "data:text/html,x"},
		{name: "userinfo", raw: "http://user@host/api/"},
		{name: "absolute backslash", raw: `http://host/api\x/`},
		{name: "absolute encoded dot", raw: "http://host/api/%2e%2e/"},
	}
	for _, tc := range append(extra, rejectedPathVectors...) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := urlpath.ParseBrowserBase(tc.raw)
			require.ErrorIs(t, err, urlpath.ErrInvalid)
		})
	}
}
