package torrent

import "testing"

func TestTorrent(t *testing.T) {
	tor, err := New("test_files/ubuntu-14.04-server-amd64.iso.torrent")
	if err != nil {
		t.Fatal(err)
	}

	if tor.Info.Name != "ubuntu-14.04-server-amd64.iso" {
		t.Errorf("invalid name: %q", tor.Info.Name)
	}

	if tor.Info.Length != 591396864 {
		t.Errorf("invalid length: %d", tor.Info.Length)
	}

	if tor.Info.Hash.String() != "757b25d9681d493167b8d3759dbfddc983e80646" {
		t.Errorf("invalid info hash: %q must be '757b25d9681d493167b8d3759dbfddc983e80646'", tor.Info.Hash)
	}
}