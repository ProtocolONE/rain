package torrent

import (
	"fmt"

	"github.com/ProtocolONE/rain/internal/allocator"
	"github.com/ProtocolONE/rain/internal/bitfield"
	"github.com/ProtocolONE/rain/internal/piece"
	"github.com/ProtocolONE/rain/internal/piecepicker"
)

func (t *torrent) handleAllocationDone(al *allocator.Allocator) {
	if t.allocator != al {
		panic("invalid allocator")
	}
	t.allocator = nil

	if al.Error != nil {
		t.stop(fmt.Errorf("file allocation error: %s", al.Error))
		return
	}

	if t.files != nil {
		panic("files exist")
	}
	t.files = al.Files

	if t.pieces != nil {
		panic("pieces exists")
	}
	t.pieces = piece.NewPieces(t.info, t.files)

	if t.piecePicker != nil {
		panic("piece picker exists")
	}
	t.piecePicker = piecepicker.New(t.pieces, t.session.config.EndgameMaxDuplicateDownloads, t.webseedSources)

	for pe := range t.peers {
		pe.Bitfield = bitfield.New(t.info.NumPieces)
	}

	// If we already have bitfield from resume db, skip verification and start downloading.
	if t.bitfield != nil {
		for i := uint32(0); i < t.bitfield.Len(); i++ {
			t.pieces[i].Done = t.bitfield.Test(i)
		}
		t.checkCompletion()
		t.processQueuedMessages()
		t.addFixedPeers()
		t.startAcceptor()
		t.startAnnouncers()
		t.startPieceDownloaders()
		return
	}

	// No need to verify files if they didn't exist when we create them.
	if !al.NeedHashCheck {
		t.mBitfield.Lock()
		t.bitfield = bitfield.New(t.info.NumPieces)
		t.mBitfield.Unlock()
		t.processQueuedMessages()
		t.addFixedPeers()
		t.startAcceptor()
		t.startAnnouncers()
		t.startPieceDownloaders()
		return
	}

	// Some files exists on the disk, need to verify pieces to create a correct bitfield.
	t.startVerifier()
}
