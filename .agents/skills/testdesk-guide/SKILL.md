---
name: testdesk-guide
description: Use TestDesk MCP to inspect capabilities, discover test IDs and choose a focused test workflow for a configured local project.
---

# TestDesk workflow

Check `testdesk_capabilities` and a compact `testdesk_snapshot` before unfamiliar runs. Search `testdesk_tests` with a narrow query; retain returned IDs rather than inventing selectors. Include `project` when targeting a particular repository.

Plan broad runs before execution. Queue a focused selection, wait for completion and inspect bounded diagnostics on failure. A plan's item count reflects execution units, which may be individual tests, files or a whole provider. It is not always a test count.

TestDesk uses installed test runners and trusted local configuration. A provider template is not a guarantee of compatibility with every version or platform. Use `testdesk_settings` to inspect the actual commands. Do not install frameworks or edit project configuration merely to make a test green.
