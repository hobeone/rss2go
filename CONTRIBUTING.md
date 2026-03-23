# Contributing to rss2go

Thank you for your interest in contributing to `rss2go`! To ensure a smooth process, please follow these guidelines.

## Development Setup

1.  **Go Version**: Ensure you have Go 1.24+ installed.
2.  **Clone the Repo**:
    ```bash
    git clone https://github.com/hobeone/rss2go.git
    cd rss2go
    ```
3.  **Install Linters**: We use `golangci-lint`.
    ```bash
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    ```

## Workflow

1.  **Branching**: Create a feature branch for your changes.
2.  **Coding Standards**:
    *   Follow standard Go idioms.
    *   Ensure code is formatted with `gofmt` or `goimports`.
    *   Run `golangci-lint run` before submitting.
3.  **Testing**:
    *   Write tests for new functionality.
    *   Run the full suite: `go test -race ./...`.
    *   All tests must pass.
4.  **Database Changes**:
    *   Never modify existing migration files in `migrations/`.
    *   Add a new `.sql` file for any schema changes.
5.  **Pull Requests**:
    *   Provide a clear description of the changes.
    *   Ensure CI passes on your PR.

## Bug Reports

Please use the GitHub Issues tracker to report bugs. Include:
*   Steps to reproduce.
*   Expected vs. actual behavior.
*   Relevant logs or error messages.
