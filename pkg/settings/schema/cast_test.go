package schema

import "testing"

func TestMigrateLoungeKeepsTheScreensFromTheCastSection(t *testing.T) {
	old := `{"cast":{"youtube":{"device":"d","music":"m","video":"v","onDemand":true,"skip":["intro"]}}}`
	var got Lounge
	if err := MigrateLounge([]byte(old), &got); err != nil {
		t.Fatal(err)
	}
	if want := (Lounge{Device: "d", Music: "m", Video: "v", OnDemand: true}); got != want {
		t.Errorf("migrated to %+v, want %+v", got, want)
	}
}

func TestMigrateLoungeLeavesAMovedLoungeAlone(t *testing.T) {
	saved := `{"lounge":{"device":"new"},"cast":{"youtube":{"device":"old"}}}`
	got := Lounge{Device: "new"}
	if err := MigrateLounge([]byte(saved), &got); err != nil {
		t.Fatal(err)
	}
	if got.Device != "new" {
		t.Errorf("device is %q, want new", got.Device)
	}
}
