package bt

import (
	"encoding/binary"

	"github.com/ygelfand/libcountertop/pkg/bluetooth/sdp"
)

// Some hosts, including macOS, search for handles before reading attributes.
func (s *Sink) answerSeparate(p sdp.PDU, mtu int) sdp.PDU {
	bad := func(code uint16) sdp.PDU { return sdp.Error(p.Transaction, code) }
	var search sdp.Search
	var body []byte
	var total int
	var limit int
	if p.ID == sdp.PDUServiceSearchRequest {
		pattern, used, err := sdp.ParseElement(p.Params)
		if err != nil || len(p.Params) < used+3 || pattern.Type != sdp.TypeSequence ||
			len(pattern.Children) < 1 || len(pattern.Children) > 12 {
			return bad(sdp.ErrorBadSyntax)
		}
		maxCount := int(binary.BigEndian.Uint16(p.Params[used:]))
		if maxCount == 0 {
			return bad(sdp.ErrorBadSyntax)
		}
		for _, uuid := range pattern.Children {
			value, ok := uuid.Uint()
			if !ok || uuid.Type != sdp.TypeUUID {
				return bad(sdp.ErrorBadSyntax)
			}
			search.Pattern = append(search.Pattern, value)
		}
		state := p.Params[used+2:]
		if len(state) != 1+int(state[0]) {
			return bad(sdp.ErrorBadContinuation)
		}
		search.Continuation = state[1:]
		for _, r := range s.records {
			if !search.Wants(r.UUIDs()) {
				continue
			}
			h, ok := r.Attribute(sdp.AttrRecordHandle)
			if !ok {
				continue
			}
			v, ok := h.Uint()
			if !ok {
				continue
			}
			body = binary.BigEndian.AppendUint32(body, v)
			if len(body)/4 == maxCount {
				break
			}
		}
		total = len(body) / 4
		// Reserve space for the PDU header, both counts and the continuation.
		limit = (mtu - 5 - 4 - 3) / 4 * 4
	} else {
		if len(p.Params) < 7 {
			return bad(sdp.ErrorBadSyntax)
		}
		handle := binary.BigEndian.Uint32(p.Params)
		params := append([]byte{0x35, 0x00}, p.Params[4:]...)
		var err error
		search, err = sdp.ParseSearchAttribute(params)
		if err != nil || search.MaxBytes < 7 {
			return bad(sdp.ErrorBadSyntax)
		}
		var found bool
		for _, r := range s.records {
			h, ok := r.Attribute(sdp.AttrRecordHandle)
			if !ok {
				continue
			}
			v, ok := h.Uint()
			if !ok || v != handle {
				continue
			}
			found = true
			body, err = search.Selected(r).Element().Marshal()
			if err != nil {
				return bad(sdp.ErrorNoResources)
			}
			break
		}
		if !found {
			return bad(sdp.ErrorBadRecordHandle)
		}
		limit = min(int(search.MaxBytes), mtu-5-2-3)
	}
	if limit <= 0 {
		return bad(sdp.ErrorNoResources)
	}
	start := 0
	if len(search.Continuation) != 0 {
		if len(search.Continuation) != 2 {
			return bad(sdp.ErrorBadContinuation)
		}
		start = int(binary.BigEndian.Uint16(search.Continuation))
		if start == 0 || start >= len(body) ||
			(p.ID == sdp.PDUServiceSearchRequest && start%4 != 0) {
			return bad(sdp.ErrorBadContinuation)
		}
	}
	end := min(start+limit, len(body))
	var params []byte
	id := byte(sdp.PDUAttributeResponse)
	if p.ID == sdp.PDUServiceSearchRequest {
		id = sdp.PDUServiceSearchResponse
		params = binary.BigEndian.AppendUint16(params, uint16(total))
		params = binary.BigEndian.AppendUint16(params, uint16((end-start)/4))
	} else {
		params = binary.BigEndian.AppendUint16(params, uint16(end-start))
	}
	params = append(params, body[start:end]...)
	if end < len(body) {
		params = append(params, 2, byte(end>>8), byte(end))
	} else {
		params = append(params, 0)
	}
	return sdp.PDU{ID: id, Transaction: p.Transaction, Params: params}
}
