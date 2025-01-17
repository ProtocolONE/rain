package peerprotocol

import (
	"encoding/binary"
	"io"
)

type Message interface {
	io.Reader
	ID() MessageID
}

type HaveMessage struct {
	Index uint32
}

func (m HaveMessage) ID() MessageID { return Have }

func (m HaveMessage) Read(b []byte) (int, error) {
	binary.BigEndian.PutUint32(b[0:4], m.Index)
	return 4, io.EOF
}

type RequestMessage struct {
	Index, Begin, Length uint32
}

func (m RequestMessage) ID() MessageID { return Request }

func (m RequestMessage) Read(b []byte) (int, error) {
	binary.BigEndian.PutUint32(b[0:4], m.Index)
	binary.BigEndian.PutUint32(b[4:8], m.Begin)
	binary.BigEndian.PutUint32(b[8:12], m.Length)
	return 12, io.EOF
}

type PieceMessage struct {
	Index, Begin uint32
}

func (m PieceMessage) ID() MessageID { return Piece }

func (m PieceMessage) Read(b []byte) (int, error) {
	binary.BigEndian.PutUint32(b[0:4], m.Index)
	binary.BigEndian.PutUint32(b[4:8], m.Begin)
	return 8, io.EOF
}

type BitfieldMessage struct {
	Data []byte
	pos  int
}

func (m BitfieldMessage) ID() MessageID { return Bitfield }

func (m *BitfieldMessage) Read(b []byte) (n int, err error) {
	n = copy(b, m.Data[m.pos:])
	m.pos += n
	if m.pos == len(m.Data) {
		err = io.EOF
	}
	return
}

type PortMessage struct {
	Port uint16
}

func (m PortMessage) ID() MessageID { return Port }

func (m PortMessage) Read(b []byte) (n int, err error) {
	binary.BigEndian.PutUint16(b[0:2], m.Port)
	return 2, io.EOF
}

type emptyMessage struct{}

func (m emptyMessage) Read(b []byte) (int, error) {
	return 0, io.EOF
}

type AllowedFastMessage struct{ HaveMessage }

type ChokeMessage struct{ emptyMessage }
type UnchokeMessage struct{ emptyMessage }
type InterestedMessage struct{ emptyMessage }
type NotInterestedMessage struct{ emptyMessage }
type HaveAllMessage struct{ emptyMessage }
type HaveNoneMessage struct{ emptyMessage }
type RejectMessage struct{ RequestMessage }
type CancelMessage struct{ RequestMessage }

func (m ChokeMessage) ID() MessageID         { return Choke }
func (m UnchokeMessage) ID() MessageID       { return Unchoke }
func (m InterestedMessage) ID() MessageID    { return Interested }
func (m NotInterestedMessage) ID() MessageID { return NotInterested }
func (m HaveAllMessage) ID() MessageID       { return HaveAll }
func (m HaveNoneMessage) ID() MessageID      { return HaveNone }
func (m RejectMessage) ID() MessageID        { return Reject }
func (m CancelMessage) ID() MessageID        { return Cancel }
