package peerreader

import (
	"github.com/ProtocolONE/rain/internal/bufferpool"
	"github.com/ProtocolONE/rain/internal/peerprotocol"
)

type Piece struct {
	peerprotocol.PieceMessage
	Buffer bufferpool.Buffer
}
