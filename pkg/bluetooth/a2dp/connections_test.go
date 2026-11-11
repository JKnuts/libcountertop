package a2dp

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/audio/background"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/l2cap"
	"github.com/ygelfand/libcountertop/pkg/bluetooth/pair"
)

func connectTestSources(t *testing.T) (*Sink, *connections) {
	t.Helper()
	host := testSink()
	host.pairs = pair.New(nil)
	links := &connections{host: host, name: "Display", peers: make(map[uint16]*connectedPeer)}
	host.links = links

	for handle, name := range []string{"Mac", "iPhone"} {
		id := uint16(handle + 1)
		addr := pair.Addr{byte(id), 2, 3, 4, 5, 6}
		params := binary.LittleEndian.AppendUint16([]byte{0}, id)
		params = append(params, addr[:]...)
		params = append(params, pair.LinkACL, 0)
		if _, err := links.Event(
			pair.Event{Code: pair.EventConnectionComplete, Params: params},
		); err != nil {
			t.Fatal(err)
		}
		params = append([]byte{0}, addr[:]...)
		params = append(params, name...)
		params = append(params, 0)
		if _, err := links.Event(
			pair.Event{Code: pair.EventRemoteName, Params: params},
		); err != nil {
			t.Fatal(err)
		}
	}
	return host, links
}

func TestTwoSourcesCanReuseTheSameL2CAPChannel(t *testing.T) {
	_, links := connectTestSources(t)
	pdu := l2cap.Signal(l2cap.Connect{PSM: l2cap.PSMSDP, SourceCID: 0x41}.Command(1)).Marshal()
	for _, handle := range []uint16{1, 2} {
		replies, err := links.Data(handle, pdu)
		if err != nil || len(replies) == 0 {
			t.Fatalf("handle %d: %v, replies %x", handle, err, replies)
		}
		reply, err := l2cap.ParseFrame(replies[0])
		if err != nil {
			t.Fatal(err)
		}
		commands, err := l2cap.ParseCommands(reply.Payload)
		if err != nil {
			t.Fatal(err)
		}
		got := commands[0]
		if got.Code != l2cap.CodeConnectResponse || len(got.Data) < 6 ||
			binary.LittleEndian.Uint16(got.Data[4:]) != 0 {
			t.Fatalf("handle %d was refused: %+v", handle, got)
		}
	}
}

func TestPlaybackSourceFollowsStartsUntilPinned(t *testing.T) {
	host, links := connectTestSources(t)
	mac, phone := links.peers[1], links.peers[2]
	arbiter := host.options.Arbitration.(*background.Arbiter)
	mac.sink.sink.Started(mac.sink.sink.Endpoint())
	phone.sink.sink.Started(phone.sink.sink.Endpoint())
	if host.selected() != phone.sink || arbiter.Playing() != phone.sink {
		t.Fatal("the new stream did not become the audible source")
	}
	if err := host.SelectConnection(mac.address.String()); err != nil {
		t.Fatal(err)
	}
	phone.sink.sink.Stopped(phone.sink.sink.Endpoint())
	phone.sink.sink.Started(phone.sink.sink.Endpoint())
	got, auto := host.Connections()
	if auto || len(got) != 2 || host.selected() != mac.sink || arbiter.Playing() != mac.sink {
		t.Fatalf("pin was lost or a connection was dropped: %+v (auto=%v)", got, auto)
	}
	if mac.sink.Label() != "Mac" || phone.sink.Label() != "iPhone" {
		t.Fatalf("peer names were mixed: %+v", got)
	}
	links.Gone(1)
	got, auto = host.Connections()
	if !auto || len(got) != 1 || host.selected() != phone.sink || arbiter.Playing() != phone.sink {
		t.Fatalf("disconnection did not fall back to the remaining source: %+v", got)
	}
}

func TestAutomaticHandoffWaitsForNewSourcesFirstPacket(t *testing.T) {
	host, links := connectTestSources(t)
	mac, phone := links.peers[1], links.peers[2]
	mac.sink.sink.Started(mac.sink.sink.Endpoint())
	phone.sink.sink.Started(phone.sink.sink.Endpoint())

	// A packet from the old source arrives before the new one's first packet.
	links.frame(mac.sink, nil)
	if host.selected() != phone.sink {
		t.Fatal("an old source packet stole playback before the new source could send audio")
	}

	// If the new source never sends media, the old live source can resume.
	phone.lastActivity = time.Now().Add(-2 * sourceIdle)
	links.frame(mac.sink, nil)
	if host.selected() != mac.sink {
		t.Fatal("an idle source kept playback from a source sending audio")
	}
}
