package homecontrol

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	ha "github.com/ygelfand/libcountertop/pkg/homeassistant"
	"github.com/ygelfand/libcountertop/pkg/say"
	config "github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type Level int

const (
	LevelHome Level = iota
	LevelFloor
	LevelArea
	LevelKind
	LevelDevice
)

type Kind struct {
	Key     string
	Domains []string
	Classes []string
}

var kinds = []Kind{
	{Key: "lights", Domains: []string{"light"}},
	{Key: "switches", Domains: []string{"switch", "input_boolean"}},
	{Key: "media", Domains: []string{"media_player"}},
	{Key: "climate", Domains: []string{"climate", "water_heater"}},
	{Key: "fans", Domains: []string{"fan"}},
	{Key: "covers", Domains: []string{"cover"}},
	{Key: "locks", Domains: []string{"lock"}},
	{
		Key:     "access",
		Domains: []string{"binary_sensor"},
		Classes: []string{"door", "window", "garage_door", "opening"},
	},
	{
		Key:     "presence",
		Domains: []string{"binary_sensor"},
		Classes: []string{"motion", "occupancy", "presence"},
	},
	{Key: "cameras", Domains: []string{"camera"}},
}

func Kinds() []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = k.Key
	}
	return out
}

func KindName(key string) string { return say.T("rings.kind." + key) }

func kindOf(e ha.Entity) (string, bool) {
	domain := e.Domain()
	for _, k := range kinds {
		if !slices.Contains(k.Domains, domain) {
			continue
		}
		if len(k.Classes) > 0 && !slices.Contains(k.Classes, e.Attributes["device_class"]) {
			continue
		}
		return k.Key, true
	}
	return "", false
}

func ringsFilter() ha.Filter {
	var domains []string
	for _, k := range kinds {
		domains = append(domains, k.Domains...)
	}
	return ha.Filter{
		Domains: domains,
		Attributes: []string{
			"device_class", "brightness", "temperature", "current_temperature",
		},
	}
}

type Device struct {
	ID        string
	Name      string
	Kind      string
	State     string
	Active    bool
	Available bool
	Level     int
	Current   string
	Target    string
}

func (d Device) Domain() string { return ha.Entity{ID: d.ID}.Domain() }

type Ring struct {
	Key      string
	Name     string
	Level    Level
	Kind     string
	Hue      int
	Device   *Device
	Children []*Ring
	Devices  int
	Active   int
}

func (r *Ring) Find(key string) *Ring {
	if r == nil {
		return nil
	}
	if r.Key == key {
		return r
	}
	for _, c := range r.Children {
		if f := c.Find(key); f != nil {
			return f
		}
	}
	return nil
}

func (r *Ring) Path(key string) []*Ring {
	if r == nil {
		return nil
	}
	if r.Key == key {
		return []*Ring{r}
	}
	for _, c := range r.Children {
		if p := c.Path(key); p != nil {
			return append([]*Ring{r}, p...)
		}
	}
	return nil
}

func (r *Ring) Leaves() []*Device {
	if r.Device != nil {
		return []*Device{r.Device}
	}
	var out []*Device
	for _, c := range r.Children {
		out = append(out, c.Leaves()...)
	}
	return out
}

type Reading func(entity, attribute string) (string, bool)

func shown(cfg config.Rings, e ha.Entity, kind string) bool {
	if slices.Contains(cfg.Kinds, kind) || slices.Contains(cfg.Areas, areaKey(e.AreaID)) {
		return false
	}
	if len(cfg.Labels) == 0 {
		return true
	}
	return slices.ContainsFunc(
		e.Labels,
		func(l string) bool { return slices.Contains(cfg.Labels, l) },
	)
}

func BuildRings(entities []ha.Entity, cfg config.Rings, read Reading) *Ring {
	type placed struct {
		e    ha.Entity
		kind string
	}
	var keep []placed
	floors := map[string]bool{}
	for _, e := range entities {
		kind, ok := kindOf(e)
		if !ok || !shown(cfg, e, kind) {
			continue
		}
		keep = append(keep, placed{e, kind})
		if e.FloorID != "" {
			floors[e.FloorID] = true
		}
	}
	root := &Ring{Key: "home", Name: say.T("rings.home"), Level: LevelHome}
	order, levels := hues(entities), floorHues(entities)
	byFloor := len(floors) > 1
	for _, p := range keep {
		parent := root
		if byFloor {
			name := p.e.Floor
			if p.e.FloorID == "" {
				name = say.T("rings.nofloor")
			}
			parent = child(parent, "floor:"+p.e.FloorID, name, LevelFloor, "")
			parent.Hue = levels[p.e.FloorID]
		}
		area := p.e.Area
		if p.e.AreaID == "" {
			area = say.T("home.noarea")
		}
		parent = child(parent, parent.Key+"/area:"+p.e.AreaID, area, LevelArea, "")
		hue := order[areaKey(p.e.AreaID)]
		parent.Hue = hue
		parent = child(parent, parent.Key+"/kind:"+p.kind, KindName(p.kind), LevelKind, p.kind)
		parent.Hue = hue
		d := device(p.e, p.kind, read)
		parent.Children = append(parent.Children, &Ring{
			Key:    "device:" + d.ID,
			Name:   d.Name,
			Level:  LevelDevice,
			Kind:   p.kind,
			Hue:    hue,
			Device: &d,
		})
	}
	settle(root)
	return root
}

func hues(entities []ha.Entity) map[string]int {
	type spot struct{ id, floor, area string }
	seen := map[string]bool{}
	var spots []spot
	for _, e := range entities {
		if _, ok := kindOf(e); !ok || e.AreaID == "" || seen[e.AreaID] {
			continue
		}
		seen[e.AreaID] = true
		spots = append(spots, spot{e.AreaID, strings.ToLower(e.Floor), strings.ToLower(e.Area)})
	}
	slices.SortFunc(spots, func(a, b spot) int {
		return cmp.Or(
			cmp.Compare(blank(a.floor), blank(b.floor)),
			cmp.Compare(a.floor, b.floor),
			cmp.Compare(a.area, b.area),
			cmp.Compare(a.id, b.id),
		)
	})
	out := make(map[string]int, len(spots))
	for i, s := range spots {
		out[s.id] = i + 1
	}
	return out
}

func floorHues(entities []ha.Entity) map[string]int {
	type spot struct{ id, name string }
	seen := map[string]bool{}
	var spots []spot
	for _, e := range entities {
		if _, ok := kindOf(e); !ok || e.FloorID == "" || seen[e.FloorID] {
			continue
		}
		seen[e.FloorID] = true
		spots = append(spots, spot{e.FloorID, strings.ToLower(e.Floor)})
	}
	slices.SortFunc(spots, func(a, b spot) int {
		return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.id, b.id))
	})
	out := make(map[string]int, len(spots))
	for i, s := range spots {
		out[s.id] = i + 1
	}
	return out
}

func blank(s string) int {
	if s == "" {
		return 1
	}
	return 0
}

func child(parent *Ring, key, name string, level Level, kind string) *Ring {
	for _, c := range parent.Children {
		if c.Key == key {
			return c
		}
	}
	c := &Ring{Key: key, Name: name, Level: level, Kind: kind}
	parent.Children = append(parent.Children, c)
	return c
}

func settle(r *Ring) {
	if r.Device != nil {
		r.Devices = 1
		if r.Device.Active {
			r.Active = 1
		}
		return
	}
	r.Devices, r.Active = 0, 0
	for _, c := range r.Children {
		settle(c)
		r.Devices += c.Devices
		r.Active += c.Active
	}
	slices.SortStableFunc(r.Children, func(a, b *Ring) int {
		if a.Level == LevelKind {
			return cmp.Compare(slices.Index(Kinds(), a.Kind), slices.Index(Kinds(), b.Kind))
		}
		if last := cmp.Compare(unplaced(a), unplaced(b)); last != 0 {
			return last
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

func unplaced(r *Ring) int {
	if strings.HasSuffix(r.Key, ":") {
		return 1
	}
	return 0
}

func device(e ha.Entity, kind string, read Reading) Device {
	state := e.State
	if v, ok := read(e.ID, ""); ok {
		state = v
	}
	d := Device{
		ID:        e.ID,
		Name:      e.Name,
		Kind:      kind,
		State:     state,
		Available: state != "unavailable" && state != "unknown",
		Level:     -1,
	}
	if d.Name == "" {
		d.Name = e.ID
	}
	d.Active = active(kind, state)
	attr := func(name string) string {
		if v, ok := read(e.ID, name); ok {
			return v
		}
		return e.Attributes[name]
	}
	switch kind {
	case "lights":
		if n, err := strconv.ParseFloat(attr("brightness"), 64); err == nil && d.Active {
			d.Level = int(n*100/255 + 0.5)
		}
	case "climate":
		d.Current, d.Target = attr("current_temperature"), attr("temperature")
	}
	return d
}

func active(kind, state string) bool {
	switch kind {
	case "media":
		return state == "playing" || state == "on" || state == "paused" || state == "buffering"
	case "climate":
		return state != "off" && state != "unavailable" && state != "unknown"
	case "covers":
		return state == "open" || state == "opening" || state == "closing"
	case "locks":
		return state == "unlocked" || state == "unlocking" || state == "open"
	case "cameras":
		return state == "recording" || state == "streaming"
	}
	return state == "on"
}

type Place struct {
	ID   string
	Name string
}

func (e *Engine) Rings() (*Ring, bool, error) {
	got := e.cache.get(e.rings)
	if got.asking {
		return nil, true, nil
	}
	if got.err != nil {
		return nil, false, got.err
	}
	cfg := e.options.Settings.Read().Rings
	b := e.board
	return BuildRings(got.entities, cfg, func(entity, attribute string) (string, bool) {
		return b.value(entity, attribute).Get()
	}), false, nil
}

func (e *Engine) RingsLook() config.RingsLook { return e.options.Settings.Read().Rings.Shape() }

func (e *Engine) SetRingsLook(l config.RingsLook) { e.set(ringsLook, string(l)) }

func (e *Engine) RingsDepth() int { return e.options.Settings.Read().Rings.Deep() }

func (e *Engine) RingsSide() config.RingsSide { return e.options.Settings.Read().Rings.Placed() }

func (e *Engine) SetRingsSide(s config.RingsSide) { e.set(ringsSide, string(s)) }

func (e *Engine) SetRingsDepth(n int) { e.set(ringsDepth, strconv.Itoa(n)) }

func (e *Engine) RingAreas() ([]Place, bool) {
	got := e.cache.get(e.rings)
	if got.asking || got.err != nil {
		return nil, got.asking
	}
	seen := map[string]bool{}
	var out []Place
	loose := false
	for _, ent := range got.entities {
		if _, ok := kindOf(ent); !ok || seen[ent.AreaID] {
			continue
		}
		seen[ent.AreaID] = true
		if ent.AreaID == "" {
			loose = true
			continue
		}
		out = append(out, Place{ID: ent.AreaID, Name: ent.Area})
	}
	slices.SortFunc(out, func(a, b Place) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	if loose {
		out = append(out, Place{ID: NoArea, Name: say.T("home.noarea")})
	}
	return out, false
}

const NoArea = "-"

func areaKey(id string) string {
	if id == "" {
		return NoArea
	}
	return id
}

func (e *Engine) RingLabels() ([]Place, bool) {
	got := e.cache.get(e.rings)
	if got.asking || got.err != nil {
		return nil, got.asking
	}
	out := make([]Place, len(got.labels))
	for i, l := range got.labels {
		out[i] = Place{ID: l.ID, Name: l.Name}
	}
	return out, false
}

func (e *Engine) RingsShown() config.Rings { return e.options.Settings.Read().Rings.Clone() }

func (e *Engine) ShowRingArea(id string, on bool) {
	e.set(ringsAreas, strings.Join(config.Toggled(e.RingsShown().Areas, id, !on), ","))
}

func (e *Engine) ShowRingKind(key string, on bool) {
	e.set(ringsKinds, strings.Join(config.Toggled(e.RingsShown().Kinds, key, !on), ","))
}

func (e *Engine) PickRingLabel(id string, on bool) {
	e.set(ringsLabels, strings.Join(config.Toggled(e.RingsShown().Labels, id, on), ","))
}
