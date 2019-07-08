package torrent

import (
	"bytes"
	"crypto/sha1" // nolint: gosec
	"errors"
	"fmt"

	"github.com/ProtocolONE/rain/internal/bufferpool"
	"github.com/ProtocolONE/rain/internal/metainfo"
	"github.com/ProtocolONE/rain/internal/peer"
	"github.com/ProtocolONE/rain/internal/peerprotocol"
)

func (t *torrent) handleMetadataMessage(pe *peer.Peer, msg peerprotocol.ExtensionMetadataMessage) {
	switch msg.Type {
	case peerprotocol.ExtensionMetadataMessageTypeRequest:
		if pe.ExtensionHandshake == nil {
			// Peer sent a request without sending handshake first.
			break
		}
		extMsgID, ok := pe.ExtensionHandshake.M[peerprotocol.ExtensionKeyMetadata]
		if !ok {
			break
		}
		if t.info == nil {
			t.sendMetadataReject(pe, msg.Piece, extMsgID)
			break
		}
		start := 16 * 1024 * msg.Piece
		end := start + 16*1024
		totalSize := uint32(len(t.info.Bytes))
		if end > totalSize {
			end = totalSize
		}
		if start >= totalSize {
			t.sendMetadataReject(pe, msg.Piece, extMsgID)
			break
		}
		if end > totalSize {
			t.sendMetadataReject(pe, msg.Piece, extMsgID)
			break
		}
		data := t.info.Bytes[start:end]
		dataMsg := peerprotocol.ExtensionMetadataMessage{
			Type:      peerprotocol.ExtensionMetadataMessageTypeData,
			Piece:     msg.Piece,
			TotalSize: int(totalSize),
			Data:      data,
		}
		extDataMsg := peerprotocol.ExtensionMessage{
			ExtendedMessageID: extMsgID,
			Payload:           dataMsg,
		}
		pe.SendMessage(extDataMsg)
	case peerprotocol.ExtensionMetadataMessageTypeData:
		id, ok := t.infoDownloaders[pe]
		if !ok {
			break
		}
		err := id.GotBlock(msg.Piece, msg.Data)
		if err != nil {
			pe.Logger().Error(err)
			t.closePeer(pe)
			t.startInfoDownloaders()
			break
		}
		if !id.Done() {
			id.RequestBlocks(t.maxAllowedRequests(pe))
			pe.ResetSnubTimer()
			break
		}
		pe.StopSnubTimer()

		hash := sha1.New()                              // nolint: gosec
		_, _ = hash.Write(id.Bytes)                     // nolint: gosec
		if !bytes.Equal(hash.Sum(nil), t.infoHash[:]) { // nolint: gosec
			pe.Logger().Errorln("received info does not match with hash")
			t.closePeer(id.Peer.(*peer.Peer))
			t.startInfoDownloaders()
			break
		}
		t.stopInfoDownloaders()

		info, err := metainfo.NewInfo(id.Bytes)
		if err != nil {
			t.stop(fmt.Errorf("cannot parse info bytes: %s", err))
			break
		}
		if info.Private == 1 {
			t.stop(errors.New("private torrent from magnet"))
			break
		}
		t.info = info
		t.piecePool = bufferpool.New(int(info.PieceLength))
		err = t.session.resumer.WriteInfo(t.id, t.info.Bytes)
		if err != nil {
			t.stop(fmt.Errorf("cannot write resume info: %s", err))
			break
		}
		t.startAllocator()
	case peerprotocol.ExtensionMetadataMessageTypeReject:
		id, ok := t.infoDownloaders[pe]
		if ok {
			t.closePeer(id.Peer.(*peer.Peer))
			t.startInfoDownloaders()
		}
	}
}

func (t *torrent) sendMetadataReject(pe *peer.Peer, i uint32, msgID uint8) {
	dataMsg := peerprotocol.ExtensionMetadataMessage{
		Type:  peerprotocol.ExtensionMetadataMessageTypeReject,
		Piece: i,
	}
	extDataMsg := peerprotocol.ExtensionMessage{
		ExtendedMessageID: msgID,
		Payload:           &dataMsg,
	}
	pe.SendMessage(extDataMsg)
}
