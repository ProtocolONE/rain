package torrent

import (
	"sort"

	"github.com/cenkalti/rain/torrent/internal/infodownloader"
	"github.com/cenkalti/rain/torrent/internal/peerprotocol"
	"github.com/cenkalti/rain/torrent/internal/piece"
	"github.com/cenkalti/rain/torrent/internal/piecedownloader"
)

func (t *Torrent) nextInfoDownload() *infodownloader.InfoDownloader {
	for pe := range t.peers {
		if _, ok := t.infoDownloaders[pe]; ok {
			continue
		}
		extID, ok := pe.ExtensionHandshake.M[peerprotocol.ExtensionMetadataKey]
		if !ok {
			continue
		}
		return infodownloader.New(pe, extID, pe.ExtensionHandshake.MetadataSize, t.infoDownloaderResultC)
	}
	return nil
}

func (t *Torrent) nextPieceDownload() *piecedownloader.PieceDownloader {
	// TODO request first 4 pieces randomly
	sort.Sort(piece.ByAvailability(t.sortedPieces))
	for _, p := range t.sortedPieces {
		if t.bitfield.Test(p.Index) {
			continue
		}
		if len(p.RequestedPeers) > 0 {
			continue
		}
		if p.Writing {
			continue
		}
		if len(p.HavingPeers) == 0 {
			continue
		}
		// prefer allowed fast peers first
		for pe := range p.HavingPeers {
			if _, ok := p.AllowedFastPeers[pe]; !ok {
				continue
			}
			if _, ok := t.pieceDownloaders[pe]; ok {
				continue
			}
			// TODO selecting first peer having the piece, change to more smart decision
			return piecedownloader.New(p.Piece, pe, t.pieceDownloaderResultC)
		}
		for pe := range p.HavingPeers {
			if pe.PeerChoking {
				continue
			}
			if _, ok := t.pieceDownloaders[pe]; ok {
				continue
			}
			// TODO selecting first peer having the piece, change to more smart decision
			return piecedownloader.New(p.Piece, pe, t.pieceDownloaderResultC)
		}
	}
	return nil
}
