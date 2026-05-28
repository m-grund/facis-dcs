package builder

import "fmt"

// xmpMetadata returns the XMP metadata stream bytes for PDF/A-3 conformance level U.
// This is injected via fpdf.SetXmpMetadata so the PDF is marked as PDF/A-3U,
// allowing embedded file attachments (DCS-FR-SM-27, DCS-FR-CSA-06).
func xmpMetadata(title, did string) []byte {
	return []byte(fmt.Sprintf(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about=""
        xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/"
        xmlns:dc="http://purl.org/dc/elements/1.1/"
        xmlns:xmp="http://ns.adobe.com/xap/1.0/">
      <pdfaid:part>3</pdfaid:part>
      <pdfaid:conformance>U</pdfaid:conformance>
      <dc:title>
        <rdf:Alt>
          <rdf:li xml:lang="x-default">%s</rdf:li>
        </rdf:Alt>
      </dc:title>
      <dc:description>
        <rdf:Alt>
          <rdf:li xml:lang="x-default">DCS contract %s</rdf:li>
        </rdf:Alt>
      </dc:description>
      <xmp:CreatorTool>%s</xmp:CreatorTool>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`, title, did, producer))
}
