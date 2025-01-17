package torrent

import "time"

var (
	publicPeerIDPrefix                    = "-RN" + Version + "-"
	publicExtensionHandshakeClientVersion = "Rain " + Version
	trackerHTTPPublicUserAgent            = "Rain/" + Version
)

// Config for Session.
type Config struct {
	// Database file to save resume data.
	Database string
	// DataDir is where files are downloaded.
	DataDir string
	// New torrents will be listened at selected port in this range.
	PortBegin, PortEnd uint16
	// Enable peer exchange protocol.
	PEXEnabled bool
	// Resume data (bitfield & stats) are saved to disk at interval to keep IO lower.
	ResumeWriteInterval time.Duration
	// Peer id is prefixed with this string. See BEP 20. Remaining bytes of peer id will be randomized.
	// Only applies to private torrents.
	PrivatePeerIDPrefix string
	// Client version that is sent in BEP 10 handshake message.
	// Only applies to private torrents.
	PrivateExtensionHandshakeClientVersion string
	// URL to the blocklist file in CIDR format.
	BlocklistURL string
	// When to refresh blocklist
	BlocklistUpdateInterval time.Duration
	// HTTP timeout for downloading blocklist
	BlocklistUpdateTimeout time.Duration
	// Time to wait when adding torrent with AddURI().
	TorrentAddHTTPTimeout time.Duration
	// Maximum allowed size to be received by metadata extension.
	MaxMetadataSize uint
	// Maximum allowed size to be read when adding torrent.
	MaxTorrentSize uint
	// Time to wait when resolving host names for trackers and peers.
	DNSResolveTimeout time.Duration

	// Enable RPC server
	RPCEnabled bool
	// Host to listen for RPC server
	RPCHost string
	// Listen port for RPC server
	RPCPort int
	// Time to wait for ongoing requests before shutting down RPC HTTP server.
	RPCShutdownTimeout time.Duration

	// Enable DHT node.
	DHTEnabled bool
	// DHT node will listen on this IP.
	DHTHost string
	// DHT node will listen on this UDP port.
	DHTPort uint16
	// DHT announce interval
	DHTAnnounceInterval time.Duration
	// Minimum announce interval when announcing to DHT.
	DHTMinAnnounceInterval time.Duration
	// Known routers to bootstrap local DHT node.
	DHTBootstrapNodes []string

	// Number of peer addresses to request in announce request.
	TrackerNumWant int
	// Time to wait for announcing stopped event.
	// Stopped event is sent to the tracker when torrent is stopped.
	TrackerStopTimeout time.Duration
	// When the client needs new peer addresses to connect, it ask to the tracker.
	// To prevent spamming the tracker an interval is set to wait before the next announce.
	TrackerMinAnnounceInterval time.Duration
	// Total time to wait for response to be read.
	// This includes ConnectTimeout and TLSHandshakeTimeout.
	TrackerHTTPTimeout time.Duration
	// User agent sent when communicating with HTTP trackers.
	// Only applies to private torrents.
	TrackerHTTPPrivateUserAgent string
	// Max number of bytes in a tracker response.
	TrackerHTTPMaxResponseSize uint

	// Number of unchoked peers.
	UnchokedPeers int
	// Number of optimistic unchoked peers.
	OptimisticUnchokedPeers int
	// Max number of blocks allowed to be queued without dropping any.
	MaxRequestsIn int
	// Max number of blocks requested from a peer but not received yet.
	// `rreq` value from extended handshake cannot exceed this limit.
	MaxRequestsOut int
	// Number of bloks requested from peer if it does not send `rreq` value in extended handshake.
	DefaultRequestsOut int
	// Time to wait for a requested block to be received before marking peer as snubbed
	RequestTimeout time.Duration
	// Max number of running downloads on piece in endgame mode, snubbed and choed peers don't count
	EndgameMaxDuplicateDownloads int
	// Max number of outgoing connections to dial
	MaxPeerDial int
	// Max number of incoming connections to accept
	MaxPeerAccept int
	// Number of bytes allocated in memory for downloading piece data.
	MaxActivePieceBytes int64
	// Running metadata downloads, snubbed peers don't count
	ParallelMetadataDownloads int
	// Time to wait for TCP connection to open.
	PeerConnectTimeout time.Duration
	// Time to wait for BitTorrent handshake to complete.
	PeerHandshakeTimeout time.Duration
	// When peer has started to send piece block, if it does not send any bytes in PieceReadTimeout, the connection is closed.
	PieceReadTimeout time.Duration
	// Max number of peer addresses to keep in connect queue.
	MaxPeerAddresses int

	// Number of bytes to read when a piece is requested by a peer.
	PieceReadSize int64
	// Number of cached bytes for piece read requests.
	PieceCacheSize int64
	// Read bytes for a piece part expires after duration.
	PieceCacheTTL time.Duration
	// Number of read operations to do in parallel.
	ParallelReads uint

	// When the client want to connect a peer, first it tries to do encrypted handshake.
	// If it does not work, it connects to same peer again and does unencrypted handshake.
	// This behavior can be changed via this variable.
	DisableOutgoingEncryption bool
	// Dial only encrypted connections.
	ForceOutgoingEncryption bool
	// Do not accept unencrypted connections.
	ForceIncomingEncryption bool

	WebseedDialTimeout             time.Duration
	WebseedTLSHandshakeTimeout     time.Duration
	WebseedResponseHeaderTimeout   time.Duration
	WebseedResponseBodyReadTimeout time.Duration
	WebseedRetryInterval           time.Duration
}

var DefaultConfig = Config{
	// Session
	Database:                               "~/rain/session.db",
	DataDir:                                "~/rain/data",
	PortBegin:                              50000,
	PortEnd:                                60000,
	PEXEnabled:                             true,
	ResumeWriteInterval:                    30 * time.Second,
	PrivatePeerIDPrefix:                    "-RN" + Version + "-",
	PrivateExtensionHandshakeClientVersion: "Rain " + Version,
	BlocklistUpdateInterval:                24 * time.Hour,
	BlocklistUpdateTimeout:                 10 * time.Minute,
	TorrentAddHTTPTimeout:                  30 * time.Second,
	MaxMetadataSize:                        10 * 1024 * 1024,
	MaxTorrentSize:                         10 * 1024 * 1024,
	DNSResolveTimeout:                      5 * time.Second,

	// RPC Server
	RPCEnabled:         true,
	RPCHost:            "127.0.0.1",
	RPCPort:            7246,
	RPCShutdownTimeout: 5 * time.Second,

	// Tracker
	TrackerNumWant:              200,
	TrackerStopTimeout:          5 * time.Second,
	TrackerMinAnnounceInterval:  time.Minute,
	TrackerHTTPTimeout:          10 * time.Second,
	TrackerHTTPPrivateUserAgent: "Rain/" + Version,
	TrackerHTTPMaxResponseSize:  2 * 1024 * 1024,

	// DHT node
	DHTEnabled:             true,
	DHTHost:                "0.0.0.0",
	DHTPort:                7246,
	DHTAnnounceInterval:    30 * time.Minute,
	DHTMinAnnounceInterval: time.Minute,
	DHTBootstrapNodes: []string{
		"router.bittorrent.com:6881",
		"dht.transmissionbt.com:6881",
		"router.utorrent.com:6881",
		"dht.libtorrent.org:25401",
		"dht.aelitis.com:6881",
	},

	// Peer
	UnchokedPeers:                3,
	OptimisticUnchokedPeers:      1,
	MaxRequestsIn:                250,
	MaxRequestsOut:               250,
	DefaultRequestsOut:           50,
	RequestTimeout:               20 * time.Second,
	EndgameMaxDuplicateDownloads: 20,
	MaxPeerDial:                  80,
	MaxPeerAccept:                20,
	MaxActivePieceBytes:          1024 * 1024 * 1024,
	ParallelMetadataDownloads:    2,
	PeerConnectTimeout:           5 * time.Second,
	PeerHandshakeTimeout:         10 * time.Second,
	PieceReadTimeout:             30 * time.Second,
	MaxPeerAddresses:             2000,

	// Piece cache
	PieceReadSize:  256 * 1024,
	PieceCacheSize: 256 * 1024 * 1024,
	PieceCacheTTL:  5 * time.Minute,
	ParallelReads:  1,

	// Webseed settings
	WebseedDialTimeout:             10 * time.Second,
	WebseedTLSHandshakeTimeout:     10 * time.Second,
	WebseedResponseHeaderTimeout:   10 * time.Second,
	WebseedResponseBodyReadTimeout: 10 * time.Second,
	WebseedRetryInterval:           time.Minute,
}
