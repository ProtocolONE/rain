package announcer

import (
	"context"
	"errors"
	"math"
	"net"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/ProtocolONE/rain/internal/logger"
	"github.com/ProtocolONE/rain/internal/tracker"
)

type Status int

const (
	NotContactedYet Status = iota
	Contacting
	Working
	NotWorking
)

type PeriodicalAnnouncer struct {
	Tracker       tracker.Tracker
	status        Status
	statsCommandC chan statsRequest
	numWant       int
	interval      time.Duration
	minInterval   time.Duration
	seeders       int
	leechers      int
	lastError     error
	log           logger.Logger
	completedC    chan struct{}
	newPeers      chan []*net.TCPAddr
	backoff       backoff.BackOff
	getTorrent    func() tracker.Torrent
	lastAnnounce  time.Time
	HasAnnounced  bool
	responseC     chan *tracker.AnnounceResponse
	errC          chan error
	closeC        chan struct{}
	doneC         chan struct{}

	needMorePeers  bool
	mNeedMorePeers sync.RWMutex
	needMorePeersC chan struct{}
}

func NewPeriodicalAnnouncer(trk tracker.Tracker, numWant int, minInterval time.Duration, getTorrent func() tracker.Torrent, completedC chan struct{}, newPeers chan []*net.TCPAddr, l logger.Logger) *PeriodicalAnnouncer {
	return &PeriodicalAnnouncer{
		Tracker:        trk,
		status:         NotContactedYet,
		statsCommandC:  make(chan statsRequest),
		numWant:        numWant,
		minInterval:    minInterval,
		log:            l,
		completedC:     completedC,
		newPeers:       newPeers,
		getTorrent:     getTorrent,
		needMorePeersC: make(chan struct{}, 1),
		responseC:      make(chan *tracker.AnnounceResponse),
		errC:           make(chan error),
		closeC:         make(chan struct{}),
		doneC:          make(chan struct{}),
		backoff: &backoff.ExponentialBackOff{
			InitialInterval:     5 * time.Second,
			RandomizationFactor: 0.5,
			Multiplier:          2,
			MaxInterval:         30 * time.Minute,
			MaxElapsedTime:      0, // never stop
			Clock:               backoff.SystemClock,
		},
	}
}

func (a *PeriodicalAnnouncer) Close() {
	close(a.closeC)
	<-a.doneC
}

type statsRequest struct {
	Response chan Stats
}

func (a *PeriodicalAnnouncer) Stats() Stats {
	var stats Stats
	req := statsRequest{Response: make(chan Stats, 1)}
	select {
	case a.statsCommandC <- req:
	case <-a.closeC:
	}
	select {
	case stats = <-req.Response:
	case <-a.closeC:
	}
	return stats
}

func (a *PeriodicalAnnouncer) NeedMorePeers(val bool) {
	a.mNeedMorePeers.Lock()
	a.needMorePeers = val
	a.mNeedMorePeers.Unlock()
	select {
	case a.needMorePeersC <- struct{}{}:
	case <-a.doneC:
	default:
	}
}

func (a *PeriodicalAnnouncer) Run() {
	defer close(a.doneC)
	a.backoff.Reset()

	timer := time.NewTimer(math.MaxInt64)
	defer timer.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go a.announce(ctx, tracker.EventStarted, a.numWant)
	a.status = Contacting
	for {
		select {
		case <-timer.C:
			if a.status == Contacting {
				break
			}
			go a.announce(ctx, tracker.EventNone, a.numWant)
			a.status = Contacting
		case resp := <-a.responseC:
			a.status = Working
			a.lastAnnounce = time.Now()
			a.seeders = int(resp.Seeders)
			a.leechers = int(resp.Leechers)
			a.interval = resp.Interval
			if resp.MinInterval > 0 {
				a.minInterval = resp.MinInterval
			}
			a.HasAnnounced = true
			a.lastError = nil
			a.backoff.Reset()
			a.mNeedMorePeers.RLock()
			needMorePeers := a.needMorePeers
			a.mNeedMorePeers.RUnlock()
			if needMorePeers {
				timer.Reset(a.minInterval)
			} else {
				timer.Reset(a.interval)
			}
		case a.lastError = <-a.errC:
			a.status = NotWorking
			a.lastAnnounce = time.Now()
			if oerr, ok := a.lastError.(*net.OpError); ok && oerr.Error() == "operation was canceled" {
				// Give more friendly error to the user
				a.lastError = errors.New("timeout")
			}
			a.log.Debugln("announce error:", a.lastError)
			if terr, ok := a.lastError.(*tracker.Error); ok && terr.RetryIn > 0 {
				timer.Reset(terr.RetryIn)
			} else {
				timer.Reset(a.backoff.NextBackOff())
			}
		case <-a.needMorePeersC:
			a.mNeedMorePeers.RLock()
			needMorePeers := a.needMorePeers
			a.mNeedMorePeers.RUnlock()
			if a.status == Contacting {
				break
			}
			if needMorePeers {
				timer.Reset(time.Until(a.lastAnnounce.Add(a.minInterval)))
			} else {
				timer.Reset(time.Until(a.lastAnnounce.Add(a.interval)))
			}
		case <-a.completedC:
			if a.status == Contacting {
				cancel()
			}
			go a.announce(ctx, tracker.EventCompleted, 0)
			a.status = Contacting
			a.completedC = nil // do not send more than one "completed" event
		case req := <-a.statsCommandC:
			req.Response <- a.stats()
		case <-a.closeC:
			return
		}
	}
}

func (a *PeriodicalAnnouncer) announce(ctx context.Context, event tracker.Event, numWant int) {
	announce(ctx, a.Tracker, event, numWant, a.getTorrent(), a.responseC, a.errC)
}

type Stats struct {
	Status   Status
	Error    error
	Seeders  int
	Leechers int
}

func (a *PeriodicalAnnouncer) stats() Stats {
	return Stats{
		Status:   a.status,
		Error:    a.lastError,
		Seeders:  a.seeders,
		Leechers: a.leechers,
	}
}
