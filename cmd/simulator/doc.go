// Command simulator serves a standalone simulator: the simulator API and the
// manager page below one public mount prefix. A separate display command
// reads its API over HTTP.
//
//	simulator -config configs/simulator.json -config-max-bytes 65536
//
// Both flags are required. See configs/README.md for the sections. Exit
// status: 0 on a clean stop, 2 for usage errors, 1 for any other failure.
package main
