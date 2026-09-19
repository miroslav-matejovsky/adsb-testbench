// Package cli implements the shared lifecycle of the executable commands.
//
// Each command's main calls [Main] with its [testbench.Mode]; everything
// else happens in [Run], which receives its arguments and an [Environment]
// (output writers, file reading, listener creation and run-identity
// generation) so tests never call os.Exit or bind fixed addresses.
//
// # Flags and configuration
//
// Every command requires -config (a file path) and -config-max-bytes (a
// positive size bound); unknown flags and positional arguments are usage
// errors, and -help succeeds. The file is one complete JSON object whose
// sections are exactly those of the mode; missing, null, unknown and
// duplicate keys, and trailing values are rejected. A nonempty "$comment"
// string documents the settings inside the file. Nested sections reuse the
// public parsers of simulator, display and ui, and [ParseFile] checks
// cross-section constraints: unique initial stations, a local aircraft
// selection naming configured stations, browser byte budgets not below the
// server bounds, a server write timeout above every handler deadline, and a
// source timeout within the display deadline. Nothing is defaulted and no
// environment variable is read.
//
// # Startup and shutdown
//
// Validation completes before any runtime is created or listener opened.
// Simulator modes turn the configured simulation ID label into a fresh run
// ID with [FreshRunID], create the runtime paused, install the configured
// stations through the service, bind the validated address, restore the
// configured speed, and only then serve. The display mode builds its HTTP
// transport from explicit settings and closes idle connections on exit.
// Serving uses internal/httpserver: on cancellation HTTP drains first, then
// the application stops. Startup address, mode and effective run ID are
// logged; a fatal error is logged once and returned with its causes.
// [ExitStatus] maps the result to 0 (success or -help), 2 (usage) or 1.
//
// [ValidateListenAddress] checks an explicit host:port without binding a
// socket or resolving a name.
package cli
