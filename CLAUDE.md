# PayCore

Follow the coding rules (type safety and layered architecture) in @docs/coding_rules.md for all Go (`api/`) and TypeScript (`web/`) changes.

Before finishing a change, run the checks it lists: `cd web; npm run check` and `cd api; go vet ./...; go test ./...` (plus `golangci-lint run` when installed).
