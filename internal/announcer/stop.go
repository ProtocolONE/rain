package announcer

import (
	"context"
	"time"

	"github.com/ProtocolONE/rain/internal/logger"
	"github.com/ProtocolONE/rain/internal/tracker"
)

type StopAnnouncer struct {
	log      logger.Logger
	timeout  time.Duration
	trackers []tracker.Tracker
	torrent  tracker.Torrent
	resultC  chan struct{}
	closeC   chan struct{}
	doneC    chan struct{}
}

func NewStopAnnouncer(trackers []tracker.Tracker, tra tracker.Torrent, timeout time.Duration, resultC chan struct{}, l logger.Logger) *StopAnnouncer {
	return &StopAnnouncer{
		log:      l,
		timeout:  timeout,
		trackers: trackers,
		torrent:  tra,
		resultC:  resultC,
		closeC:   make(chan struct{}),
		doneC:    make(chan struct{}),
	}
}

func (a *StopAnnouncer) Close() {
	close(a.closeC)
	<-a.doneC
}

func (a *StopAnnouncer) Run() {
	defer close(a.doneC)

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(a.timeout))
	go func() {
		select {
		case <-ctx.Done():
		case <-a.closeC:
			cancel()
		}
	}()

	doneC := make(chan struct{})
	for _, trk := range a.trackers {
		go func(trk tracker.Tracker) {
			req := tracker.AnnounceRequest{
				Torrent: a.torrent,
				Event:   tracker.EventStopped,
			}
			_, _ = trk.Announce(ctx, req)
			doneC <- struct{}{}
		}(trk)
	}
	for range a.trackers {
		<-doneC
	}
	select {
	case a.resultC <- struct{}{}:
	case <-a.closeC:
	}
}
