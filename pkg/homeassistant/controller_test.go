package homeassistant

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ygelfand/go-esphome-device/api"
	"google.golang.org/protobuf/proto"

	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type fake struct {
	h      *Controller
	mu     sync.Mutex
	answer func(*api.HomeassistantActionRequest) *api.HomeassistantActionResponse
	fail   error
}

func (f *fake) Send(m proto.Message) error {
	if f.fail != nil {
		return f.fail
	}
	req := m.(*api.HomeassistantActionRequest)
	f.mu.Lock()
	answer := f.answer
	f.mu.Unlock()
	if answer == nil {
		return nil
	}
	if resp := answer(req); resp != nil {
		resp.CallId = req.GetCallId()
		go f.h.answer(f, resp)
	}
	return nil
}

func answering(data string) func(*api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
	return func(*api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
		return &api.HomeassistantActionResponse{Success: true, ResponseData: []byte(data)}
	}
}

func connected(
	answer func(*api.HomeassistantActionRequest) *api.HomeassistantActionResponse,
) (*Controller, *fake) {
	h := New(Settings{Home: testHomeSettings{}, API: testAPISettings{}})
	f := &fake{h: h, answer: answer}
	h.links[f] = &link{}
	return h, f
}

func set(h *Controller, f *fake, access Access) {
	h.mu.Lock()
	h.links[f].access = access
	h.judge()
	h.mu.Unlock()
}

func waitFor(t *testing.T, h *Controller, want Access) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for h.Access() != want {
		if time.Now().After(deadline) {
			t.Fatalf("access is %s, want %s", h.Access(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestProbeAllows(t *testing.T) {
	h, f := connected(answering(`{"response":"ok"}`))
	var heard []Access
	var mu sync.Mutex
	h.Changed.Listen(func(a Access) { mu.Lock(); heard = append(heard, a); mu.Unlock() })

	h.reprobe(f)
	waitFor(t, h, Allowed)

	mu.Lock()
	defer mu.Unlock()
	if len(heard) != 1 || heard[0] != Allowed {
		t.Errorf("heard %v, want [allowed]", heard)
	}
}

func TestThePingFindsTheIntegration(t *testing.T) {
	cases := []struct {
		name   string
		answer *api.HomeassistantActionResponse
		want   Integration
	}{
		{
			name: "ready",
			answer: &api.HomeassistantActionResponse{
				Success:      true,
				ResponseData: []byte(`{"response":"{\"supported\": true, \"protocol\": 1}"}`),
			},
			want: IntegrationReady,
		},
		{
			name: "outdated",
			answer: &api.HomeassistantActionResponse{
				Success:      true,
				ResponseData: []byte(`{"response":"{\"supported\": false, \"protocol\": 2}"}`),
			},
			want: IntegrationOutdated,
		},
		{
			name: "missing",
			answer: &api.HomeassistantActionResponse{
				ErrorMessage: "Action echolocal.ping not found",
			},
			want: IntegrationMissing,
		},
	}
	for _, c := range cases {
		var asked *api.HomeassistantActionRequest
		h, f := connected(
			func(req *api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
				asked = req
				return proto.Clone(c.answer).(*api.HomeassistantActionResponse)
			},
		)
		h.reprobe(f)
		waitFor(t, h, Allowed)
		if got := h.Integration(); got != c.want {
			t.Errorf("%s: integration %s, want %s", c.name, got, c.want)
		}
		if asked.GetService() != Ping || len(asked.GetData()) != 1 ||
			asked.GetData()[0].GetKey() != "protocol" {
			t.Errorf("%s: probed with %v", c.name, asked)
		}
	}
}

func TestTemplatesRideTheIntegrationWhenItIsThere(t *testing.T) {
	for _, c := range []struct {
		integration Integration
		want        string
	}{
		{IntegrationReady, Ping},
		{IntegrationOutdated, recorder},
		{IntegrationMissing, recorder},
	} {
		var asked string
		h, f := connected(
			func(req *api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
				asked = req.GetService()
				return &api.HomeassistantActionResponse{
					Success:      true,
					ResponseData: []byte(`{"response":"1"}`),
				}
			},
		)
		set(h, f, Allowed)
		h.mu.Lock()
		h.links[f].integration = c.integration
		h.mu.Unlock()
		if _, err := h.Template(context.Background(), "{{ 1 }}"); err != nil {
			t.Fatal(err)
		}
		if asked != c.want {
			t.Errorf("integration %s: the template rode %q, want %q", c.integration, asked, c.want)
		}
	}
}

func TestRefusedFailsFastAndReprobes(t *testing.T) {
	h, f := connected(answering(`{"response":"ok"}`))
	set(h, f, Refused)

	if err := h.Call(context.Background(), "light.turn_on", nil); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("a call while refused said %v, want ErrNotAllowed", err)
	}

	h.Probe()
	waitFor(t, h, Allowed)
}

func TestSendFailureDisconnects(t *testing.T) {
	h, f := connected(nil)
	set(h, f, Allowed)
	f.fail = errors.New("closed")
	heard := Allowed
	h.Changed.Listen(func(a Access) { heard = a })

	if err := h.Call(context.Background(), "light.turn_on", nil); err == nil {
		t.Fatal("a failed send reported success")
	}
	if err := h.Call(context.Background(), "light.turn_on", nil); !errors.Is(err, ErrNotConnected) {
		t.Errorf("the next call said %v, want ErrNotConnected", err)
	}
	if !h.Connected() || heard != Allowed {
		t.Errorf("a fresh drop showed connected=%v heard=%s, want it held", h.Connected(), heard)
	}
	h.mu.Lock()
	at := h.dropped
	h.mu.Unlock()
	h.expire(at)
	if h.Connected() {
		t.Error("still connected after the hold ran out")
	}
	if heard != Unknown {
		t.Errorf("heard %s after the hold, want unknown", heard)
	}
}

func TestAReconnectWithinTheHoldKeepsAccess(t *testing.T) {
	h, f := connected(answering(`{"response":"ok"}`))
	set(h, f, Allowed)
	var heard []Access
	var mu sync.Mutex
	h.Changed.Listen(func(a Access) { mu.Lock(); heard = append(heard, a); mu.Unlock() })

	h.lost(f)
	if !h.Connected() || h.Access() != Allowed {
		t.Fatalf("a dropped connection showed connected=%v access=%s", h.Connected(), h.Access())
	}
	h.attach(f)
	waitFor(t, h, Allowed)

	mu.Lock()
	defer mu.Unlock()
	if len(heard) != 0 {
		t.Errorf("a quick reconnect announced %v, want nothing", heard)
	}
}

func TestEntitiesSendsTheFilter(t *testing.T) {
	var sent string
	h, f := connected(func(req *api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
		sent = req.GetResponseTemplate()
		return &api.HomeassistantActionResponse{
			Success: true,
			ResponseData: []byte(
				`{"response":"{\"next\":null,\"items\":[{\"id\":\"light.a\",\"name\":\"A\",\"state\":\"on\",\"area\":\"\"}]}"}`,
			),
		}
	})
	set(h, f, Allowed)

	got, err := h.Entities(
		context.Background(),
		Filter{Domains: []string{"light"}, Labels: []string{"lanovo"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "light.a" {
		t.Errorf("got %+v", got)
	}
	for _, want := range []string{`selectattr('domain', 'in', ["light"])`, `select('in', ["lanovo"])`, `in []`} {
		if !strings.Contains(sent, want) {
			t.Errorf("the template lacks %q:\n%s", want, sent)
		}
	}
}

func TestStartup(t *testing.T) {
	born := time.Unix(1000, 0)
	early, late := born.Add(time.Second), born.Add(syncWait+time.Second)
	cases := []struct {
		name    string
		adopted bool
		to      bool
		access  Access
		at      time.Time
		done    bool
		failed  bool
	}{
		{name: "never adopted", at: early, done: true},
		{name: "not connected yet", adopted: true, at: early},
		{name: "checking access", adopted: true, to: true, at: early},
		{name: "allowed", adopted: true, to: true, access: Allowed, at: early, done: true},
		{name: "refused", adopted: true, to: true, access: Refused, at: early, failed: true},
		{name: "gave up", adopted: true, at: late, failed: true},
		{
			name:    "allowed after the wait",
			adopted: true,
			to:      true,
			access:  Allowed,
			at:      late,
			done:    true,
		},
	}
	for _, c := range cases {
		h := New(Settings{Home: testHomeSettings{}, API: testAPISettings{}})
		h.born, h.access = born, c.access
		if c.to {
			h.links[&fake{h: h}] = &link{access: c.access}
		}
		p := h.startup(c.adopted, c.at)
		if p.Done != c.done || p.Failed != c.failed {
			t.Errorf(
				"%s: done %v failed %v, want %v %v",
				c.name,
				p.Done,
				p.Failed,
				c.done,
				c.failed,
			)
		}
	}
}

type testHomeSettings struct{}

func (testHomeSettings) Read() schema.Home               { return schema.Home{Enabled: true} }
func (testHomeSettings) Update(func(*schema.Home)) error { return nil }

type testAPISettings struct{}

func (testAPISettings) Read() schema.API               { return schema.API{} }
func (testAPISettings) Update(func(*schema.API)) error { return nil }

func TestASecondHomeAssistantKeepsAccess(t *testing.T) {
	h, f := connected(answering(`{"response":"ok"}`))
	set(h, f, Allowed)
	var heard []Access
	var mu sync.Mutex
	h.Changed.Listen(func(a Access) { mu.Lock(); heard = append(heard, a); mu.Unlock() })

	h.attach(&fake{h: h})
	waitFor(t, h, Allowed)

	mu.Lock()
	defer mu.Unlock()
	if len(heard) != 0 {
		t.Errorf("a second connection announced %v, want nothing", heard)
	}
}

func TestOneAllowingHomeAssistantOpensTheGate(t *testing.T) {
	h, refusing := connected(nil)
	allowing := &fake{h: h, answer: answering(`{"response":"ok"}`)}
	h.links[allowing] = &link{access: Refused}
	set(h, refusing, Refused)
	if h.Access() != Refused {
		t.Fatalf("access is %s with every link refusing, want refused", h.Access())
	}

	var sent []*fake
	var mu sync.Mutex
	for _, f := range []*fake{refusing, allowing} {
		answer := f.answer
		f.answer = func(req *api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
			mu.Lock()
			sent = append(sent, f)
			mu.Unlock()
			if answer == nil {
				return nil
			}
			return answer(req)
		}
	}
	set(h, allowing, Allowed)
	if h.Access() != Allowed {
		t.Fatalf("access is %s with one link allowing, want allowed", h.Access())
	}
	if err := h.Call(context.Background(), "light.turn_on", nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	if len(sent) != 1 || sent[0] != allowing {
		t.Errorf("the call went to %d links, want only the allowing one", len(sent))
	}
	mu.Unlock()

	h.lost(allowing)
	if h.Access() != Allowed {
		t.Errorf("access is %s inside the hold, want allowed", h.Access())
	}
	h.mu.Lock()
	at := h.dropped
	h.mu.Unlock()
	h.expire(at)
	if h.Access() != Refused {
		t.Errorf("access is %s once only the refusing link is left, want refused", h.Access())
	}
}

func TestEveryAllowedHomeAssistantGetsTheCall(t *testing.T) {
	h, failing := connected(func(*api.HomeassistantActionRequest) *api.HomeassistantActionResponse {
		return &api.HomeassistantActionResponse{ErrorMessage: "no such entity"}
	})
	ok := &fake{h: h, answer: answering(`{"response":"on"}`)}
	h.links[ok] = &link{}
	set(h, failing, Allowed)
	set(h, ok, Allowed)

	got, err := h.Ask(context.Background(), recorder, nil, nil, "x")
	if err != nil {
		t.Fatalf("one failure beat a success: %v", err)
	}
	if string(got) != "on" {
		t.Errorf("got %s, want on", got)
	}

	h.lost(ok)
	if err := h.Call(context.Background(), "light.turn_on", nil); err == nil {
		t.Error("the only failure left reported success")
	}
}
