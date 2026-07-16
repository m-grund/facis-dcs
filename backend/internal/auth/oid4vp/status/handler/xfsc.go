package handler

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/codec"
	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"

	"github.com/golang-jwt/jwt/v5"
)

// XFSC verifies status lists from eclipse-xfsc/statuslist-service.
//
// Unsigned lists are served as application/json ({tenantId, listId, list}).
// Signed lists are fetched with Content-Type: statuslist+jwt only (no Accept header).
// When AllowUnsignedFallback is true, a failed signed fetch or verification falls back
// to the unsigned JSON envelope (dev/BDD without crypto-provider signer).
type XFSC struct {
	Fetcher               *fetch.Client
	Trust                 *status.TrustConfig
	AllowUnsignedFallback bool
}

func (h *XFSC) Mechanism() status.Mechanism {
	return status.MechanismXFSC
}

func (h *XFSC) Check(
	ctx context.Context,
	_ status.VerifiedCredential,
	ref status.Reference,
) (status.Result, error) {
	client := h.Fetcher
	if client == nil {
		client = fetch.NewClient()
	}

	signedResp, signedErr := status.FetchStatusList(ctx, client, ref.URI, fetch.RequestOpts{
		ContentType: status.XFSCSignedContentType,
	})
	if signedErr == nil {
		verified, err := h.verifyStatusListJWT(UnwrapStatusListJWTBody(signedResp.Body))
		if err == nil {
			return h.resultFromSignedClaims(ref, verified.Claims)
		}
		if !h.AllowUnsignedFallback {
			return status.Result{}, status.ErrStatusSignature
		}
	} else if !h.AllowUnsignedFallback {
		return status.Result{}, status.ErrStatusRetrieval
	}

	if !h.AllowUnsignedFallback {
		return status.Result{}, status.ErrStatusSignature
	}

	if ref.Prefetched != nil && status.IsXFSCStatusListJSON(ref.Prefetched.Body) {
		return h.resultFromUnsignedJSON(ref, ref.Prefetched.Body)
	}

	unsignedResp, err := status.FetchStatusList(ctx, client, ref.URI, fetch.RequestOpts{
		ContentType: status.XFSCProbeContentType,
	})
	if err != nil {
		return status.Result{}, status.ErrStatusRetrieval
	}
	if !status.IsXFSCStatusListJSON(unsignedResp.Body) {
		return status.Result{}, status.ErrStatusListNotSecured
	}

	return h.resultFromUnsignedJSON(ref, unsignedResp.Body)
}

func (h *XFSC) resultFromSignedClaims(ref status.Reference, claims map[string]any) (status.Result, error) {
	subject, _ := claims["sub"].(string)
	if !status.SubjectMatchesURI(subject, ref.URI) {
		log.Printf(
			"xfsc status list: jwt sub %q does not match credential uri %q (known XFSC statuslist-service issue)",
			strings.TrimSpace(subject),
			strings.TrimSpace(ref.URI),
		)
	}

	bitstring, bits, err := decodeXFSCSignedClaims(claims)
	if err != nil {
		return status.Result{}, err
	}

	return h.mapBitstringResult(ref, bitstring, bits)
}

func (h *XFSC) resultFromUnsignedJSON(ref status.Reference, body []byte) (status.Result, error) {
	bitstring, bits, err := decodeXFSCUnsignedJSON(body)
	if err != nil {
		return status.Result{}, err
	}
	return h.mapBitstringResult(ref, bitstring, bits)
}

func (h *XFSC) mapBitstringResult(ref status.Reference, bitstring []byte, bits uint) (status.Result, error) {
	width := ref.StatusSize
	if width == 0 {
		width = bits
	}
	if width == 0 {
		width = 1
	}

	value, err := codec.ReadStatusValue(bitstring, ref.Index, width, codec.LSBFirst)
	if err != nil {
		return status.Result{}, err
	}

	return status.MapIETFResult(ref, value), nil
}

// UnwrapStatusListJWTBody strips a JSON-quoted JWT string returned by XFSC statuslist-service.
func UnwrapStatusListJWTBody(body []byte) []byte {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, `"`) {
		var quoted string
		if err := json.Unmarshal(body, &quoted); err == nil {
			quoted = strings.TrimSpace(quoted)
			if status.IsLikelyJWT([]byte(quoted)) {
				return []byte(quoted)
			}
		}
	}
	if status.IsLikelyJWT([]byte(trimmed)) {
		return []byte(trimmed)
	}
	return body
}

func (h *XFSC) verifyStatusListJWT(body []byte) (envelope.VerifiedJWT, error) {
	if !status.IsLikelyJWT(body) {
		return envelope.VerifiedJWT{}, fmt.Errorf("xfsc status list response is not a jwt")
	}
	if h.Trust == nil {
		return envelope.VerifiedJWT{}, status.ErrStatusTrustNotConfigured
	}
	verified, err := envelope.VerifyES256JWT(body, func(issuer string, _ *jwt.Token) (*ecdsa.PublicKey, error) {
		return h.Trust.ResolveECDSAPublicKey(issuer)
	})
	if err != nil {
		return envelope.VerifiedJWT{}, err
	}
	if !status.IsXFSCStatusListJWTType(verified.Header) {
		return envelope.VerifiedJWT{}, fmt.Errorf("xfsc jwt typ must be statuslist+jwt or JWT")
	}
	return verified, nil
}

func decodeXFSCSignedClaims(claims map[string]any) ([]byte, uint, error) {
	statusList, ok := claims["status_list"].(map[string]any)
	if !ok {
		return nil, 0, status.ErrStatusDecoding
	}

	bits, ok := status.ParseTokenStatusBits(statusList["bits"])
	if !ok {
		bits = 1
	}

	lst, _ := statusList["lst"].(string)
	if strings.TrimSpace(lst) == "" {
		return nil, 0, status.ErrStatusDecoding
	}

	compressed, err := codec.DecodeBase64Flexible(lst)
	if err != nil {
		return nil, 0, status.ErrStatusDecoding
	}

	bitstring, err := codec.GZIPDecompressLimited(compressed, 0)
	if err != nil {
		return nil, 0, status.ErrStatusDecompression
	}
	return bitstring, bits, nil
}

func decodeXFSCUnsignedJSON(body []byte) ([]byte, uint, error) {
	var doc struct {
		List string `json:"list"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, 0, status.ErrStatusDecoding
	}
	if strings.TrimSpace(doc.List) == "" {
		return nil, 0, status.ErrStatusDecoding
	}

	compressed, err := codec.DecodeBase64Flexible(doc.List)
	if err != nil {
		return nil, 0, status.ErrStatusDecoding
	}

	bitstring, err := codec.GZIPDecompressLimited(compressed, 0)
	if err != nil {
		return nil, 0, status.ErrStatusDecompression
	}
	return bitstring, 1, nil
}
