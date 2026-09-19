package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/miroslav-matejovsky/adsb-testbench/internal/cli"
)

func TestValidateListenAddressAccepts(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"127.0.0.1:8080",
		"127.0.0.1:0",
		"0.0.0.0:80",
		"[::1]:8080",
		"[::]:8080",
		"[fe80::1%eth0]:8080",
		"localhost:65535",
		"bench-1.example.com:443",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, cli.ValidateListenAddress(address))
		})
	}
}

func TestValidateListenAddressRejects(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"",
		":8080",
		"127.0.0.1",
		"127.0.0.1:",
		"127.0.0.1:http",
		"127.0.0.1:-1",
		"127.0.0.1:+80",
		"127.0.0.1:65536",
		"::1:8080",
		"[127.0.0.1]:8080",
		"[::1]",
		"http://127.0.0.1:8080",
		"127.0.0.1:8080/",
		"host_name:80",
		"-host:80",
		"host..name:80",
		"ho st:80",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, cli.ValidateListenAddress(address), cli.ErrInvalidAddress)
		})
	}
}
