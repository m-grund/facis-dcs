package builder

import (
	"fmt"
	"sync"

	"github.com/go-pdf/fpdf"
)

var semanticTagMu sync.Mutex
var semanticTagState = map[*fpdf.Fpdf]map[int]int{}

func semanticReset(f *fpdf.Fpdf) {
	semanticTagMu.Lock()
	defer semanticTagMu.Unlock()
	semanticTagState[f] = map[int]int{}
}

func semanticNextMCID(f *fpdf.Fpdf) int {
	semanticTagMu.Lock()
	defer semanticTagMu.Unlock()
	perPage, ok := semanticTagState[f]
	if !ok {
		perPage = map[int]int{}
		semanticTagState[f] = perPage
	}
	page := f.PageNo()
	mcid := perPage[page]
	perPage[page] = mcid + 1
	return mcid
}

func semanticWithTag(f *fpdf.Fpdf, tag string, draw func()) {
	if draw == nil {
		return
	}
	mcid := semanticNextMCID(f)
	f.RawWriteStr(fmt.Sprintf("/%s <</MCID %d>> BDC\n", tag, mcid))
	draw()
	f.RawWriteStr("\nEMC\n")
}

func semanticArtifact(f *fpdf.Fpdf, draw func()) {
	if draw == nil {
		return
	}
	f.RawWriteStr("/Artifact BMC\n")
	draw()
	f.RawWriteStr("\nEMC\n")
}
