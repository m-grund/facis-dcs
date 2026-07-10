package sign

import (
	"fmt"
	"sort"
)

// writeIncrXrefTable writes the incremental cross-reference table to the
// output buffer. updatedXrefEntries (existing objects whose value changed,
// e.g. a signature field's /V) and newXrefEntries (freshly appended objects)
// are combined, sorted by object ID, and grouped into subsections only where
// IDs are actually contiguous — mirroring how the compiler package's own
// incremental-update writer (buildUpdateAppendixBytes) does it, which is
// known-good against external C2PA/PDF validators (c2patool, veraPDF).
// Previously each updatedXrefEntries entry got its own single-entry "id 1"
// subsection header even when adjacent to another entry, and the c2patool
// (c2pa-rs) PDF reader rejected the resulting multi-subsection xref outright
// ("failed parsing cross reference table: invalid start value") for any
// DCS-signed PDF — i.e. every one, since every signature triggers this path.
func (context *SignContext) writeIncrXrefTable() error {
	all := make([]xrefEntry, 0, len(context.updatedXrefEntries)+len(context.newXrefEntries))
	all = append(all, context.updatedXrefEntries...)
	all = append(all, context.newXrefEntries...)
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	if _, err := context.OutputBuffer.Write([]byte("xref\n")); err != nil {
		return fmt.Errorf("failed to write incremental xref header: %w", err)
	}

	i := 0
	for i < len(all) {
		j := i + 1
		for j < len(all) && all[j].ID == all[j-1].ID+1 {
			j++
		}
		subsectionHeader := fmt.Sprintf("%d %d\n", all[i].ID, j-i)
		if _, err := context.OutputBuffer.Write([]byte(subsectionHeader)); err != nil {
			return fmt.Errorf("failed to write xref subsection header: %w", err)
		}
		for k := i; k < j; k++ {
			xrefLine := fmt.Sprintf("%010d 00000 n\r\n", all[k].Offset)
			if _, err := context.OutputBuffer.Write([]byte(xrefLine)); err != nil {
				return fmt.Errorf("failed to write incremental xref entry: %w", err)
			}
		}
		i = j
	}

	return nil
}
