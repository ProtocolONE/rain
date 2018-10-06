package peerreader

import (
	"encoding/binary"
	"io"
	"io/ioutil"
	"net"
	"time"

	"github.com/cenkalti/rain/torrent/internal/logger"
	"github.com/cenkalti/rain/torrent/internal/peerconn/peerprotocol"
)

// MaxAllowedBlockSize is the max size of block data that we accept from peers.
const MaxAllowedBlockSize = 32 * 1024

const connReadTimeout = 3 * time.Minute

type PeerReader struct {
	conn              net.Conn
	log               logger.Logger
	messages          chan interface{}
	fastExtension     bool
	extensionProtocol bool
}

func New(conn net.Conn, l logger.Logger, fastExtension, extensionProtocol bool) *PeerReader {
	return &PeerReader{
		conn:              conn,
		log:               l,
		messages:          make(chan interface{}),
		fastExtension:     fastExtension,
		extensionProtocol: extensionProtocol,
	}
}

func (p *PeerReader) Messages() <-chan interface{} {
	return p.messages
}

func (p *PeerReader) Run(stopC chan struct{}) {
	defer close(p.messages)
	first := true
	for {
		err := p.conn.SetReadDeadline(time.Now().Add(connReadTimeout))
		if err != nil {
			p.log.Error(err)
			return
		}

		// TODO use bufio.Reader for reading peer messages to reduce syscalls
		var length uint32
		// p.log.Debug("Reading message...")
		err = binary.Read(p.conn, binary.BigEndian, &length)
		if err != nil {
			select {
			case <-stopC:
			default:
				p.log.Error(err)
			}
			return
		}
		// p.log.Debugf("Received message of length: %d", length)

		if length == 0 { // keep-alive message
			p.log.Debug("Received message of type \"keep alive\"")
			continue
		}

		var id peerprotocol.MessageID
		err = binary.Read(p.conn, binary.BigEndian, &id)
		if err != nil {
			p.log.Error(err)
			return
		}
		length--

		p.log.Debugf("Received message of type: %q", id)

		// TODO consider defining a type for peer message
		var msg interface{}

		switch id {
		case peerprotocol.Choke:
			first = false
			msg = peerprotocol.ChokeMessage{}
		case peerprotocol.Unchoke:
			first = false
			msg = peerprotocol.UnchokeMessage{}
		case peerprotocol.Interested:
			first = false
			msg = peerprotocol.InterestedMessage{}
		case peerprotocol.NotInterested:
			first = false
			msg = peerprotocol.NotInterestedMessage{}
		case peerprotocol.Have:
			first = false
			var hm peerprotocol.HaveMessage
			err = binary.Read(p.conn, binary.BigEndian, &hm)
			if err != nil {
				p.log.Error(err)
				return
			}
			msg = hm
		case peerprotocol.Bitfield:
			if !first {
				p.log.Error("bitfield can only be sent after handshake")
				return
			}
			first = false
			var bm peerprotocol.BitfieldMessage
			bm.Data = make([]byte, length)
			_, err = io.ReadFull(p.conn, bm.Data)
			if err != nil {
				p.log.Error(err)
				return
			}
			msg = bm
		case peerprotocol.Request:
			first = false
			var rm peerprotocol.RequestMessage
			err = binary.Read(p.conn, binary.BigEndian, &rm)
			if err != nil {
				p.log.Error(err)
				return
			}
			p.log.Debugf("Received Request: %+v", rm)

			if rm.Length > MaxAllowedBlockSize {
				p.log.Error("received a request with block size larger than allowed")
				return
			}
			msg = rm
		case peerprotocol.Reject:
			if !p.fastExtension {
				p.log.Error("reject message received but fast extensions is not enabled")
				return
			}
			var rm peerprotocol.RejectMessage
			err = binary.Read(p.conn, binary.BigEndian, &rm)
			if err != nil {
				p.log.Error(err)
				return
			}
			p.log.Debugf("Received Reject: %+v", rm)

			if rm.Length > MaxAllowedBlockSize {
				p.log.Error("received a reject with block size larger than allowed")
				return
			}
			msg = rm
		case peerprotocol.Piece:
			first = false
			// TODO send a reader as message to read directly onto the piece buffer
			buf := make([]byte, length)
			_, err = io.ReadFull(p.conn, buf)
			if err != nil {
				p.log.Error(err)
				return
			}
			pm := peerprotocol.PieceMessage{Length: length - 8}
			err = pm.UnmarshalBinary(buf)
			if err != nil {
				p.log.Error(err)
				return
			}
			msg = pm
		case peerprotocol.HaveAll:
			if !p.fastExtension {
				p.log.Error("have_all message received but fast extensions is not enabled")
				return
			}
			if !first {
				p.log.Error("have_all can only be sent after handshake")
				return
			}
			first = false
			msg = peerprotocol.HaveAllMessage{}
		case peerprotocol.HaveNone:
			if !p.fastExtension {
				p.log.Error("have_none message received but fast extensions is not enabled")
				return
			}
			if !first {
				p.log.Error("have_none can only be sent after handshake")
				return
			}
			first = false
			msg = peerprotocol.HaveNoneMessage{}
		// case peerprotocol.Suggest:
		case peerprotocol.AllowedFast:
			var am peerprotocol.AllowedFastMessage
			err = binary.Read(p.conn, binary.BigEndian, &am)
			if err != nil {
				p.log.Error(err)
				return
			}
			msg = am
		// case peerprotocol.Cancel: TODO handle cancel messages
		// TODO handle extension messages
		case peerprotocol.Extension:
			buf := make([]byte, length)
			_, err = io.ReadFull(p.conn, buf)
			if err != nil {
				p.log.Error(err)
				return
			}
			if !p.extensionProtocol {
				p.log.Error("extension message received but it is not enabled in bitfield")
				break
			}
			em := peerprotocol.NewExtensionMessage(length - 1)
			err = em.UnmarshalBinary(buf)
			if err != nil {
				p.log.Error(err)
				return
			}
			msg = em.Payload
		default:
			p.log.Debugf("unhandled message type: %s", id)
			p.log.Debugln("Discarding", length, "bytes...")
			_, err = io.CopyN(ioutil.Discard, p.conn, int64(length))
			if err != nil {
				p.log.Error(err)
				return
			}
			continue
		}
		if msg == nil {
			panic("msg unset")
		}
		select {
		case p.messages <- msg:
		case <-stopC:
			return
		}
	}
}