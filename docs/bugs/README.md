# Bugs

Observed defects awaiting a fix. One bug per file. These are confirmed wrong behaviors
seen in real captures or demonstrated by code review and focused tests, distinct from the
optional enhancements in
[`../backlog/`](../backlog/README.md).

A file may also record a defect found by reading the code that no capture has shown
yet. Its **Status** says so, and its fix direction names the verification the fix
still owes. A data-only investigation may leave exact code localization to the fix
when the captured behavior already proves the defect.

Each file is self-contained and has a yaml front matter section with the following fields:

```yaml
status: "Observed|Awaiting reproduction"
kind: "Regression|New|Existing"
complexity: "trivial|easy|medium|hard"
severity: "trivial|minor|major|critical"
```

Then the file continues with the detailed description of the defect:

- **Area** - the subsystem that owns the defect.
- **Symptom** - what was observed, with the capture path and log/issue excerpts that
  prove it.
- **Root cause** - the code path at fault, cited by `file` and function.
- **Impact** - who or what it breaks, and when.
- **Fix direction** - the intended remedy, not a committed design.

Remove a file when its code fix is implemented. Track any still-needed live
verification as a direct action in the root `.todo` file.

## Open bugs

None.
