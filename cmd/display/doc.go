// Command display serves a standalone aircraft display that reads received
// evidence and station discovery over HTTP from a simulator command's API.
// Browsers only contact this command.
//
//	display -config configs/display.json -config-max-bytes 65536
//
// Both flags are required. See configs/README.md for the source and
// transport sections. Exit status: 0 on a clean stop, 2 for usage errors, 1
// for any other failure.
package main
