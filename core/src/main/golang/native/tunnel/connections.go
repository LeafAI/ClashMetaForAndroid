package tunnel

import (
	"strings"
	"sync"
	"time"

	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel/statistic"

	"cfa/native/app"
	"cfa/native/connview"
	"cfa/native/sampler"
)

const (
	maxConnectionEntries   = 500
	connectionHistorySize  = 1000
	connectionPollInterval = 250 * time.Millisecond
	connectionSampleGap    = 20 * time.Millisecond
)

var (
	connections = connview.NewHistory(connectionHistorySize)

	recorderOnce sync.Once
)

// StartConnectionRecorder begins remembering connections for the lifetime of the
// process. The core only lists live connections, so they are sampled: on a timer,
// and right when the core logs that it opened one, which catches connections that
// live for less than a tick. It is safe to call more than once.
func StartConnectionRecorder() {
	recorderOnce.Do(func() {
		sampler := sampler.New(ObserveConnections, connectionPollInterval, connectionSampleGap)

		go func() {
			sub := log.Subscribe()
			defer log.UnSubscribe(sub)

			for event := range sub {
				if isConnectionEvent(event) {
					sampler.Trigger()
				}
			}
		}()
	})
}

// isConnectionEvent reports whether the log event is the line the core writes
// when it opens a connection, for example "[TCP] 1.2.3.4:5 --> host:80 match ...".
func isConnectionEvent(event log.Event) bool {
	if event.LogLevel != log.INFO {
		return false
	}

	return (strings.HasPrefix(event.Payload, "[TCP] ") || strings.HasPrefix(event.Payload, "[UDP] ")) &&
		strings.Contains(event.Payload, " --> ")
}

// ObserveConnections takes a snapshot of the live connections right away, so
// connections that just closed are marked without waiting for the next poll.
func ObserveConnections() {
	live := make([]connview.Entry, 0, 64)

	statistic.DefaultManager.Range(func(t statistic.Tracker) bool {
		live = append(live, toEntry(t.Info()))

		return true
	})

	connections.Observe(live, time.Now())
}

// ResetConnectionHistory forgets every remembered connection, including live ones.
func ResetConnectionHistory() {
	connections.Reset()
}

// ClearClosedConnections forgets the connections that already closed.
func ClearClosedConnections() {
	ObserveConnections()
	connections.ClearClosed()
}

// QueryConnections returns every connection seen since the history was last reset
// that matches filter, closed ones included unless activeOnly is set.
func QueryConnections(filter string, sortMode connview.SortMode, activeOnly bool) []connview.Entry {
	ObserveConnections()

	return connview.Query(connections.Entries(), connview.Options{
		Filter:     filter,
		Sort:       sortMode,
		ActiveOnly: activeOnly,
		Limit:      maxConnectionEntries,
	})
}

// CloseConnection closes one tracked connection and reports whether it existed.
func CloseConnection(id string) bool {
	tracker := statistic.DefaultManager.Get(id)
	if tracker == nil {
		return false
	}

	_ = tracker.Close()

	ObserveConnections()

	return true
}

func toEntry(info *statistic.TrackerInfo) connview.Entry {
	entry := connview.Entry{
		ID:          info.UUID.String(),
		Rule:        info.Rule,
		RulePayload: info.RulePayload,
		Chains:      append([]string(nil), info.Chain...),
		Upload:      info.UploadTotal.Load(),
		Download:    info.DownloadTotal.Load(),
		Start:       info.Start,
	}

	if meta := info.Metadata; meta != nil {
		entry.Network = meta.NetWork.String()
		entry.Host = meta.Host
		entry.Port = meta.DstPort
		entry.Process = processName(meta)

		if meta.DstIP.IsValid() {
			entry.DestinationIP = meta.DstIP.String()
		}
	}

	return entry
}

func processName(meta *C.Metadata) string {
	if meta.Process != "" {
		return meta.Process
	}

	if meta.Uid != 0 {
		return app.QueryAppByUid(int(meta.Uid))
	}

	return ""
}
