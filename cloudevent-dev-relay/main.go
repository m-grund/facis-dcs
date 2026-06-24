package main

import (
	"cloudevent-dev-relay/event"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
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

	// Jede Seite published mit einer eigenen, unterscheidbaren source-DID.
	// Das nutzen wir zur Loop-Prevention: eine Richtung leitet nur Events
	// weiter, die *wirklich* von der ihr zugeordneten Seite stammen - ein
	// bereits durchgereichtes Event behält die source der Gegenseite und
	// wird dadurch automatisch nicht zurückgespiegelt.
	srcOriginPrefix = "did:web:localhost%3A8991"
	dstOriginPrefix = "did:web:localhost%3A8992"
)

// direction beschreibt eine Subscribe->Publish-Verbindung für ein Subject
// in genau eine Richtung (z.B. src->dst oder dst->src).
type direction struct {
	label        string
	subject      string
	sub          *event.CloudEventSubClient
	pub          *event.CloudEventPubClient
	originPrefix string // nur Events mit dieser source werden weitergeleitet
}

func setupDirection(logger *log.Logger, label, subject, fromURL, toURL, originPrefix string) direction {
	logger.Printf("[%s] creating sub client for %q <- %s", label, subject, fromURL)
	subClient, err := event.NewNatsSubClient(subject, fromURL)
	if err != nil {
		logger.Fatalf("[%s] could not create sub client for %q: %v", label, subject, err)
	}

	logger.Printf("[%s] creating pub client for %q -> %s", label, subject, toURL)
	pubClient, err := event.NewNatsPubClient(subject, toURL)
	if err != nil {
		logger.Fatalf("[%s] could not create pub client for %q: %v", label, subject, err)
	}

	return direction{label: label, subject: subject, sub: subClient, pub: pubClient, originPrefix: originPrefix}
}

func main() {
	logger := log.New(os.Stdout, "[relay] ", log.LstdFlags|log.Lmicroseconds)

	var forwarded, failed, loopPrevented atomic.Int64
	var directions []direction

	for _, subj := range allowedSubjects {
		directions = append(directions, setupDirection(logger, "src->dst", subj, srcURL, dstURL, srcOriginPrefix))
		directions = append(directions, setupDirection(logger, "dst->src", subj, dstURL, srcURL, dstOriginPrefix))
	}

	for _, d := range directions {
		dir := d // lokale Kopie für die Closure

		// Subscribe() blockiert intern (cloudevents-go SDK StartReceiver ist
		// ein blocking call) - ohne eigene Goroutine würde bereits der erste
		// Subscribe-Aufruf alle weiteren verhindern.
		go func() {
			err := dir.sub.Subscribe(func(evt cloudevent.Event) {
				start := time.Now()

				if slices.Contains(filteredEvents, evt.Type()) {
					return
				}

				if !strings.HasPrefix(evt.Source(), dir.originPrefix) {
					loopPrevented.Add(1)
					logger.Printf("[%s] skipping event on %q (id=%s, source=%s) - not from expected origin %q",
						dir.label, dir.subject, evt.ID(), evt.Source(), dir.originPrefix)
					return
				}

				logger.Printf("[%s] received event on %q: id=%s type=%s source=%s (%d bytes)",
					dir.label, dir.subject, evt.ID(), evt.Type(), evt.Source(), len(evt.Data()))

				if err := dir.pub.PublishEvent(evt); err != nil {
					failed.Add(1)
					logger.Printf("[%s] forward FAILED for %q (id=%s): %v (total failed: %d)",
						dir.label, dir.subject, evt.ID(), err, failed.Load())
					return
				}

				forwarded.Add(1)
				logger.Printf("[%s] forwarded event on %q (id=%s) in %s (total forwarded: %d)",
					dir.label, dir.subject, evt.ID(), time.Since(start), forwarded.Load())
			})
			if err != nil {
				logger.Fatalf("[%s] could not subscribe to %q: %v", dir.label, dir.subject, err)
			}
		}()
	}

	// Kurze Pause, damit alle Subscribe-Goroutinen tatsächlich angelaufen sind,
	// bevor die "relay running"-Zeile geloggt wird (rein kosmetisch).
	time.Sleep(200 * time.Millisecond)

	logger.Printf("relay running bidirectionally: %s <-> %s for %d subject(s): %v",
		srcURL, dstURL, len(allowedSubjects), allowedSubjects)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			logger.Printf("status: forwarded=%d failed=%d loop-prevented=%d",
				forwarded.Load(), failed.Load(), loopPrevented.Load())
		case sig := <-sigCh:
			logger.Printf("received signal %v, shutting down", sig)
			for _, d := range directions {
				if err := d.sub.Close(); err != nil {
					logger.Printf("[%s] error closing sub client for %q: %v", d.label, d.subject, err)
				}
				if err := d.pub.Close(); err != nil {
					logger.Printf("[%s] error closing pub client for %q: %v", d.label, d.subject, err)
				}
			}
			logger.Printf("final stats: forwarded=%d failed=%d loop-prevented=%d",
				forwarded.Load(), failed.Load(), loopPrevented.Load())
			return
		}
	}
}
