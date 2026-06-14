package compiler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"sort"
	"strings"
)

func buildC2PAExclusions(streamStart, streamLen int) []c2paExclusion {
	if streamLen <= 0 {
		return nil
	}
	return []c2paExclusion{{Start: streamStart, Length: streamLen}}
}

func exclusionsEqual(a, b []c2paExclusion) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findObjectStreamRange(pdf []byte, objectID int) (int, int, bool) {
	marker := []byte(fmt.Sprintf("%d 0 obj\n", objectID))
	objPos := bytes.Index(pdf, marker)
	if objPos < 0 {
		return 0, 0, false
	}
	streamStartRel := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStartRel < 0 {
		return 0, 0, false
	}
	streamStart := objPos + streamStartRel + len("stream\n")
	streamEndRel := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEndRel < 0 {
		return 0, 0, false
	}
	streamEnd := streamStart + streamEndRel
	return streamStart, streamEnd - streamStart, true
}

func findLastObjectStreamRange(pdf []byte, objectID int) (int, int, bool) {
	objPos := findLastObjectHeaderOffset(pdf, objectID)
	if objPos < 0 {
		return 0, 0, false
	}
	streamStartRel := bytes.Index(pdf[objPos:], []byte("stream\n"))
	if streamStartRel < 0 {
		return 0, 0, false
	}
	streamStart := objPos + streamStartRel + len("stream\n")
	streamEndRel := bytes.Index(pdf[streamStart:], []byte("\nendstream"))
	if streamEndRel < 0 {
		return 0, 0, false
	}
	streamEnd := streamStart + streamEndRel
	return streamStart, streamEnd - streamStart, true
}

func findLastObjectHeaderOffset(pdf []byte, objID int) int {
	headerAtStart := []byte(fmt.Sprintf("%d 0 obj\n", objID))
	headerWithNewline := []byte(fmt.Sprintf("\n%d 0 obj\n", objID))
	best := -1
	if bytes.HasPrefix(pdf, headerAtStart) {
		best = 0
	}
	searchFrom := 0
	for {
		rel := bytes.Index(pdf[searchFrom:], headerWithNewline)
		if rel < 0 {
			break
		}
		best = searchFrom + rel + 1
		searchFrom = best + 1
	}
	return best
}

func sha256WithExclusions(data []byte, exclusions []c2paExclusion) [32]byte {
	if len(exclusions) == 0 {
		return sha256.Sum256(data)
	}

	ranges := make([]c2paExclusion, 0, len(exclusions))
	for _, exclusion := range exclusions {
		if exclusion.Length <= 0 {
			continue
		}
		ranges = append(ranges, exclusion)
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start < ranges[j].Start
	})

	hasher := sha256.New()
	pos := 0
	for _, exclusion := range ranges {
		if exclusion.Start > len(data) {
			break
		}
		if pos < exclusion.Start {
			_, _ = hasher.Write(data[pos:exclusion.Start])
		}
		excludeEnd := exclusion.Start + exclusion.Length
		if excludeEnd > len(data) {
			excludeEnd = len(data)
		}
		if excludeEnd > pos {
			pos = excludeEnd
		}
	}
	if pos < len(data) {
		_, _ = hasher.Write(data[pos:])
	}

	var out [32]byte
	copy(out[:], hasher.Sum(nil))
	return out
}

func parseBMFFBoxes(data []byte) ([]bmffBox, error) {
	var boxes []bmffBox
	for pos := 0; pos+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if size < 8 || pos+size > len(data) {
			return nil, fmt.Errorf("invalid BMFF box framing")
		}
		payloadStart := pos + 8
		boxes = append(boxes, bmffBox{
			Type:    string(data[pos+4 : pos+8]),
			Payload: append([]byte(nil), data[payloadStart:pos+size]...),
			Raw:     append([]byte(nil), data[pos:pos+size]...),
		})
		pos += size
	}
	return boxes, nil
}

func extractTopLevelManifestBoxes(c2pa []byte) ([][]byte, error) {
	rootBoxes, err := parseBMFFBoxes(c2pa)
	if err != nil {
		return nil, err
	}
	if len(rootBoxes) == 0 || rootBoxes[0].Type != "jumb" {
		return nil, fmt.Errorf("C2PA root JUMBF box not found")
	}
	rootChildren, err := parseBMFFBoxes(rootBoxes[0].Payload)
	if err != nil {
		return nil, err
	}
	var manifests [][]byte
	for _, child := range rootChildren {
		if child.Type == "jumb" {
			manifests = append(manifests, child.Raw)
		}
	}
	return manifests, nil
}

func extractJUMBFLabel(jumbBox []byte) (string, error) {
	boxes, err := parseBMFFBoxes(jumbBox)
	if err != nil {
		return "", err
	}
	if len(boxes) != 1 || boxes[0].Type != "jumb" {
		return "", fmt.Errorf("JUMBF superbox expected")
	}
	children, err := parseBMFFBoxes(boxes[0].Payload)
	if err != nil {
		return "", err
	}
	if len(children) == 0 || children[0].Type != "jumd" {
		return "", fmt.Errorf("JUMBF description box missing")
	}
	jumd := children[0].Payload
	if len(jumd) < 17 {
		return "", fmt.Errorf("JUMBF description box too small")
	}
	labelBytes := jumd[17:]
	idx := bytes.IndexByte(labelBytes, 0x00)
	if idx < 0 {
		return "", fmt.Errorf("JUMBF label terminator missing")
	}
	return string(labelBytes[:idx]), nil
}

func extractLabeledChildJUMBFBox(parentJumbBox []byte, label string) ([]byte, error) {
	boxes, err := parseBMFFBoxes(parentJumbBox)
	if err != nil {
		return nil, err
	}
	if len(boxes) != 1 || boxes[0].Type != "jumb" {
		return nil, fmt.Errorf("JUMBF superbox expected")
	}
	children, err := parseBMFFBoxes(boxes[0].Payload)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.Type != "jumb" {
			continue
		}
		childLabel, err := extractJUMBFLabel(child.Raw)
		if err == nil && childLabel == label {
			return child.Raw, nil
		}
	}
	return nil, fmt.Errorf("child JUMBF box %s not found", label)
}

func renderC2PAManifestStore(ctx context.Context, payloadHash string, hardBindingHash []byte, exclusions []c2paExclusion) ([]byte, error) {
	// Build a deterministic, syntactically valid JUMBF C2PA manifest-store:
	//   jumb(c2pa) -> jumb(c2ma)
	//                    |- jumb(c2pa.assertions)
	//                    |- jumb(c2pa.claim.v2)
	//                    |- jumb(c2pa.signature)

	hardBindingAssertionPayload := renderMinimalDataHashAssertionCBOR(hardBindingHash, exclusions)
	hardBindingAssertionBox := renderJUMBFSuperbox(cborUUID, 0x03, "c2pa.hash.data", [][]byte{renderBMFFBox("cbor", hardBindingAssertionPayload)})
	hardBindingAssertionHash := sha256.Sum256(hardBindingAssertionBox[8:])

	actionsAssertionPayload := renderMinimalActionsAssertionCBOR()
	actionsAssertionBox := renderJUMBFSuperbox(cborUUID, 0x03, "c2pa.actions.v2", [][]byte{renderBMFFBox("cbor", actionsAssertionPayload)})
	actionsAssertionHash := sha256.Sum256(actionsAssertionBox[8:])

	assertionStore := renderJUMBFSuperbox(c2paAsrtUUID, 0x03, "c2pa.assertions", [][]byte{hardBindingAssertionBox, actionsAssertionBox})

	claimPayload := renderMinimalClaimCBOR(
		payloadHash,
		hardBindingAssertionHash[:],
		"self#jumbf=c2pa.assertions/c2pa.hash.data",
		actionsAssertionHash[:],
		"self#jumbf=c2pa.assertions/c2pa.actions.v2",
	)
	claimBox := renderJUMBFSuperbox(c2paClmUUID, 0x03, "c2pa.claim.v2", [][]byte{renderBMFFBox("cbor", claimPayload)})

	// Tag(18) COSE_Sign1 with protected headers containing:
	//   1 -> -8   (alg=EdDSA)
	//   33 -> [certDer]  (x5chain per RFC 9360)
	// plus empty unprotected map, detached payload and a real signature over claim bytes.
	protected := buildCoseProtectedHeadersWithX5Chain()
	sig, err := signClaimSigStructure(ctx, protected, claimPayload)
	if err != nil {
		return nil, err
	}
	signaturePayload := []byte{0xD2, 0x84}
	signaturePayload = append(signaturePayload, cborBytes(protected)...)
	signaturePayload = append(signaturePayload, 0xA0, 0xF6)
	signaturePayload = append(signaturePayload, cborBytes(sig)...)
	signatureBox := renderJUMBFSuperbox(c2paSigUUID, 0x03, "c2pa.signature", [][]byte{renderBMFFBox("cbor", signaturePayload)})

	manifestLabel := urnUUIDFromHash(payloadHash)
	manifestBox := renderJUMBFSuperbox(c2paManifUUID, 0x03, manifestLabel, [][]byte{assertionStore, claimBox, signatureBox})
	return renderJUMBFSuperbox(c2paStoreUUID, 0x03, "c2pa", [][]byte{manifestBox}), nil
}

func renderVerificationManifestStore(ctx context.Context, originalC2PA []byte, manifestLabel string, payloadHash string, hardBindingHash []byte, exclusions []c2paExclusion) ([]byte, error) {
	manifestBoxes, err := extractTopLevelManifestBoxes(originalC2PA)
	if err != nil {
		return nil, err
	}
	if len(manifestBoxes) == 0 {
		return nil, fmt.Errorf("no manifests found in original C2PA store")
	}
	originalManifestBox := manifestBoxes[len(manifestBoxes)-1]
	originalManifestLabel, err := extractJUMBFLabel(originalManifestBox)
	if err != nil {
		return nil, err
	}
	originalSignatureBox, err := extractLabeledChildJUMBFBox(originalManifestBox, "c2pa.signature")
	if err != nil {
		return nil, err
	}
	originalManifestHash := sha256.Sum256(originalManifestBox[8:])
	originalSignatureHash := sha256.Sum256(originalSignatureBox[8:])

	updateManifestBox, err := renderVerificationUpdateManifest(ctx, manifestLabel, payloadHash, originalManifestLabel, originalManifestHash[:], originalSignatureHash[:], hardBindingHash, exclusions)
	if err != nil {
		return nil, err
	}
	children := make([][]byte, 0, len(manifestBoxes)+1)
	children = append(children, manifestBoxes...)
	children = append(children, updateManifestBox)
	return renderJUMBFSuperbox(c2paStoreUUID, 0x03, "c2pa", children), nil
}

func renderVerificationUpdateManifest(ctx context.Context, manifestLabel string, payloadHash string, parentManifestLabel string, parentManifestHash []byte, _ []byte, hardBindingHash []byte, exclusions []c2paExclusion) ([]byte, error) {
	updateLabel := manifestLabel
	hardBindingLabel := "c2pa.hash.data"
	hardBindingPayload := renderMinimalDataHashAssertionCBOR(hardBindingHash, exclusions)
	hardBindingBox := renderJUMBFSuperbox(cborUUID, 0x03, hardBindingLabel, [][]byte{renderBMFFBox("cbor", hardBindingPayload)})
	hardBindingAssertionHash := sha256.Sum256(hardBindingBox[8:])
	hardBindingURI := absoluteAssertionURI(updateLabel, hardBindingLabel)

	ingredientLabel := "c2pa.ingredient.v2"
	ingredientPayload := renderMinimalIngredientAssertionCBOR(payloadHash, parentManifestLabel, parentManifestHash)
	ingredientBox := renderJUMBFSuperbox(cborUUID, 0x03, ingredientLabel, [][]byte{renderBMFFBox("cbor", ingredientPayload)})
	ingredientHash := sha256.Sum256(ingredientBox[8:])
	ingredientURI := absoluteAssertionURI(updateLabel, ingredientLabel)

	actionsPayload := renderVerificationActionsAssertionCBOR(ingredientURI, ingredientHash[:])
	actionsBox := renderJUMBFSuperbox(cborUUID, 0x03, "c2pa.actions.v2", [][]byte{renderBMFFBox("cbor", actionsPayload)})
	actionsHash := sha256.Sum256(actionsBox[8:])
	actionsURI := absoluteAssertionURI(updateLabel, "c2pa.actions.v2")

	assertionStore := renderJUMBFSuperbox(c2paAsrtUUID, 0x03, "c2pa.assertions", [][]byte{hardBindingBox, ingredientBox, actionsBox})
	claimPayload := renderVerificationClaimCBOR(payloadHash, updateLabel, hardBindingURI, hardBindingAssertionHash[:], ingredientURI, ingredientHash[:], actionsURI, actionsHash[:])
	claimBox := renderJUMBFSuperbox(c2paClmUUID, 0x03, "c2pa.claim.v2", [][]byte{renderBMFFBox("cbor", claimPayload)})

	protected := buildCoseProtectedHeadersWithX5Chain()
	sig, err := signClaimSigStructure(ctx, protected, claimPayload)
	if err != nil {
		return nil, err
	}
	signaturePayload := []byte{0xD2, 0x84}
	signaturePayload = append(signaturePayload, cborBytes(protected)...)
	signaturePayload = append(signaturePayload, 0xA0, 0xF6)
	signaturePayload = append(signaturePayload, cborBytes(sig)...)
	signatureBox := renderJUMBFSuperbox(c2paSigUUID, 0x03, "c2pa.signature", [][]byte{renderBMFFBox("cbor", signaturePayload)})
	return renderJUMBFSuperbox(c2paUpdateUUID, 0x03, updateLabel, [][]byte{assertionStore, claimBox, signatureBox}), nil
}

func renderJUMBFSuperbox(uuidHex string, toggles byte, label string, children [][]byte) []byte {
	desc := renderJUMBFDescriptionBox(uuidHex, toggles, label)
	payload := make([]byte, 0, len(desc)+128)
	payload = append(payload, desc...)
	for _, child := range children {
		payload = append(payload, child...)
	}
	return renderBMFFBox("jumb", payload)
}

func renderBMFFBox(boxType string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[0:4], uint32(len(box)))
	copy(box[4:8], []byte(boxType))
	copy(box[8:], payload)
	return box
}

func renderUUIDContentBox(uuidHex string, payload []byte) []byte {
	uuid := decodeUUIDHex(uuidHex)
	boxPayload := make([]byte, 0, 16+len(payload))
	boxPayload = append(boxPayload, uuid[:]...)
	boxPayload = append(boxPayload, payload...)
	return renderBMFFBox("uuid", boxPayload)
}

func renderJUMBFDescriptionBox(uuidHex string, toggles byte, label string) []byte {
	uuid := decodeUUIDHex(uuidHex)
	payload := make([]byte, 0, 16+1+len(label)+1)
	payload = append(payload, uuid[:]...)
	payload = append(payload, toggles)
	if toggles&0x03 == 0x03 {
		payload = append(payload, []byte(label)...)
		payload = append(payload, 0x00)
	}
	return renderBMFFBox("jumd", payload)
}

func cborHead(major byte, n int) []byte {
	if n < 0 {
		n = 0
	}
	if n <= 23 {
		return []byte{byte((major << 5) | byte(n))}
	}
	if n <= 0xFF {
		return []byte{byte((major << 5) | 24), byte(n)}
	}
	if n <= 0xFFFF {
		return []byte{byte((major << 5) | 25), byte(n >> 8), byte(n)}
	}
	if n <= 0xFFFFFFFF {
		return []byte{byte((major << 5) | 26), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
	u := uint64(n)
	return []byte{byte((major << 5) | 27), byte(u >> 56), byte(u >> 48), byte(u >> 40), byte(u >> 32), byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)}
}

func cborText(s string) []byte {
	h := cborHead(3, len(s))
	return append(h, []byte(s)...)
}

func cborBytes(b []byte) []byte {
	h := cborHead(2, len(b))
	return append(h, b...)
}

func cborArray(items ...[]byte) []byte {
	out := cborHead(4, len(items))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

func cborMap(pairs ...[]byte) []byte {
	out := cborHead(5, len(pairs)/2)
	for _, p := range pairs {
		out = append(out, p...)
	}
	return out
}

func cborUint(n int) []byte {
	return cborHead(0, n)
}

func cborNegInt(v int) []byte {
	// CBOR negative integer encoding stores -(n+1).
	return cborHead(1, -1-v)
}

func renderMinimalActionsAssertionCBOR() []byte {
	action := cborMap(
		cborText("action"), cborText("c2pa.created"),
		cborText("softwareAgent"), cborMap(
			cborText("name"), cborText("DCS-PDF-CORE"),
			cborText("version"), cborText("1.0"),
		),
	)
	return cborMap(
		cborText("actions"), cborArray(action),
		cborText("allActionsIncluded"), []byte{0xF5},
	)
}

func renderMinimalDataHashAssertionCBOR(hashBytes []byte, exclusions []c2paExclusion) []byte {
	exclusionItems := make([][]byte, 0, len(exclusions))
	for _, exclusion := range exclusions {
		if exclusion.Length <= 0 {
			continue
		}
		exclusionItems = append(exclusionItems, cborMap(
			cborText("start"), cborUint(exclusion.Start),
			cborText("length"), cborUint(exclusion.Length),
		))
	}

	pairs := [][]byte{
		cborText("alg"), cborText("sha256"),
		cborText("name"), cborText("pdf"),
		cborText("exclusions"), cborArray(exclusionItems...),
		cborText("hash"), cborBytes(hashBytes),
		cborText("pad"), cborBytes([]byte{0x00}),
	}
	return cborMap(pairs...)
}

func renderMinimalClaimCBOR(payloadHash string, hardBindingHash []byte, hardBindingURL string, actionsHash []byte, actionsURL string) []byte {
	instanceID := "xmp:iid:" + uuidFromHashPrefix(payloadHash)
	hardBindingHashedURI := cborMap(
		cborText("url"), cborText(hardBindingURL),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(hardBindingHash),
	)
	actionsHashedURI := cborMap(
		cborText("url"), cborText(actionsURL),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(actionsHash),
	)
	claimGenInfo := cborMap(
		cborText("name"), cborText("DCS-PDF-CORE"),
		cborText("version"), cborText("1.0"),
	)
	return cborMap(
		cborText("instanceID"), cborText(instanceID),
		cborText("claim_generator_info"), claimGenInfo,
		cborText("alg"), cborText("sha256"),
		cborText("signature"), cborText("self#jumbf=c2pa.signature"),
		cborText("created_assertions"), cborArray(hardBindingHashedURI, actionsHashedURI),
	)
}

func renderMinimalIngredientAssertionCBOR(payloadHash string, parentManifestLabel string, parentManifestHash []byte) []byte {
	instanceID := "xmp:iid:" + uuidFromHashPrefix(payloadHash)
	return cborMap(
		cborText("dc:title"), cborText("DCS-PDF-CORE verified source"),
		cborText("dc:format"), cborText("application/pdf"),
		cborText("instanceID"), cborText(instanceID),
		cborText("relationship"), cborText("parentOf"),
		cborText("c2pa_manifest"), cborMap(
			cborText("url"), cborText(absoluteManifestURI(parentManifestLabel)),
			cborText("alg"), cborText("sha256"),
			cborText("hash"), cborBytes(parentManifestHash),
		),
	)
}

func renderVerificationActionsAssertionCBOR(ingredientURI string, ingredientHash []byte) []byte {
	ingredientRef := cborMap(
		cborText("url"), cborText(ingredientURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(ingredientHash),
	)
	opened := cborMap(
		cborText("action"), cborText("c2pa.opened"),
		cborText("parameters"), cborMap(
			cborText("ingredients"), cborArray(ingredientRef),
		),
		cborText("softwareAgent"), cborMap(
			cborText("name"), cborText("DCS-PDF-CORE"),
			cborText("version"), cborText("1.0"),
		),
	)
	return cborMap(
		cborText("actions"), cborArray(opened),
		cborText("allActionsIncluded"), []byte{0xF5},
	)
}

func renderVerificationClaimCBOR(payloadHash string, manifestLabel string, hardBindingURI string, hardBindingHash []byte, ingredientURI string, ingredientHash []byte, actionsURI string, actionsHash []byte) []byte {
	instanceID := "xmp:iid:" + uuidFromHashPrefix(payloadHash)
	hardBindingRef := cborMap(
		cborText("url"), cborText(hardBindingURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(hardBindingHash),
	)
	ingredientRef := cborMap(
		cborText("url"), cborText(ingredientURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(ingredientHash),
	)
	actionsRef := cborMap(
		cborText("url"), cborText(actionsURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(actionsHash),
	)
	claimGenInfo := cborMap(
		cborText("name"), cborText("DCS-PDF-CORE"),
		cborText("version"), cborText("1.0"),
	)
	return cborMap(
		cborText("instanceID"), cborText(instanceID),
		cborText("claim_generator_info"), claimGenInfo,
		cborText("alg"), cborText("sha256"),
		cborText("signature"), cborText(absoluteSignatureURI(manifestLabel)),
		cborText("created_assertions"), cborArray(hardBindingRef, ingredientRef, actionsRef),
	)
}

func buildCoseProtectedHeadersWithX5Chain() []byte {
	material := mustSigningMaterial()
	chainItems := make([][]byte, 0, len(material.certChainDER))
	for _, certDER := range material.certChainDER {
		chainItems = append(chainItems, cborBytes(certDER))
	}

	headers := cborMap(
		cborUint(1), cborNegInt(-8),
		cborUint(33), cborArray(chainItems...),
	)
	return headers
}

func signClaimSigStructure(ctx context.Context, protected []byte, claimPayload []byte) ([]byte, error) {
	signer := mustSigningMaterial().signer
	sigStructure := cborArray(
		cborText("Signature1"),
		cborBytes(protected),
		cborBytes([]byte{}),
		cborBytes(claimPayload),
	)
	return signer.Sign(ctx, sigStructure)
}

func mustSigningMaterial() signingMaterial {
	signingMaterialOnce.Do(func() {
		signingMaterialCached, signingMaterialErr = loadSigningMaterialFromEnv(os.Getenv, os.ReadFile)
	})
	if signingMaterialErr != nil {
		panic(fmt.Sprintf("c2pa signing material configuration error: %v", signingMaterialErr))
	}
	return signingMaterialCached
}

func loadSigningMaterialFromEnv(getenv func(string) string, readFile func(string) ([]byte, error)) (signingMaterial, error) {
	cryptoURL := strings.TrimSpace(getenv(envCryptoProviderURL))
	if cryptoURL != "" {
		ns := strings.TrimSpace(getenv(envCryptoProviderNamespace))
		key := strings.TrimSpace(getenv(envCryptoProviderKey))
		if ns == "" {
			return signingMaterial{}, fmt.Errorf("%s is required when %s is set", envCryptoProviderNamespace, envCryptoProviderURL)
		}
		if key == "" {
			return signingMaterial{}, fmt.Errorf("%s is required when %s is set", envCryptoProviderKey, envCryptoProviderURL)
		}
		chainInline := strings.TrimSpace(getenv(envX5ChainPEM))
		chainFile := strings.TrimSpace(getenv(envX5ChainPEMFile))
		chainPEM, chainProvided, err := resolveSigningConfigValue(readFile, chainInline, chainFile, envX5ChainPEM, envX5ChainPEMFile)
		if err != nil {
			return signingMaterial{}, err
		}
		if !chainProvided {
			return signingMaterial{}, fmt.Errorf("x5chain must be provided when using transit signer; set %s or %s", envX5ChainPEM, envX5ChainPEMFile)
		}
		certs, err := parseCertificateChainPEM([]byte(chainPEM))
		if err != nil {
			return signingMaterial{}, err
		}
		return signingMaterial{signer: newTransitSigner(cryptoURL, ns, key), certChainDER: certs}, nil
	}

	keyInline := strings.TrimSpace(getenv(envSignerKeyPEM))
	keyFile := strings.TrimSpace(getenv(envSignerKeyPEMFile))
	keyPEM, keyProvided, err := resolveSigningConfigValue(
		readFile,
		keyInline,
		keyFile,
		envSignerKeyPEM,
		envSignerKeyPEMFile,
	)
	if err != nil {
		return signingMaterial{}, err
	}
	chainInline := strings.TrimSpace(getenv(envX5ChainPEM))
	chainFile := strings.TrimSpace(getenv(envX5ChainPEMFile))
	chainPEM, chainProvided, err := resolveSigningConfigValue(
		readFile,
		chainInline,
		chainFile,
		envX5ChainPEM,
		envX5ChainPEMFile,
	)
	if err != nil {
		return signingMaterial{}, err
	}

	requireExternal := strings.EqualFold(strings.TrimSpace(getenv(envRequireExternalSigningMaterial)), "true")
	if !keyProvided && !chainProvided {
		if requireExternal {
			return signingMaterial{}, fmt.Errorf("external signing material required but not provided")
		}
		return signingMaterial{}, fmt.Errorf("signing material not configured; set %s or %s and %s or %s", envSignerKeyPEM, envSignerKeyPEMFile, envX5ChainPEM, envX5ChainPEMFile)
	}
	if !keyProvided || !chainProvided {
		return signingMaterial{}, fmt.Errorf("both signing key and x5chain must be provided together")
	}

	priv, err := parsePKCS8Ed25519PrivateKeyPEM([]byte(keyPEM))
	if err != nil {
		return signingMaterial{}, err
	}
	certs, err := parseCertificateChainPEM([]byte(chainPEM))
	if err != nil {
		return signingMaterial{}, err
	}
	return signingMaterial{signer: localSigner{priv: priv}, certChainDER: certs}, nil
}

func resolveSigningConfigValue(readFile func(string) ([]byte, error), inlineValue string, filePath string, inlineName string, fileName string) (string, bool, error) {
	if inlineValue != "" && filePath != "" {
		return "", false, fmt.Errorf("%s and %s are mutually exclusive", inlineName, fileName)
	}
	if inlineValue != "" {
		return inlineValue, true, nil
	}
	if filePath == "" {
		return "", false, nil
	}
	content, err := readFile(filePath)
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", filePath, err)
	}
	return string(content), true, nil
}

func parsePKCS8Ed25519PrivateKeyPEM(pemBytes []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("signer key PEM block not found")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse signer PKCS8 key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("signer key is not Ed25519")
	}
	return priv, nil
}

func parseCertificateChainPEM(pemBytes []byte) ([][]byte, error) {
	rest := pemBytes
	var certs [][]byte
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		certs = append(certs, append([]byte(nil), block.Bytes...))
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("x5chain PEM does not include any CERTIFICATE blocks")
	}
	return certs, nil
}

func urnUUIDFromHash(payloadHash string) string {
	if len(payloadHash) < 32 {
		return "urn:c2pa:00000000-0000-0000-0000-000000000000"
	}
	h := strings.ToUpper(payloadHash[:32])
	return fmt.Sprintf("urn:c2pa:%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func uuidFromHashPrefix(payloadHash string) string {
	h := strings.ToUpper(payloadHash)
	if len(h) < 32 {
		h += strings.Repeat("0", 32-len(h))
	}
	h = h[:32]
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func payloadHashBytes(payloadHash string) []byte {
	if len(payloadHash) < 64 {
		return make([]byte, 32)
	}
	decoded, err := hex.DecodeString(payloadHash[:64])
	if err != nil || len(decoded) != 32 {
		return make([]byte, 32)
	}
	return decoded
}

func decodeUUIDHex(uuidHex string) [16]byte {
	var uuid [16]byte
	decoded, err := hex.DecodeString(uuidHex)
	if err != nil || len(decoded) != 16 {
		return uuid
	}
	copy(uuid[:], decoded)
	return uuid
}

func updateManifestLabelFromHash(payloadHash string) string {
	return urnUUIDFromHash(payloadHash) + ":dcs-pdf-core:2_1"
}

// witnessManifestLabel returns a manifest label for the verification witness
// derived from the hard binding hash (SHA-256 of the file bytes at witness
// time).  Using the hard binding hash instead of the payload hash guarantees
// the label is distinct from the update manifest that has already been written
// for the same payload — avoiding a cyclic-ingredient report from c2patool.
func witnessManifestLabel(hardBindingHash []byte) string {
	return urnUUIDFromHash(hex.EncodeToString(hardBindingHash)) + ":dcs-pdf-core:witness_1"
}

func absoluteManifestURI(manifestLabel string) string {
	return "self#jumbf=/c2pa/" + manifestLabel
}

func absoluteAssertionURI(manifestLabel string, assertionLabel string) string {
	return absoluteManifestURI(manifestLabel) + "/c2pa.assertions/" + assertionLabel
}

func absoluteSignatureURI(manifestLabel string) string {
	return absoluteManifestURI(manifestLabel) + "/c2pa.signature"
}
