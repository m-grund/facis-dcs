package handler

// Options configures status-list mechanism handlers for DCS.
type Options struct {
	// XFSCAllowUnsignedFallback enables unsigned application/json fallback when
	// signed statuslist+jwt retrieval or verification fails inside the XFSC handler.
	XFSCAllowUnsignedFallback bool
}
