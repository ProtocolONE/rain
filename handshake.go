package rain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/cenkalti/rain/internal/bitfield"
	"github.com/cenkalti/rain/internal/protocol"
)

type peerHandShake struct {
	Pstrlen    byte
	Pstr       [protocol.PstrLen]byte
	Extensions [8]byte
	InfoHash   protocol.InfoHash
	PeerID     protocol.PeerID
}

func newPeerHandShake(ih protocol.InfoHash, id protocol.PeerID, extensions [8]byte) *peerHandShake {
	h := &peerHandShake{
		Pstrlen:    protocol.PstrLen,
		Extensions: extensions,
		InfoHash:   ih,
		PeerID:     id,
	}
	copy(h.Pstr[:], protocol.Pstr)
	return h
}

func (p *peer) sendHandShake(ih protocol.InfoHash, id protocol.PeerID, extensions [8]byte) error {
	return binary.Write(p.conn, binary.BigEndian, newPeerHandShake(ih, id, extensions))
}

func (p *peer) readHandShake1() (extensions *bitfield.BitField, ih *protocol.InfoHash, err error) {
	var pstrLen byte
	err = binary.Read(p.conn, binary.BigEndian, &pstrLen)
	if err != nil {
		return
	}
	if pstrLen != protocol.PstrLen {
		err = fmt.Errorf("invalid pstrlen: %d != %d", pstrLen, protocol.PstrLen)
		return
	}

	pstr := make([]byte, protocol.PstrLen)
	_, err = io.ReadFull(p.conn, pstr)
	if err != nil {
		return
	}
	if bytes.Compare(pstr, protocol.Pstr) != 0 {
		err = fmt.Errorf("invalid pstr: %q != %q", string(pstr), string(protocol.Pstr))
		return
	}

	b := bitfield.New(nil, 64)
	_, err = io.ReadFull(p.conn, b.Bytes())
	if err != nil {
		return
	}
	extensions = &b

	var infoHash protocol.InfoHash
	_, err = io.ReadFull(p.conn, infoHash[:])
	if err != nil {
		return
	}
	ih = &infoHash

	return
}

func (p *peer) readHandShake2() (*protocol.PeerID, error) {
	var id protocol.PeerID
	_, err := io.ReadFull(p.conn, id[:])
	if err != nil {
		return nil, err
	}
	return &id, nil
}