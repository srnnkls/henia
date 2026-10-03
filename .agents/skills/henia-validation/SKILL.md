---
name: henia-validation
description: Henia's native validation, the gate CI runs. Read only when listed as a slot provider.
metadata:
  provides: code.validation.go
  code.validation.go: "go vet ./... && go build ./... && go test ./... && mise run test:integration"
---

# Henia validation

The `code.validation.go` command mirrors the `lint`, `build` and `integration`
jobs in [CI](../../../.github/workflows/ci.yml).
