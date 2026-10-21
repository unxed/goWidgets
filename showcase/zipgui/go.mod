module github.com/unxed/goWidgets/showcase/zipgui

go 1.26.0

require (
	github.com/unxed/goWidgets v0.0.0-00010101000000-000000000000
	github.com/unxed/zipper v0.1.179
)

require (
	github.com/STARRY-S/zip v0.2.3 // indirect
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/bodgit/plumbing v1.3.0 // indirect
	github.com/bodgit/windows v1.0.1 // indirect
	github.com/dsnet/compress v0.0.2-0.20230904184137-39efe44ab707 // indirect
	github.com/ebitengine/purego v0.9.0 // indirect
	github.com/go-webgpu/goffi v0.6.2 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/minio/minlz v1.1.1 // indirect
	github.com/ncruces/go-sqlite3 v0.35.2 // indirect
	github.com/ncruces/go-sqlite3-wasm/v3 v3.2.35303 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/nwaples/rardecode/v2 v2.4.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.27 // indirect
	github.com/sorairolake/lzip-go v0.3.8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/stangelandcl/ppmd v0.1.1 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/unxed/archives v0.1.3 // indirect
	github.com/unxed/kiwi-go v0.1.0 // indirect
	github.com/unxed/localecp v0.1.6 // indirect
	github.com/unxed/par2 v0.1.3 // indirect
	github.com/unxed/sevenzip v0.1.7 // indirect
	github.com/unxed/tar v0.1.137 // indirect
	github.com/unxed/winkeys v0.1.1 // indirect
	github.com/unxed/xz v0.1.48 // indirect
	github.com/unxed/zip v0.1.145 // indirect
	github.com/unxed/zipcharset v0.1.5 // indirect
	github.com/unxed/zlib4go v0.1.16 // indirect
	go4.org v0.0.0-20260112195520-a5071408f32f // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

// The library lives two levels up (ADR-0019): zipgui always builds against
// the goWidgets in the same checkout.
replace github.com/unxed/goWidgets => ../..

// Same as the library module: replace directives of a dependency do not
// apply here, so the purego fork has to be named again.
replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.19
