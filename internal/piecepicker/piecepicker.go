package piecepicker

import (
	"fmt"
	"sort"

	"github.com/ProtocolONE/rain/internal/peer"
	"github.com/ProtocolONE/rain/internal/peerset"
	"github.com/ProtocolONE/rain/internal/piece"
	"github.com/ProtocolONE/rain/internal/webseedsource"
)

/*

These are the things to consider when selecting a piece for downloading:

  * Piece is done (hash checked and written to disk)
  * Piece is writing
  * Peer has the piece
  * Peer is choking us
  * Piece is marked as allowed-fast
  * Piece is requested from another peers
  * Piece is reserved for downloading by a webseed source
  * Is endgame mode activated (all pieces are requested)
  * Are there stalled peers (snubbed or choked in the middle of download)

Do not forget to re-check these when making changes.

*/

type PiecePicker struct {
	webseedSources       []*webseedsource.WebseedSource
	pieces               []myPiece
	piecesByAvailability []*myPiece
	piecesByStalled      []*myPiece
	maxDuplicateDownload int
	available            uint32
	endgame              bool
}

type myPiece struct {
	*piece.Piece
	Having    peerset.PeerSet
	Requested peerset.PeerSet
	Snubbed   peerset.PeerSet
	Choked    peerset.PeerSet

	// Downloading from webseed source or marked to be downloaded later.
	RequestedWebseed *webseedsource.WebseedSource
}

func (p *myPiece) RunningDownloads() int {
	return p.Requested.Len() - p.Snubbed.Len() - p.Choked.Len()
}

func (p *myPiece) StalledDownloads() int {
	return p.Snubbed.Len() + p.Choked.Len()
}

func (p *myPiece) AvailableForWebseed(duplicate bool) bool {
	if p.Done || p.Writing || p.RequestedWebseed != nil {
		return false
	}
	if !duplicate {
		return p.RequestedWebseed != nil
	}
	return true
}

func New(pieces []piece.Piece, maxDuplicateDownload int, webseedSources []*webseedsource.WebseedSource) *PiecePicker {
	ps := make([]myPiece, len(pieces))
	for i := range pieces {
		ps[i] = myPiece{Piece: &pieces[i]}
	}
	sps := make([]*myPiece, len(ps))
	sps2 := make([]*myPiece, len(ps))
	for i := range sps {
		sps[i] = &ps[i]
		sps2[i] = &ps[i]
	}
	return &PiecePicker{
		pieces:               ps,
		piecesByAvailability: sps,
		piecesByStalled:      sps2,
		maxDuplicateDownload: maxDuplicateDownload,
		webseedSources:       webseedSources,
	}
}

func (p *PiecePicker) CloseWebseedDownloader(src *webseedsource.WebseedSource) {
	if src.Downloader == nil {
		return
	}
	for i := src.Downloader.Begin; i < src.Downloader.End; i++ {
		if p.pieces[i].RequestedWebseed != src {
			panic(fmt.Sprintf("invalid source in piece: %d", i))
		}
		p.pieces[i].RequestedWebseed = nil
	}
	src.Downloader.Close()
	src.Downloader = nil
	src.ResetSpeed()
}

func (p *PiecePicker) WebseedStopAt(src *webseedsource.WebseedSource, i uint32) (closed bool) {
	oldEnd := src.Downloader.End
	newEnd := i
	for i := newEnd; i < oldEnd; i++ {
		if p.pieces[i].RequestedWebseed != src {
			panic(fmt.Sprintf("invalid source in piece #%d: %s", i, p.pieces[i].RequestedWebseed.URL))
		}
		p.pieces[i].RequestedWebseed = nil
	}
	src.Downloader.UpdateEnd(newEnd)
	if src.Downloader.ReadCurrent() >= newEnd {
		p.CloseWebseedDownloader(src)
		return true
	}
	return false
}

func (p *PiecePicker) Available() uint32 {
	return p.available
}

func (p *PiecePicker) RequestedPeers(i uint32) []*peer.Peer {
	return p.pieces[i].Requested.Peers
}

func (p *PiecePicker) RequestedWebseedSource(i uint32) *webseedsource.WebseedSource {
	return p.pieces[i].RequestedWebseed
}

func (p *PiecePicker) HandleHave(pe *peer.Peer, i uint32) {
	pe.Bitfield.Set(i)
	p.addHavingPeer(i, pe)
}

func (p *PiecePicker) HandleAllowedFast(pe *peer.Peer, i uint32) {
	pe.AllowedFast.Add(p.pieces[i].Piece)
}

func (p *PiecePicker) HandleSnubbed(pe *peer.Peer, i uint32) {
	if p.pieces[i].Choked.Has(pe) {
		panic("peer snubbed while choked")
	}
	p.pieces[i].Snubbed.Add(pe)
}

func (p *PiecePicker) HandleChoke(pe *peer.Peer, i uint32) {
	p.pieces[i].Snubbed.Remove(pe)
	p.pieces[i].Choked.Add(pe)
}

func (p *PiecePicker) HandleUnchoke(pe *peer.Peer, i uint32) {
	p.pieces[i].Choked.Remove(pe)
}

func (p *PiecePicker) HandleCancelDownload(pe *peer.Peer, i uint32) {
	p.pieces[i].Requested.Remove(pe)
	p.pieces[i].Snubbed.Remove(pe)
}

func (p *PiecePicker) HandleDisconnect(pe *peer.Peer) {
	for i := range p.pieces {
		p.HandleCancelDownload(pe, uint32(i))
		p.removeHavingPeer(i, pe)
	}
}

func (p *PiecePicker) addHavingPeer(i uint32, pe *peer.Peer) {
	ok := p.pieces[i].Having.Add(pe)
	if ok && p.pieces[i].Having.Len() == 1 {
		p.available++
	}
}

func (p *PiecePicker) removeHavingPeer(i int, pe *peer.Peer) {
	ok := p.pieces[i].Having.Remove(pe)
	if ok && p.pieces[i].Having.Len() == 0 {
		p.available--
	}
}

func (p *PiecePicker) pickFor(pe *peer.Peer) *piece.Piece {
	pi, _ := p.PickFor(pe)
	return pi
}

func (p *PiecePicker) PickFor(pe *peer.Peer) (pp *piece.Piece, allowedFast bool) {
	pi, allowedFast := p.findPiece(pe)
	if pi == nil {
		return nil, false
	}
	pe.Snubbed = false
	pi.Requested.Add(pe)
	return pi.Piece, allowedFast
}

func (p *PiecePicker) findPiece(pe *peer.Peer) (mp *myPiece, allowedFast bool) {
	// Peer is allowed to download only one piece at a time
	if pe.Downloading {
		return nil, false
	}
	if p.downloadingWebseed() {
		if pe.PeerChoking {
			return nil, false
		}
		mp = p.pickLastPieceOfSmallestGap(pe)
		if mp != nil {
			return mp, pe.AllowedFast.Has(mp.Piece)
		}
		mp = p.peerStealsFromWebseed(pe)
		if mp != nil {
			return mp, pe.AllowedFast.Has(mp.Piece)
		}
		return nil, false
	}
	// Pick allowed fast piece
	pi := p.pickAllowedFast(pe)
	if pi != nil {
		return pi, true
	}
	// Must be unchoked to request a peer
	if pe.PeerChoking {
		return nil, false
	}
	// Short path for endgame mode.
	if p.endgame {
		return p.pickEndgame(pe), false
	}
	// Pieck rarest piece
	pi = p.pickRarest(pe)
	if pi != nil {
		return pi, false
	}
	// Check if endgame mode is activated
	if p.endgame {
		return p.pickEndgame(pe), false
	}
	// Re-request stalled downloads
	return p.pickStalled(pe), false
}

func (p *PiecePicker) pickAllowedFast(pe *peer.Peer) *myPiece {
	for _, pi := range pe.AllowedFast.Pieces {
		mp := &p.pieces[pi.Index]
		if mp.Done || mp.Writing {
			continue
		}
		if mp.Requested.Len() == 0 && mp.Having.Has(pe) {
			return mp
		}
	}
	return nil
}

func (p *PiecePicker) pickRarest(pe *peer.Peer) *myPiece {
	// Sort by rarity
	sort.Slice(p.piecesByAvailability, func(i, j int) bool {
		return len(p.piecesByAvailability[i].Having.Peers) < len(p.piecesByAvailability[j].Having.Peers)
	})
	var picked *myPiece
	var hasUnrequested bool
	// Select unrequested piece
	for _, mp := range p.piecesByAvailability {
		if mp.Done || mp.Writing {
			continue
		}
		if mp.Requested.Len() == 0 && mp.Having.Has(pe) {
			picked = mp
			break
		}
		if mp.Requested.Len() == 0 {
			hasUnrequested = true
		}
	}
	if picked == nil && !hasUnrequested {
		p.endgame = true
	}
	return picked
}

func (p *PiecePicker) pickEndgame(pe *peer.Peer) *myPiece {
	// Sort by request count
	sort.Slice(p.piecesByAvailability, func(i, j int) bool {
		return p.piecesByAvailability[i].RunningDownloads() < p.piecesByAvailability[j].RunningDownloads()
	})
	// Select unrequested piece
	for _, mp := range p.piecesByAvailability {
		if mp.Done || mp.Writing {
			continue
		}
		if mp.Requested.Len() < p.maxDuplicateDownload && mp.Having.Has(pe) {
			return mp
		}
	}
	return nil
}

func (p *PiecePicker) pickStalled(pe *peer.Peer) *myPiece {
	// Sort by request count
	sort.Slice(p.piecesByStalled, func(i, j int) bool {
		return p.piecesByStalled[i].StalledDownloads() < p.piecesByStalled[j].StalledDownloads()
	})
	// Select unrequested piece
	for _, mp := range p.piecesByStalled {
		if mp.Done || mp.Writing {
			continue
		}
		if mp.RunningDownloads() > 0 {
			continue
		}
		if mp.Requested.Len() < p.maxDuplicateDownload && mp.Having.Has(pe) {
			return mp
		}
	}
	return nil
}

type Range struct {
	Begin, End uint32
}

func (r Range) Len() uint32 {
	return r.End - r.Begin
}
