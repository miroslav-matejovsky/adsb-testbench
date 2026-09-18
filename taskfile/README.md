# Task scripts

PowerShell helpers for repository cleanup, unit-test output, command reachability,
and dependency vendoring, invoked through the root `Taskfile.yml`.

`task all` runs tidy, vet, formatting, architecture lint, code lint, and package
tests. The skeleton has no executable entry points, so command reachability is
deferred until the [UI and commands plan](../docs/plans/08-ui-and-commands/README.md)
adds executable commands and enables reachability in step 12.
