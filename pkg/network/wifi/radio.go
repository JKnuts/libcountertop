package wifi

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/ygelfand/libcountertop/pkg/hook"
	"github.com/ygelfand/libcountertop/pkg/network/wpa"
	"github.com/ygelfand/libcountertop/pkg/runtime/startup"
)

type Options struct {
	Interface  string
	SocketDir  string
	ConfigPath string
	Bgscan     string
	User       int
	Client     string
	Load       func() error
}

type Radio struct {
	Associated hook.Hook[bool]

	scanned hook.Hook[struct{}]

	opts     Options
	networks wpa.NetworksClient
	dials    atomic.Uint64

	mu   sync.Mutex
	mac  string
	up   bool
	ssid string

	configured      bool
	configuredKnown bool

	ctl *Control
}

func New(opts Options) *Radio {
	r := &Radio{opts: opts}
	r.networks = wpa.NetworksClient{Dial: func() (wpa.Connection, error) { return r.Dial() }}
	return r
}

func (r *Radio) Name() string { return "wifi" }

func (r *Radio) MAC() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.mac
}

func (r *Radio) Startup() startup.Progress {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := startup.Progress{Done: r.up}
	switch {
	case r.up:
		p.Doing = "on " + r.ssid
	case r.ssid != "":
		p.Doing = "joining " + r.ssid
	case r.configuredKnown && !r.configured:
		p.Failed, p.Doing = true, "no network set up"
	default:
		p.Doing = "looking for the network"
	}
	return p
}

func (r *Radio) Configured() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.configured
}

func (r *Radio) Network() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.up {
		return ""
	}
	return r.ssid
}

func (r *Radio) Start(context.Context) error {
	if err := r.load(); err != nil {
		return err
	}
	if err := linkUp(r.opts.Interface); err != nil {
		return err
	}

	mac, err := ReadMAC(r.opts.Interface)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.mac = mac
	r.mu.Unlock()

	return nil
}

func (r *Radio) Run(ctx context.Context) error {
	if err := r.awaitSupplicant(); err != nil {
		return err
	}

	if err := SetPowerSave(r.opts.Interface, false); err != nil {
		slog.Warn("could not turn power save off", "err", err)
	}

	if err := r.Connect(); err != nil {
		return err
	}

	ctl, err := r.Dial()
	if err != nil {
		return err
	}
	defer ctl.Close()

	r.mu.Lock()
	r.ctl = ctl
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.ctl = nil
		r.mu.Unlock()
		r.apply(false, "")
	}()

	return wpa.Monitor(ctx, eventSocket{ctl}, r.settle, func(e wpa.Event) {
		switch {
		case e.Is(wpa.EventConnected):
			r.connected()
		case e.Is(wpa.EventStateChange):
			if s, ok := wpa.ParseState(e.Text); ok {
				r.apply(s.Completed, s.SSID)
			}
		case e.Is(wpa.EventDisconnected):
			r.apply(false, "")
		case e.Is(wpa.EventTerminating):
			r.apply(false, "")
			slog.Warn("the supplicant is going away")
		case e.Is(wpa.EventScanResults):
			r.scanned.Emit(struct{}{})
		}
	})
}

// STATUS sends the supplicant through set_state, and set_state emits the state change again.
func (r *Radio) settle() {
	r.mu.Lock()
	ctl := r.ctl
	r.mu.Unlock()

	if ctl == nil {
		return
	}

	if known, err := wpa.Configured(wpa.Commands{Connection: ctl}); err == nil {
		r.mu.Lock()
		r.configured = len(known) > 0
		r.configuredKnown = true
		r.mu.Unlock()
	}

	s, err := ctl.Status()
	if err != nil {
		slog.Warn("could not read the supplicant's status", "err", err)
		return
	}
	r.apply(s["wpa_state"] == "COMPLETED", s["ssid"])
}

// Some vendor supplicants omit STATE-CHANGE, and their STATUS replies can emit CONNECTED again.
func (r *Radio) connected() {
	r.mu.Lock()
	ctl, ssid, already := r.ctl, r.ssid, r.up
	r.mu.Unlock()
	if already {
		return
	}
	r.apply(true, ssid)
	if ctl != nil {
		if status, err := ctl.Status(); err == nil && status["ssid"] != "" {
			r.apply(true, status["ssid"])
		}
	}
}

func (r *Radio) apply(up bool, ssid string) {
	r.mu.Lock()
	changed := up != r.up || ssid != r.ssid
	r.up, r.ssid = up, ssid
	if up {
		r.configured, r.configuredKnown = true, true
	}
	r.mu.Unlock()

	if !changed {
		return
	}
	if up {
		slog.Info("associated", "network", ssid)
	} else {
		slog.Warn("not associated")
	}
	r.Associated.Emit(up)
}
