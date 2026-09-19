// Command adsb-testbench serves a combined test bench: one simulator with
// its API and manager, and one display that reads the same simulator in
// process, below one public mount prefix.
//
//	adsb-testbench -config configs/combined.json -config-max-bytes 65536
//
// Both flags are required. The configuration sections and their meaning are
// documented in configs/README.md. Interrupt or termination drains HTTP and
// stops the simulator; the exit status is 0 on a clean stop, 2 for usage
// errors and 1 for any other failure.
package main
