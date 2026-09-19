module example.com/adsb-testbench-embedding

go 1.27.1

require github.com/miroslav-matejovsky/adsb-testbench v0.0.0

require (
	github.com/ccoveille/go-safecast/v2 v2.0.0 // indirect
	kreklow.us/go/go-adsb v0.4.1 // indirect
)

// The example compiles against the repository checkout, using only its
// public packages.
replace github.com/miroslav-matejovsky/adsb-testbench => ../..
