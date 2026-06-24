package main

import (
	"cloudevent-dev-relay/event"
	"log"
	"os"
	"os/signal"
	"slices"
	"sync/atomic"
	"syscall"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
)

var allowedSubjects = []string{
	"dcs.contract_workflow_engine",
	"dcs.contract_storage_archive",
	"dcs.signature_management",
}

var filteredEvents = []string{
	"RETRIEVE_ALL_CONTRACTS",
	"RETRIEVE_CONTRACT_BY_ID",
}

const (
	srcURL = "nats://localhost:30422"
	dstURL = "nats://localhost:30122"
)

type relayPair struct {
	subject string
	sub     *event.CloudEventSubClient
	pub     *event.CloudEventPubClient
}

func main() {
	logger := log.New(os.Stdout, "[relay] ", log.LstdFlags|log.Lmicroseconds)

	var forwarded, failed atomic.Int64
	var pairs []relayPair

	for _, subj := range allowedSubjects {
		logger.Printf("creating pub client for %q -> %s", subj, dstURL)
		pubClient, err := event.NewNatsPubClient(subj, dstURL)
		if err != nil {
			logger.Fatalf("could not create pub client for %q: %v", subj, err)
		}

		logger.Printf("creating sub client for %q <- %s", subj, srcURL)
		subClient, err := event.NewNatsSubClient(subj, srcURL)
		if err != nil {
			logger.Fatalf("could not create sub client for %q: %v", subj, err)
		}

		pairs = append(pairs, relayPair{subject: subj, sub: subClient, pub: pubClient})
	}

	for _, p := range pairs {
		subject := p.subject
		pubClient := p.pub

		err := p.sub.Subscribe(func(evt cloudevent.Event) {
			start := time.Now()

			if slices.Contains(filteredEvents, evt.Type()) {
				return
			}

			logger.Printf("received event on %q: id=%s type=%s source=%s (%d bytes)",
				subject, evt.ID(), evt.Type(), evt.Source(), len(evt.Data()))

			if err := pubClient.Publish(evt.Source(), evt.Type(), evt.Data()); err != nil {
				failed.Add(1)
				logger.Printf("forward FAILED for %q (orig id=%s): %v (total failed: %d)",
					subject, evt.ID(), err, failed.Load())
				return
			}

			forwarded.Add(1)
			logger.Printf("forwarded event on %q (orig id=%s) in %s (total forwarded: %d)",
				subject, evt.ID(), time.Since(start), forwarded.Load())
		})
		if err != nil {
			logger.Fatalf("could not subscribe to %q: %v", subject, err)
		}
	}

	logger.Printf("relay running: %s -> %s for %d subject(s): %v", srcURL, dstURL, len(pairs), allowedSubjects)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			logger.Printf("status: forwarded=%d failed=%d", forwarded.Load(), failed.Load())
		case sig := <-sigCh:
			logger.Printf("received signal %v, shutting down", sig)
			for _, p := range pairs {
				if err := p.sub.Close(); err != nil {
					logger.Printf("error closing sub client for %q: %v", p.subject, err)
				}
				if err := p.pub.Close(); err != nil {
					logger.Printf("error closing pub client for %q: %v", p.subject, err)
				}
			}
			logger.Printf("final stats: forwarded=%d failed=%d", forwarded.Load(), failed.Load())
			return
		}
	}
}
