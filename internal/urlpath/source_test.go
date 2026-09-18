package urlpath_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/urlpath"
)

func TestParseSourceBaseKeepsMountPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "root", raw: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080/"},
		{name: "root slash", raw: "http://127.0.0.1:8080/", want: "http://127.0.0.1:8080/"},
		{name: "nested", raw: "https://host/bench/a/simulator", want: "https://host/bench/a/simulator/"},
		{name: "nested slash", raw: "https://host/bench/a/simulator/", want: "https://host/bench/a/simulator/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := urlpath.ParseSourceBase(tc.raw)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.want+"observations/receptions", got+"observations/receptions")
		})
	}
}

func TestParseSourceBaseRejectsAmbiguousURLs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "relative", raw: "/bench/a/simulator"},
		{name: "unsupported scheme", raw: "ftp://host/sim"},
		{name: "no host", raw: "http:///sim"},
		{name: "userinfo", raw: "http://user:pass@host/sim"},
		{name: "query", raw: "http://host/sim?a=1"},
		{name: "forced query", raw: "http://host/sim?"},
		{name: "fragment", raw: "http://host/sim#a"},
		{name: "empty segment", raw: "http://host/bench//simulator"},
		{name: "dot segment", raw: "http://host/bench/./simulator"},
		{name: "parent segment", raw: "http://host/bench/../simulator"},
		{name: "encoded separator", raw: "http://host/bench%2Fsimulator"},
		{name: "malformed escape", raw: "http://host/bench%zz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := urlpath.ParseSourceBase(tc.raw)
			require.ErrorIs(t, err, urlpath.ErrInvalid)
		})
	}
}
