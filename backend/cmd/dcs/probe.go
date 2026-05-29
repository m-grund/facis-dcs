package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// probeHTTP performs a single GET to the given URL and returns an error if the
// response is not 2xx. Used at startup to fail fast when a required dependency
// is down.
func probeHTTP(rawURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// probeHTTPAny tries multiple URLs and returns nil on first success.
func probeHTTPAny(urls ...string) error {
	if len(urls) == 0 {
		return fmt.Errorf("no URLs provided")
	}
	var lastErr error
	for _, rawURL := range urls {
		if err := probeHTTP(rawURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return lastErr
}

// probeTCP dials the host:port extracted from rawURL and returns an error if
// the TCP connection cannot be established within 5 seconds. Use this for
// services that don't expose a documented HTTP health endpoint.
func probeTCP(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
