# Workspace Mandates

The following steps are REQUIRED for any interaction or change in this workspace:

1. **Run Tests**: Always execute `go test ./...` to ensure all tests pass and no regressions are introduced.
2. **Run go fix**: Execute `go fix ./...` to ensure the codebase remains compliant with Go's automated fixes.
3. **Verify Build**: Ensure that `cmd/rss2go` and other packages build successfully without any missing imports or compilation errors.
4. **Database Migrations**: Any database schema change MUST be done by adding a new migration file in the `migrations/` directory to modify the existing schema. Do NOT modify existing migration files.
