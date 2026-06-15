// Separate module: the avo code generator must not pull avo into the parent tga module.
// Run via the Makefile `generate` target to regenerate the committed kernels_amd64.s
// and kernels_stub_amd64.go files in ../.
module github.com/woozymasta/tga/internal/simd/asmgen

go 1.25.0

require github.com/mmcloughlin/avo v0.6.0

require (
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/tools v0.46.0 // indirect
)
