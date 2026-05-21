# Contributing

## Development Setup

1. Install the pinned toolchain with `mise install`.
2. Run the test suite with `mise exec -- go test ./...`.
3. Run static checks with `mise exec -- go vet ./...`.

## Pull Requests

1. Keep changes focused on one concern.
2. Add or update tests when behavior changes.
3. Make sure `mise exec -- go test ./...` passes before opening a pull request.

## Release Process

1. Push changes to `main` or `master`.
2. Release Please opens or updates a release PR.
3. Merging that PR creates a `vX.Y.Z` tag, updates release notes, and triggers artifact publishing.
