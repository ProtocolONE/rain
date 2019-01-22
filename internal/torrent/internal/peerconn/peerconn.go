package peerconn

import (
	"io"
	"net"
	"time"

	"github.com/cenkalti/rain/internal/logger"
	"github.com/cenkalti/rain/internal/torrent/bitfield"
	"github.com/cenkalti/rain/internal/torrent/internal/peerconn/peerreader"
	"github.com/cenkalti/rain/internal/torrent/internal/peerconn/peerwriter"
	"github.com/cenkalti/rain/internal/torrent/internal/peerprotocol"
)

type Conn struct {
	conn          net.Conn
	id            [20]byte
	FastExtension bool
	reader        *peerreader.PeerReader
	writer        *peerwriter.PeerWriter
	messages      chan interface{}
	log           logger.Logger
	closeC        chan struct{}
	doneC         chan struct{}
}

func New(conn net.Conn, id [20]byte, extensions *bitfield.Bitfield, l logger.Logger, pieceTimeout time.Duration, readBufferSize int) *Conn {
	fastExtension := extensions.Test(61)
	extensionProtocol := extensions.Test(43)
	return &Conn{
		conn:          conn,
		id:            id,
		FastExtension: fastExtension,
		reader:        peerreader.New(conn, l, pieceTimeout, readBufferSize, fastExtension, extensionProtocol),
		writer:        peerwriter.New(conn, l),
		messages:      make(chan interface{}),
		log:           l,
		closeC:        make(chan struct{}),
		doneC:         make(chan struct{}),
	}
}

func (p *Conn) ID() [20]byte {
	return p.id
}

func (p *Conn) Addr() *net.TCPAddr {
	return p.conn.RemoteAddr().(*net.TCPAddr)
}

func (p *Conn) IP() string {
	return p.conn.RemoteAddr().(*net.TCPAddr).IP.String()
}

func (p *Conn) String() string {
	return p.conn.RemoteAddr().String()
}

func (p *Conn) Close() {
	close(p.closeC)
	<-p.doneC
}

func (p *Conn) CloseConn() {
	p.conn.Close()
}

func (p *Conn) Logger() logger.Logger {
	return p.log
}

func (p *Conn) Messages() <-chan interface{} {
	return p.messages
}

func (p *Conn) SendMessage(msg peerprotocol.Message) {
	p.writer.SendMessage(msg)
}

func (p *Conn) SendPiece(msg peerprotocol.RequestMessage, pi io.ReaderAt) {
	p.writer.SendPiece(msg, pi)
}

func (p *Conn) CancelRequest(msg peerprotocol.CancelMessage) {
	p.writer.CancelRequest(msg)
}

// Run reads and processes incoming messages after handshake.
func (p *Conn) Run() {
	defer close(p.doneC)
	defer close(p.messages)

	p.log.Debugln("Communicating peer", p.conn.RemoteAddr())

	go p.reader.Run()
	defer func() { <-p.reader.Done() }()

	go p.writer.Run()
	defer func() { <-p.writer.Done() }()

	defer p.conn.Close()
	for {
		select {
		case msg := <-p.reader.Messages():
			select {
			case p.messages <- msg:
			case <-p.closeC:
			}
		case msg := <-p.writer.Messages():
			select {
			case p.messages <- msg:
			case <-p.closeC:
			}
		case <-p.closeC:
			p.reader.Stop()
			p.writer.Stop()
			return
		case <-p.reader.Done():
			p.writer.Stop()
			return
		case <-p.writer.Done():
			p.reader.Stop()
			return
		}
	}

}