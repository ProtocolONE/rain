package handler

import (
	"net"

	"github.com/cenkalti/rain/internal/btconn"
	"github.com/cenkalti/rain/internal/logger"
	"github.com/cenkalti/rain/internal/peer"
	"github.com/cenkalti/rain/internal/peermanager/peerids"
	"github.com/cenkalti/rain/internal/torrentdata"
)

type Handler struct {
	conn     net.Conn
	peerIDs  *peerids.PeerIDs
	data     *torrentdata.Data
	peerID   [20]byte
	sKeyHash [20]byte
	infoHash [20]byte
	messages *peer.Messages
	log      logger.Logger
}

func New(conn net.Conn, peerIDs *peerids.PeerIDs, data *torrentdata.Data, peerID, sKeyHash, infoHash [20]byte, messages *peer.Messages, l logger.Logger) *Handler {
	return &Handler{
		conn:     conn,
		peerIDs:  peerIDs,
		data:     data,
		peerID:   peerID,
		sKeyHash: sKeyHash,
		infoHash: infoHash,
		messages: messages,
		log:      l,
	}
}

func (h *Handler) Run(stopC chan struct{}) {
	log := logger.New("peer <- " + h.conn.RemoteAddr().String())

	// TODO get this from config
	encryptionForceIncoming := false
	extensions := [8]byte{}

	// TODO close conn during handshake when stopC is closed
	encConn, cipher, extensions, peerID, _, err := btconn.Accept(
		h.conn, h.getSKey, encryptionForceIncoming, h.checkInfoHash, extensions, h.peerID)
	if err != nil {
		log.Error(err)
		_ = h.conn.Close()
		return
	}
	log.Infof("Connection accepted. (cipher=%s extensions=%x client=%q)", cipher, extensions, peerID[:8])

	ok := h.peerIDs.Add(peerID)
	if !ok {
		_ = h.conn.Close()
		return
	}
	defer h.peerIDs.Remove(peerID)

	p := peer.New(encConn, peerID, h.data, log, h.messages)
	p.Run(stopC)
}

func (h *Handler) getSKey(sKeyHash [20]byte) []byte {
	if sKeyHash == h.sKeyHash {
		return h.infoHash[:]
	}
	return nil
}

func (h *Handler) checkInfoHash(infoHash [20]byte) bool {
	return infoHash == h.infoHash
}
