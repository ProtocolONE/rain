package torrent

import (
	"errors"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ProtocolONE/rain/internal/acceptor"
	"github.com/ProtocolONE/rain/internal/addrlist"
	"github.com/ProtocolONE/rain/internal/allocator"
	"github.com/ProtocolONE/rain/internal/announcer"
	"github.com/ProtocolONE/rain/internal/bitfield"
	"github.com/ProtocolONE/rain/internal/bufferpool"
	"github.com/ProtocolONE/rain/internal/counters"
	"github.com/ProtocolONE/rain/internal/externalip"
	"github.com/ProtocolONE/rain/internal/handshaker/incominghandshaker"
	"github.com/ProtocolONE/rain/internal/handshaker/outgoinghandshaker"
	"github.com/ProtocolONE/rain/internal/infodownloader"
	"github.com/ProtocolONE/rain/internal/logger"
	"github.com/ProtocolONE/rain/internal/metainfo"
	"github.com/ProtocolONE/rain/internal/mse"
	"github.com/ProtocolONE/rain/internal/peer"
	"github.com/ProtocolONE/rain/internal/piece"
	"github.com/ProtocolONE/rain/internal/piecedownloader"
	"github.com/ProtocolONE/rain/internal/piecepicker"
	"github.com/ProtocolONE/rain/internal/piecewriter"
	"github.com/ProtocolONE/rain/internal/resumer"
	"github.com/ProtocolONE/rain/internal/storage"
	"github.com/ProtocolONE/rain/internal/suspendchan"
	"github.com/ProtocolONE/rain/internal/tracker"
	"github.com/ProtocolONE/rain/internal/unchoker"
	"github.com/ProtocolONE/rain/internal/verifier"
	"github.com/ProtocolONE/rain/internal/webseedsource"
	"github.com/rcrowley/go-metrics"
)

// torrent connects to peers and downloads files from swarm.
type torrent struct {
	session *Session
	id      string
	addedAt time.Time

	// Identifies the torrent being downloaded.
	infoHash [20]byte

	// List of addresses to announce this torrent.
	trackers []tracker.Tracker

	// Peers added from magnet URLS with x.pe parameter.
	fixedPeers []string

	// Name of the torrent.
	name string

	// Storage implementation to save the files in torrent.
	storage storage.Storage

	// TCP Port to listen for peer connections.
	port int

	// Contains info about files in torrent. This can be nil at start for magnet downloads.
	info *metainfo.Info

	// Bitfield for pieces we have. It is created after we got info.
	bitfield *bitfield.Bitfield

	// Protects bitfield writing from torrent loop and reading from announcer loop.
	mBitfield sync.RWMutex

	// Unique peer ID is generated per downloader.
	peerID [20]byte

	files  []allocator.File
	pieces []piece.Piece

	piecePicker *piecepicker.PiecePicker

	// Peers are sent to this channel when they are disconnected.
	peerDisconnectedC chan *peer.Peer

	// Piece messages coming from peers are sent this channel.
	pieceMessagesC *suspendchan.Chan

	// Other messages coming from peers are sent to this channel.
	messages chan peer.Message

	// We keep connected peers in this map after they complete handshake phase.
	peers map[*peer.Peer]struct{}

	// Also keep a reference to incoming and outgoing peers separately to count them quickly.
	incomingPeers map[*peer.Peer]struct{}
	outgoingPeers map[*peer.Peer]struct{}

	// Unchoker implements an algorithm to select peers to unchoke based on their download speed.
	unchoker *unchoker.Unchoker

	// Active piece downloads are kept in this map.
	pieceDownloaders        map[*peer.Peer]*piecedownloader.PieceDownloader
	pieceDownloadersSnubbed map[*peer.Peer]*piecedownloader.PieceDownloader
	pieceDownloadersChoked  map[*peer.Peer]*piecedownloader.PieceDownloader

	// When a peer has snubbed us, a message sent to this channel.
	peerSnubbedC chan *peer.Peer

	// Active metadata downloads are kept in this map.
	infoDownloaders        map[*peer.Peer]*infodownloader.InfoDownloader
	infoDownloadersSnubbed map[*peer.Peer]*infodownloader.InfoDownloader

	pieceWriterResultC chan *piecewriter.PieceWriter

	// This channel is closed once all pieces are downloaded and verified.
	completeC chan struct{}

	// True after all pieces are download, verified and written to disk.
	completed bool

	// If any unrecoverable error occurs, it will be sent to this channel and download will be stopped.
	errC chan error

	// After listener has started, port will be sent to this channel.
	portC chan int

	// Contains the last error sent to errC.
	lastError error

	// When Stop() is called, it will close this channel to signal run() function to stop.
	closeC chan chan struct{}

	// Close() blocks until doneC is closed.
	doneC chan struct{}

	// These are the channels for sending a message to run() loop.
	statsCommandC        chan statsRequest        // Stats()
	trackersCommandC     chan trackersRequest     // Trackers()
	peersCommandC        chan peersRequest        // Peers()
	webseedsCommandC     chan webseedsRequest     // Webseeds()
	startCommandC        chan struct{}            // Start()
	stopCommandC         chan struct{}            // Stop()
	notifyErrorCommandC  chan notifyErrorCommand  // NotifyError()
	notifyListenCommandC chan notifyListenCommand // NotifyListen()
	addPeersCommandC     chan []*net.TCPAddr      // AddPeers()
	addTrackersCommandC  chan []tracker.Tracker   // AddTrackers()

	// Trackers send announce responses to this channel.
	addrsFromTrackers chan []*net.TCPAddr

	// Keeps a list of peer addresses to connect.
	addrList *addrlist.AddrList

	// New raw connections created by OutgoingHandshaker are sent to here.
	incomingConnC chan net.Conn

	// Keep a set of peer IDs to block duplicate connections.
	peerIDs map[[20]byte]struct{}

	// Listens for incoming peer connections.
	acceptor *acceptor.Acceptor

	// Special hash of info hash for encypted connection handshake.
	sKeyHash [20]byte

	// Announces the status of torrent to trackers to get peer addresses periodically.
	announcers []*announcer.PeriodicalAnnouncer

	// This announcer announces Stopped event to the trackers after
	// all periodical trackers are closed.
	stoppedEventAnnouncer *announcer.StopAnnouncer

	// If not nil, torrent is announced to DHT periodically.
	dhtAnnouncer *announcer.DHTAnnouncer
	dhtPeersC    chan []*net.TCPAddr

	// List of peers in handshake state.
	incomingHandshakers map[*incominghandshaker.IncomingHandshaker]struct{}
	outgoingHandshakers map[*outgoinghandshaker.OutgoingHandshaker]struct{}

	// Handshake results are sent to these channels by handshakers.
	incomingHandshakerResultC chan *incominghandshaker.IncomingHandshaker
	outgoingHandshakerResultC chan *outgoinghandshaker.OutgoingHandshaker

	// When metadata of the torrent downloaded completely, a message is sent to this channel.
	infoDownloaderResultC chan *infodownloader.InfoDownloader

	// A ticker that ticks periodically to keep a certain number of peers unchoked.
	unchokeTicker *time.Ticker

	// A worker that opens and allocates files on the disk.
	allocator          *allocator.Allocator
	allocatorProgressC chan allocator.Progress
	allocatorResultC   chan *allocator.Allocator
	bytesAllocated     int64

	// A worker that does hash check of files on the disk.
	verifier          *verifier.Verifier
	verifierProgressC chan verifier.Progress
	verifierResultC   chan *verifier.Verifier
	checkedPieces     uint32

	counters              counters.Counters
	seedDurationUpdatedAt time.Time
	seedDurationTicker    *time.Ticker

	// Holds connected peer IPs so we don't dial/accept multiple connections to/from same IP.
	connectedPeerIPs map[string]struct{}

	// Peers that are sending corrupt data are banned.
	bannedPeerIPs map[string]struct{}

	// A signal sent to run() loop when announcers are stopped.
	announcersStoppedC chan struct{}

	// Piece buffers that are being downloaded are pooled to reduce load on GC.
	piecePool *bufferpool.Pool

	// Used to calculate canonical peer priority (BEP 40).
	// Initialized with value found in network interfaces.
	// Then, updated from "yourip" field in BEP 10 extension handshake message.
	externalIP net.IP

	// Rate counters for download and upload speeds.
	downloadSpeed      metrics.EWMA
	uploadSpeed        metrics.EWMA
	speedCounterTicker *time.Ticker

	ramNotifyC chan interface{}

	webseedClient       *http.Client
	webseedSources      []*webseedsource.WebseedSource
	webseedPieceResultC *suspendchan.Chan
	webseedRetryC       chan *webseedsource.WebseedSource

	log logger.Logger
}

func newTorrent2(
	s *Session,
	id string,
	addedAt time.Time,
	infoHash []byte,
	sto storage.Storage,
	name string, // display name
	port int, // tcp peer port
	trackers []tracker.Tracker,
	fixedPeers []string,
	info *metainfo.Info,
	bf *bitfield.Bitfield,
	stats resumer.Stats, // initial stats from previous run
) (*torrent, error) {
	if len(infoHash) != 20 {
		return nil, errors.New("invalid infoHash (must be 20 bytes)")
	}
	cfg := s.config
	var ih [20]byte
	copy(ih[:], infoHash)
	t := &torrent{
		session:                   s,
		id:                        id,
		addedAt:                   addedAt,
		infoHash:                  ih,
		trackers:                  trackers,
		fixedPeers:                fixedPeers,
		name:                      name,
		storage:                   sto,
		port:                      port,
		info:                      info,
		bitfield:                  bf,
		log:                       logger.New("torrent " + id),
		peerDisconnectedC:         make(chan *peer.Peer),
		messages:                  make(chan peer.Message),
		pieceMessagesC:            suspendchan.New(0),
		peers:                     make(map[*peer.Peer]struct{}),
		incomingPeers:             make(map[*peer.Peer]struct{}),
		outgoingPeers:             make(map[*peer.Peer]struct{}),
		pieceDownloaders:          make(map[*peer.Peer]*piecedownloader.PieceDownloader),
		pieceDownloadersSnubbed:   make(map[*peer.Peer]*piecedownloader.PieceDownloader),
		pieceDownloadersChoked:    make(map[*peer.Peer]*piecedownloader.PieceDownloader),
		peerSnubbedC:              make(chan *peer.Peer),
		infoDownloaders:           make(map[*peer.Peer]*infodownloader.InfoDownloader),
		infoDownloadersSnubbed:    make(map[*peer.Peer]*infodownloader.InfoDownloader),
		pieceWriterResultC:        make(chan *piecewriter.PieceWriter),
		completeC:                 make(chan struct{}),
		closeC:                    make(chan chan struct{}),
		startCommandC:             make(chan struct{}),
		stopCommandC:              make(chan struct{}),
		statsCommandC:             make(chan statsRequest),
		trackersCommandC:          make(chan trackersRequest),
		peersCommandC:             make(chan peersRequest),
		webseedsCommandC:          make(chan webseedsRequest),
		notifyErrorCommandC:       make(chan notifyErrorCommand),
		notifyListenCommandC:      make(chan notifyListenCommand),
		addPeersCommandC:          make(chan []*net.TCPAddr),
		addTrackersCommandC:       make(chan []tracker.Tracker),
		addrsFromTrackers:         make(chan []*net.TCPAddr),
		peerIDs:                   make(map[[20]byte]struct{}),
		incomingConnC:             make(chan net.Conn),
		sKeyHash:                  mse.HashSKey(ih[:]),
		infoDownloaderResultC:     make(chan *infodownloader.InfoDownloader),
		incomingHandshakers:       make(map[*incominghandshaker.IncomingHandshaker]struct{}),
		outgoingHandshakers:       make(map[*outgoinghandshaker.OutgoingHandshaker]struct{}),
		incomingHandshakerResultC: make(chan *incominghandshaker.IncomingHandshaker),
		outgoingHandshakerResultC: make(chan *outgoinghandshaker.OutgoingHandshaker),
		allocatorProgressC:        make(chan allocator.Progress),
		allocatorResultC:          make(chan *allocator.Allocator),
		verifierProgressC:         make(chan verifier.Progress),
		verifierResultC:           make(chan *verifier.Verifier),
		connectedPeerIPs:          make(map[string]struct{}),
		bannedPeerIPs:             make(map[string]struct{}),
		announcersStoppedC:        make(chan struct{}),
		dhtPeersC:                 make(chan []*net.TCPAddr, 1),
		counters:                  counters.New(stats.BytesDownloaded, stats.BytesUploaded, stats.BytesWasted, stats.SeededFor),
		externalIP:                externalip.FirstExternalIP(),
		downloadSpeed:             metrics.NewEWMA1(),
		uploadSpeed:               metrics.NewEWMA1(),
		ramNotifyC:                make(chan interface{}),
		webseedPieceResultC:       suspendchan.New(0),
		webseedRetryC:             make(chan *webseedsource.WebseedSource),
		doneC:                     make(chan struct{}),
	}
	t.addrList = addrlist.New(cfg.MaxPeerAddresses, s.blocklist, port, &t.externalIP)
	if t.info != nil {
		t.piecePool = bufferpool.New(int(t.info.PieceLength))
	}
	n := t.copyPeerIDPrefix()
	_, err := rand.Read(t.peerID[n:]) // nolint: gosec
	if err != nil {
		return nil, err
	}
	t.unchoker = unchoker.New(cfg.UnchokedPeers, cfg.OptimisticUnchokedPeers)
	go t.run()
	return t, nil
}

func (t *torrent) copyPeerIDPrefix() int {
	if t.info.IsPrivate() {
		return copy(t.peerID[:], []byte(t.session.config.PrivatePeerIDPrefix))
	}
	return copy(t.peerID[:], []byte(publicPeerIDPrefix))
}

func (t *torrent) getPeersForUnchoker() []unchoker.Peer {
	peers := make([]unchoker.Peer, 0, len(t.peers))
	for pe := range t.peers {
		peers = append(peers, pe)
	}
	return peers
}

// Name of the torrent.
// For magnet downloads name can change after metadata is downloaded but this method still returns the initial name.
// Use Stats() method to get name in info dictionary.
func (t *torrent) Name() string {
	return t.name
}

// InfoHash is a 20-bytes value that identifies the files in torrent.
func (t *torrent) InfoHash() []byte {
	b := make([]byte, 20)
	copy(b, t.infoHash[:])
	return b
}

func (t *torrent) announceDHT() {
	t.session.mPeerRequests.Lock()
	t.session.dhtPeerRequests[t] = struct{}{}
	t.session.mPeerRequests.Unlock()
}
