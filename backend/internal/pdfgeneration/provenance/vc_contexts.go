package provenance

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/piprate/json-gold/ld"
)

// The JSON-LD contexts every lifecycle/summary VC proof canonicalizes against
// are embedded at compile time and preloaded into the document loader:
// RDFC-1.0 normalization runs on every PDF export and signature apply, and a
// default (remote-fetching, uncached) loader turns each of those into live
// w3id.org/w3.org HTTP round-trips — a runtime internet dependency that
// collapses under BDD-suite load and is unavailable in hermetic CI. The
// embedded copies are the published, versioned W3C documents; contexts outside
// this set still resolve through the fallback remote loader.
//
//go:embed contexts/credentials-v2.json contexts/data-integrity-v2.json
var vcContextFS embed.FS

var embeddedVCContexts = map[string]string{
	"https://www.w3.org/ns/credentials/v2":        "contexts/credentials-v2.json",
	"https://w3id.org/security/data-integrity/v2": "contexts/data-integrity-v2.json",
}

var (
	vcLoaderOnce sync.Once
	vcLoader     *ld.CachingDocumentLoader
)

// vcDocumentLoader returns the process-wide caching JSON-LD document loader,
// preloaded with the embedded VC contexts. Preloading is fatal on error: a
// missing or unparsable embedded context would otherwise silently degrade to
// remote fetching, which is exactly the failure mode this exists to remove.
func vcDocumentLoader() ld.DocumentLoader {
	vcLoaderOnce.Do(func() {
		vcLoader = ld.NewCachingDocumentLoader(ld.NewDefaultDocumentLoader(nil))
		for url, path := range embeddedVCContexts {
			raw, err := vcContextFS.ReadFile(path)
			if err != nil {
				panic(fmt.Sprintf("provenance: embedded VC context %s missing: %v", path, err))
			}
			doc, err := ld.DocumentFromReader(bytes.NewReader(raw))
			if err != nil {
				panic(fmt.Sprintf("provenance: embedded VC context %s unparsable: %v", path, err))
			}
			vcLoader.AddDocument(url, doc)
		}
	})
	return vcLoader
}
