# Go Upgrade — Reference

## Why `go get` before `go mod tidy`

Running `go mod tidy` alone after bumping the `go` directive is dangerous — it resolves all unversioned packages to `@latest`, which cascades into upgrading grpc, protobuf, gorm, and other unrelated dependencies. Always `go get` the specific target packages first to anchor those versions, then let tidy clean up the transitive graph.

## GOMODCACHE trick

`go mod tidy` under Go 1.26 fails with spurious errors like:

```
module github.com/pkg/errors@latest found (v0.9.1), but does not contain package github.com/pkg/errors
```

even when the package clearly exists. This happens because the default `GOMODCACHE` may have module zip files cached but not extracted — the zip is present but the source tree directory is empty, so Go reports the package as missing.

**Fix**: run `go mod tidy` with a fresh temp directory as `GOMODCACHE`. Go downloads and fully extracts each module into the fresh cache:

```bash
export GOMODCACHE=$(mktemp -d)
# then run go mod tidy in each module
```

The fresh cache is only needed for the tidy run. The module cache repopulates normally on subsequent builds.

## Why the go directive needs a manual step

`go get golang.org/x/crypto@v0.53.0` sets the `go` directive in `go.mod` to the minimum version that dep requires — e.g. `go 1.25.0`. But the intended target might be `go 1.26.0`. Without the manual step, all modules would end up at `go 1.25.0` even though the workspace is targeting 1.26.

The `update_go_directives.py` script handles this, including inserting a `toolchain` line if one doesn't exist yet (plain `sed -i ''` doesn't work reliably on macOS for multi-line insertions).

## controller-gen compatibility

`controller-gen` is installed via `go install` in `operator/Makefile` at a pinned version. Each CT version is tied to a specific k8s version and minimum Go version:

| CT version | k8s.io/* | Min Go | x/tools |
|------------|----------|--------|---------|
| v0.16.x | v0.31 | 1.22 | ~v0.24 ⚠️ |
| v0.17.x | v0.32 | 1.23 | ~v0.27 |
| v0.18.x | v0.33 | 1.23 | ~v0.30 |
| v0.19.x | v0.34 | 1.24 | ~v0.36 ✓ |
| v0.20.x | v0.35 | 1.25 | ~v0.40 ✓ |
| v0.21.x | v0.36 | 1.26 | ~v0.44 ✓ |

**The breakage**: CT v0.16.x uses `x/tools ~v0.24.0`, which contains a compile-time array assertion (`var _ [-delta*delta]byte`) that Go 1.26 rejects as an invalid negative array length. Any CT version using `x/tools >= v0.25.0` is safe.

**How to pick the right version**:
1. Check the operator's current `k8s.io/api` version in `operator/go.mod`
2. Find the CT version row whose k8s column matches
3. Verify that CT version's `x/tools` is ≥ v0.25.0 (all v0.19+ are fine)
4. Update `CONTROLLER_TOOLS_VERSION` in `operator/Makefile`

If the operator's k8s version also needs bumping, that's a larger change — coordinate k8s, controller-runtime, and CT upgrades together.

**Check a tool's x/tools version** without installing it:

```
https://pkg.go.dev/sigs.k8s.io/controller-tools@vX.Y.Z  (Dependencies tab)
```

## Other tools that may need bumping

The same `x/tools` issue can affect other tools. Check their dependency on `x/tools` the same way:

- **mockery**: installed via `installMockery` task in `build.gradle` — check `github.com/vektra/mockery/v3` deps
- **golangci-lint**: installed via `installGolangCILint` — check `github.com/golangci/golangci-lint` deps
- **protoc-gen-go / protoc-gen-go-grpc**: installed via `installProtocGenGo` / `installProtocGenGoRPC` — these track grpc/protobuf releases and rarely have this issue
