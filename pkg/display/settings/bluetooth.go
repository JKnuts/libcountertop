package settings

import (
	"log/slog"

	"github.com/ygelfand/libcountertop/pkg/bluetooth/a2dp"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func BluetoothSpeakerPage(sink *a2dp.Sink) *shell.Page {
	page := ServicePage(say.T("features.speaker"), sink.Controls())
	settings := page.Build
	page.Build = func() ([]menu.Row, []func(int)) {
		rows, actions := settings()
		sources, selectSource := bluetoothSourceRows(sink, nil)
		rows = append(
			rows,
			menu.Row{Label: say.T("bluetooth.connected_sources"), Kind: menu.Plain, Dim: true},
		)
		actions = append(actions, nil)
		return append(rows, sources...), append(actions, selectSource...)
	}
	return page
}

// BluetoothSourcesPage is the picker opened from the player.
func BluetoothSourcesPage(sink *a2dp.Sink, stack *shell.Shell) *shell.Page {
	page := &shell.Page{Title: say.T("bluetooth.connected_sources")}
	page.Build = func() ([]menu.Row, []func(int)) {
		return bluetoothSourceRows(sink, func() { stack.Remove(page) })
	}
	return page
}

func bluetoothSourceRows(sink *a2dp.Sink, selected func()) ([]menu.Row, []func(int)) {
	peers, auto := sink.Connections()
	if len(peers) == 0 {
		return []menu.Row{
			{Label: say.T("bluetooth.source_empty"), Kind: menu.Plain, Dim: true},
		}, []func(int){
			nil,
		}
	}
	choose := func(address string) func(int) {
		return func(int) {
			if err := sink.SelectConnection(address); err != nil {
				slog.Warn("could not select Bluetooth device", "err", err)
				return
			}
			if selected != nil {
				selected()
			}
		}
	}
	rows := []menu.Row{{Label: say.T("bluetooth.source_auto"), Kind: menu.Plain, Chosen: auto}}
	actions := []func(int){choose("")}
	for _, peer := range peers {
		row := menu.Row{Label: peer.Name, Kind: menu.Plain, Chosen: peer.Active && !auto}
		if peer.Active {
			row.Value = say.T("bluetooth.source_selected")
		}
		rows = append(rows, row)
		actions = append(actions, choose(peer.Address))
	}
	return rows, actions
}
