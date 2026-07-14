# Contributing

## Adding a harness adapter

An adapter should make the shared Elastic Docs skills and MCP servers work in
one additional coding-agent harness without changing unrelated user settings.

1. Add a host ID, label, and executable used for detection in
   `internal/hosts/hosts.go`. Keep the ID stable: it is persisted in user
   state and accepted by `--host`.
2. Add the host branch to `adapters.Sync` in `internal/adapters/adapters.go`.
   Prefer the harness CLI when it has a supported user-scoped MCP command;
   otherwise merge its documented user configuration file atomically.
3. Configure the public `elastic-docs` server and the optional
   `elastic-internal-docs` server using `servers(internal)`. Do not overwrite a
   different existing server unless the user supplied `--force`.
4. Validate the result after writing it. Use the harness's native MCP-list
   command where available, and return a warning—not a rollback—when listing
   fails or does not show the configured server.
5. Record only files the adapter owns in `state.HostState`. Refuse to replace
   an unmanaged file or extension.
6. Add focused unit tests for conflict handling, config-path selection, and
   MCP-list output. Update the README’s supported-harness list and run:

   ```bash
   go test ./...
   go vet ./...
   ```

If the harness has no supported MCP mechanism, do not add a partial adapter.
Document what would be required and wait until it can meet the same safety and
validation guarantees as the supported hosts.
