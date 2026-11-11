package bt

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/ygelfand/libcountertop/pkg/bluetooth/sdp"
)

func TestMacServiceDiscovery(t *testing.T) {
	s := NewSink("Panel", 2048)
	raw, _ := hex.DecodeString("02000100083503190100003000")
	p, err := sdp.ParsePDU(raw)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := s.answer(p, 672)
	if err != nil {
		t.Fatal(err)
	}
	if reply.ID != sdp.PDUServiceSearchResponse || reply.Transaction != 1 {
		t.Fatalf("search: %x", reply.Marshal())
	}
	count := int(binary.BigEndian.Uint16(reply.Params[2:]))
	if count < 1 {
		t.Fatal("no audio services")
	}
	for i := 0; i < count; i++ {
		h := reply.Params[4+i*4 : 8+i*4]
		params := append(
			append([]byte{}, h...),
			0xff,
			0xff,
			0x35,
			0x05,
			0x0a,
			0x00,
			0x00,
			0xff,
			0xff,
			0,
		)
		attr, _ := s.answer(
			sdp.PDU{ID: sdp.PDUAttributeRequest, Transaction: 2, Params: params},
			672,
		)
		if attr.ID != sdp.PDUAttributeResponse {
			t.Fatalf("attributes: %x", attr.Marshal())
		}
		n := int(binary.BigEndian.Uint16(attr.Params))
		if _, _, err := sdp.ParseElement(attr.Params[2 : 2+n]); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSeparateDiscoveryRejectsBadInput(t *testing.T) {
	s := NewSink("Panel", 2048)
	for _, p := range []sdp.PDU{
		{ID: sdp.PDUServiceSearchRequest, Params: []byte{0x35}},
		{ID: sdp.PDUAttributeRequest, Params: []byte{0, 0}},
		{ID: sdp.PDUAttributeRequest, Params: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x35, 0, 0}},
	} {
		r, e := s.answer(p, 672)
		if e != nil || r.ID != sdp.PDUError {
			t.Fatalf("bad input: %v %x", e, r.Marshal())
		}
	}
}

func TestSeparateDiscoveryContinuation(t *testing.T) {
	s := NewSink("Panel", 2048)
	base := []byte{0x35, 3, 0x19, 1, 0, 0, 0xff}
	cont := []byte{0}
	var handles []byte
	for rounds := 0; ; rounds++ {
		if rounds > 10 {
			t.Fatal("search continuation did not finish")
		}
		p, _ := s.answer(
			sdp.PDU{
				ID:     sdp.PDUServiceSearchRequest,
				Params: append(append([]byte{}, base...), cont...),
			},
			16,
		)
		if p.ID != sdp.PDUServiceSearchResponse {
			t.Fatalf("search failed: %x", p.Marshal())
		}
		n := int(binary.BigEndian.Uint16(p.Params[2:])) * 4
		handles = append(handles, p.Params[4:4+n]...)
		cont = p.Params[4+n:]
		if cont[0] == 0 {
			break
		}
	}
	if len(handles) < 8 {
		t.Fatal("expected multiple service handles")
	}
	base[6] = 1
	limited, err := s.answer(sdp.PDU{ID: sdp.PDUServiceSearchRequest, Params: append(base, 0)}, 672)
	if err != nil || limited.ID != sdp.PDUServiceSearchResponse ||
		binary.BigEndian.Uint16(limited.Params) != 1 ||
		binary.BigEndian.Uint16(limited.Params[2:]) != 1 || limited.Params[8] != 0 {
		t.Fatalf("search ignored the total record limit: %x (%v)", limited.Marshal(), err)
	}
	base = append(append([]byte{}, handles[:4]...), 0, 7, 0x35, 5, 0x0a, 0, 0, 0xff, 0xff)
	cont = []byte{0}
	var body []byte
	for rounds := 0; ; rounds++ {
		if rounds > 100 {
			t.Fatal("attribute continuation did not finish")
		}
		p, _ := s.answer(
			sdp.PDU{
				ID:     sdp.PDUAttributeRequest,
				Params: append(append([]byte{}, base...), cont...),
			},
			672,
		)
		if p.ID != sdp.PDUAttributeResponse {
			t.Fatalf("attributes failed: %x", p.Marshal())
		}
		n := int(binary.BigEndian.Uint16(p.Params))
		if n > 7 {
			t.Fatal("attribute size limit ignored")
		}
		body = append(body, p.Params[2:2+n]...)
		cont = p.Params[2+n:]
		if cont[0] == 0 {
			break
		}
	}
	if _, _, err := sdp.ParseElement(body); err != nil {
		t.Fatal(err)
	}
}
