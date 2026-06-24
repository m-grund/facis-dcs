package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

var allowedSubjects = []string{
	"dcs.contract_workflow_engine",
	"dcs.contract_storage_archive",
	"dcs.signature_management",
}

func main() {
	logger := log.New(os.Stdout, "[relay] ", log.LstdFlags|log.Lmicroseconds)

	var forwarded, failed atomic.Int64

	srcURL := "nats://localhost:30422"
	dstURL := "nats://localhost:30122"

	logger.Printf("connecting to source: %s", srcURL)
	src, err := nats.Connect(srcURL,
		nats.Name("relay-src"),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Printf("source disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Printf("source reconnected: %s", c.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			logger.Printf("source connection closed")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			logger.Printf("source async error (sub=%v): %v", sub, err)
		}),
	)
	if err != nil {
		logger.Fatalf("could not connect to source %s: %v", srcURL, err)
	}
	defer src.Close()
	logger.Printf("connected to source: %s", src.ConnectedUrl())

	logger.Printf("connecting to destination: %s", dstURL)
	dst, err := nats.Connect(dstURL,
		nats.Name("relay-dst"),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			logger.Printf("destination disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			logger.Printf("destination reconnected: %s", c.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			logger.Printf("destination connection closed")
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			logger.Printf("destination async error (sub=%v): %v", sub, err)
		}),
	)
	if err != nil {
		logger.Fatalf("could not connect to destination %s: %v", dstURL, err)
	}
	defer dst.Close()
	logger.Printf("connected to destination: %s", dst.ConnectedUrl())

	handler := func(msg *nats.Msg) {
		start := time.Now()
		logger.Printf("received message on %q (%d bytes)", msg.Subject, len(msg.Data))

		logger.Printf(fmt.Sprintf("Publish to subject %s: %s", msg.Subject, string(msg.Data)))
		if err := dst.Publish(msg.Subject, msg.Data); err != nil {
			failed.Add(1)
			logger.Printf("forward FAILED for %q: %v (total failed: %d)", msg.Subject, err, failed.Load())
			return
		}

		forwarded.Add(1)
		logger.Printf("forwarded message on %q in %s (total forwarded: %d)",
			msg.Subject, time.Since(start), forwarded.Load())
	}

	var subs []*nats.Subscription
	for _, subj := range allowedSubjects {
		logger.Printf("subscribing to subject %q on source", subj)
		sub, err := src.Subscribe(subj, handler)
		if err != nil {
			logger.Fatalf("could not subscribe to %q: %v", subj, err)
		}
		defer sub.Unsubscribe()
		subs = append(subs, sub)
	}
	logger.Printf("relay running: %s -> %s for %d subject(s): %v", srcURL, dstURL, len(subs), allowedSubjects)

	// Periodic stats, damit man auch ohne Traffic sieht, dass der Relay lebt.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			logger.Printf("status: forwarded=%d failed=%d src_connected=%v dst_connected=%v",
				forwarded.Load(), failed.Load(), src.IsConnected(), dst.IsConnected())
		case sig := <-sigCh:
			logger.Printf("received signal %v, shutting down", sig)
			logger.Printf("final stats: forwarded=%d failed=%d", forwarded.Load(), failed.Load())
			return
		}
	}
}
