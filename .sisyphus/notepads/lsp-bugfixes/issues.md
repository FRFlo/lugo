- GitNexus impact/context calls hit a `.gitnexus\lbug` lock in this workspace, so graph-backed blast-radius data could
  not be retrieved.
- `go test ./lsp/ -count=1` initially failed in InferColonMethod; the failure was unrelated to the requested bugs but
  had to be fixed for the package test suite to pass.
