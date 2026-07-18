// Package conf centralizes timing/topic configuration shared across domains
// (transaction timeouts, outbox/cron polling intervals, the event-bus topic
// name), so these values are changed in one place rather than duplicated
// per domain.
package conf

import (
	"os"
	"time"
)

func TransactionTimeout() time.Duration {
	return 1 * time.Minute
}

// SystemToken is the in-cluster service credential the background PDF
// regenerator presents to the internal signing primitives (it runs on NATS
// events with no user JWT). Empty when unset — no system caller is accepted.
func SystemToken() string {
	return os.Getenv("DCS_SYSTEM_TOKEN")
}

// SignerBackend selects how contract PAdES signatures are produced:
// "pdfcore" (default) uses pdf-core's in-process PKCS#11 path; "dss" routes
// through a remote EU DSS via the CSC/rQES flow (DCS-IR-SI-10), the
// production switch to a wallet-unlocked QTSP.
func SignerBackend() string {
	if b := os.Getenv("DCS_SIGNER_BACKEND"); b != "" {
		return b
	}
	return "pdfcore"
}

// DSSURL is the base URL of the EU DSS REST service used when SignerBackend is
// "dss" (and for signature validation). Empty when the DSS is not deployed.
func DSSURL() string {
	return os.Getenv("DCS_DSS_URL")
}

// PAdESX5ChainPEM is the PAdES signer's certificate chain (leaf first) in PEM,
// the same chain pdf-core embeds; the DSS backend needs it to name the signing
// certificate in the rQES parameters. Prefers DCS_PADES_X5CHAIN_PEM (inline
// PEM); falls back to reading DCS_PADES_X5CHAIN_PEM_FILE (a mounted path,
// mirroring pdf-core's DCS_PDF_CORE_PADES_X5CHAIN_PEM_FILE convention).
func PAdESX5ChainPEM() string {
	if pem := os.Getenv("DCS_PADES_X5CHAIN_PEM"); pem != "" {
		return pem
	}
	if path := os.Getenv("DCS_PADES_X5CHAIN_PEM_FILE"); path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return string(b)
		}
	}
	return ""
}

func HTTPClientTimeout() time.Duration {
	return 1 * time.Minute
}

func OutboxProcessorTimeOut() time.Duration {
	return 1 * time.Second
}

// OutboxPublishTimeOut is the poll interval for republishing outbox events
// on NATS (see event.OutboxProcessor.startPublishingJob): much tighter than
// OutboxProcessorTimeOut because publishing is a cheap, single NATS call per
// event, unlike the TSA/IPFS round-trips the (slower) anchoring loop does.
func OutboxPublishTimeOut() time.Duration {
	return 100 * time.Millisecond
}

func EventBusTopic() string {
	return "dcs"
}

func GlobalAuditTrailName() string {
	return "GLOBAL_AUDIT_TRAIL"
}

func LoginAttemptsThresholdInDuration() int {
	return 5
}

func LoginLockoutDuration() time.Duration {
	return 15 * time.Minute
}

func SyncFailCronJobTimeOut() time.Duration {
	return 24 * time.Hour
}
