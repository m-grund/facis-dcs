package builder

import _ "embed"

// notoSansRegular and notoSansBold are vendored Noto Sans TTF files (Apache 2.0).
// These are pinned at build time so PDF output is deterministic: the same JSON-LD
// input always produces the same base PDF bytes and the same SHA-256 (DCS-FR-CWE-04).
// Never update these files without re-baselining all determinism test fixtures.

//go:embed fonts/NotoSans-Regular.ttf
var notoSansRegular []byte

//go:embed fonts/NotoSans-Bold.ttf
var notoSansBold []byte
