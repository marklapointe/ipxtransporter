// SPDX-License-Identifier: BSD-3-Clause
// IPXTransporter – Author: Mark LaPointe <mark@cloudbsd.org>
// Terminal UI for live monitoring

package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mlapointe/ipxtransporter/internal/capture"
	"github.com/mlapointe/ipxtransporter/internal/config"
	"github.com/mlapointe/ipxtransporter/internal/stats"
	"github.com/rivo/tview"
)

type TUI struct {
	app           *tview.Application
	pages         *tview.Pages
	mainFlex      *tview.Flex
	table         *tview.Table
	mapView       *tview.TextView
	graphView     *tview.TextView
	statCards     *tview.TextView
	statsFunc     func() stats.Stats
	cfg           *config.Config
	configPath    string
	fileList      *tview.List
	currentDir    string
	rxHistory     []uint64
	txHistory     []uint64
	graphStep     int // Number of 500ms intervals per column
	onDemoUpdate  func(packetRate, dropRate, errorRate, numPeers int)
	onDisconnect  func(id string)
	onBan         func(id, ip string)
	lastClickTime time.Time
	lastClickRow  int
}

func NewTUI(statsFunc func() stats.Stats, cfg *config.Config, configPath string) *TUI {
	return NewTUIWithDemo(statsFunc, cfg, configPath, nil, nil, nil)
}

func NewTUIWithDemo(statsFunc func() stats.Stats, cfg *config.Config, configPath string, onDemoUpdate func(packetRate, dropRate, errorRate, numPeers int), onDisconnect func(id string), onBan func(id, ip string)) *TUI {
	app := tview.NewApplication()
	pages := tview.NewPages()

	table := tview.NewTable().
		SetFixed(1, 1).
		SetSelectable(true, false).
		SetSeparator(tview.Borders.Vertical)

	statCards := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)

	mapView := tview.NewTextView()
	mapView.SetDynamicColors(true).
		SetWrap(false).
		SetTitle("Network Topology").
		SetBorder(true)

	graphView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	graphView.SetBorder(true).SetTitle("Traffic Graph (Last 60s)")

	tuiInstance := &TUI{
		app:          app,
		pages:        pages,
		table:        table,
		mapView:      mapView,
		graphView:    graphView,
		statCards:    statCards,
		statsFunc:    statsFunc,
		cfg:          cfg,
		configPath:   configPath,
		rxHistory:    make([]uint64, 0, 7200), // Store up to 1 hour (3600s / 0.5s)
		txHistory:    make([]uint64, 0, 7200),
		graphStep:    1, // Default to 500ms per column
		onDemoUpdate: onDemoUpdate,
		onDisconnect: onDisconnect,
		onBan:        onBan,
	}

	table.SetSelectedFunc(func(row, column int) {
		tuiInstance.showPeerActions(row)
	})

	app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if action == tview.MouseLeftClick {
			row, _ := table.GetSelection()
			now := time.Now()
			if row == tuiInstance.lastClickRow && now.Sub(tuiInstance.lastClickTime) < 500*time.Millisecond {
				// Double click detected
				tuiInstance.showPeerActions(row)
				tuiInstance.lastClickRow = -1 // Reset
			} else {
				tuiInstance.lastClickRow = row
				tuiInstance.lastClickTime = now
			}
		}
		return event, action
	})

	mainFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(tview.NewFlex().
			AddItem(table, 0, 1, true).
			AddItem(mapView, 66, 0, false), 0, 1, true).
		AddItem(graphView, 10, 0, false).
		AddItem(statCards, 2, 1, false)

	tuiInstance.mainFlex = mainFlex
	pages.AddPage("main", mainFlex, true, true)

	app.SetRoot(pages, true).SetFocus(table).EnableMouse(true)

	// Add global shortcuts
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			app.Stop()
			return nil
		}
		if event.Key() == tcell.KeyF1 {
			tuiInstance.showConfigEditor()
			return nil
		}
		if event.Key() == tcell.KeyF2 {
			tuiInstance.showInterfaceSelection()
			return nil
		}
		if event.Key() == tcell.KeyF3 {
			tuiInstance.showWhois()
			return nil
		}
		if event.Key() == tcell.KeyF4 {
			tuiInstance.showSettings()
			return nil
		}
		if event.Key() == tcell.KeyF5 && tuiInstance.statsFunc().DemoProps != nil {
			tuiInstance.showDemoSettings()
			return nil
		}
		if event.Rune() == '+' || event.Key() == tcell.KeyRight {
			tuiInstance.zoomGraph(-1)
			return nil
		}
		if event.Rune() == '-' || event.Key() == tcell.KeyLeft {
			tuiInstance.zoomGraph(1)
			return nil
		}
		return event
	})

	return tuiInstance
}

func (t *TUI) Run(ctx context.Context) error {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				t.app.Stop()
				return
			case <-ticker.C:
				t.app.QueueUpdateDraw(func() {
					t.update()
				})
			}
		}
	}()

	return t.app.Run()
}

func (t *TUI) update() {
	s := t.statsFunc()

	// Update stat cards
	errorMsg := ""
	if s.CaptureError != "" {
		errorMsg = fmt.Sprintf("  [red]Capture Error: %s", s.CaptureError)
	}

	demoKey := ""
	if s.DemoProps != nil {
		demoKey = "F5: Demo  "
	}

	listenInfo := ""
	if s.ListenAddr != "" {
		listenInfo = fmt.Sprintf("  [blue]Listen: %s", s.ListenAddr)
	}

	t.statCards.SetText(fmt.Sprintf(
		"[yellow]RX: [white]%-10s [yellow]TX: [white]%-10s [yellow]Drop: [white]%-10s [yellow]Err: [white]%-10s [yellow]Up: [white]%-10s%s%s\n[blue]F1: Config  F2: Iface  F3: Whois  F4: Settings  %s+/-: Zoom  Enter: Actions  Ctrl+C: Exit",
		formatPkts(s.TotalReceived), formatPkts(s.TotalForwarded), formatPkts(s.TotalDropped), formatPkts(s.TotalErrors), s.UptimeStr, errorMsg, listenInfo, demoKey,
	))

	// Update Graph
	t.updateGraph(s)

	// Update Map
	t.drawMap(s.Peers)

	// Update table
	t.table.Clear()
	headers := []string{"ID", "IP", "Hostname", "Connected", "Last Seen", "Sent", "Recv", "Sent (Pkts)", "Recv (Pkts)", "Errors"}
	for i, h := range headers {
		t.table.SetCell(0, i, tview.NewTableCell(h).SetTextColor(tcell.ColorYellow).SetSelectable(false))
	}

	// s.SortPeers() is now called in CollectStats()
	for i, p := range s.Peers {
		row := i + 1
		color := tcell.ColorWhite
		if time.Since(p.LastSeen) > 10*time.Second {
			color = tcell.ColorRed
		} else {
			color = tcell.ColorGreen
		}

		t.table.SetCell(row, 0, tview.NewTableCell(p.ID).SetTextColor(color))
		t.table.SetCell(row, 1, tview.NewTableCell(p.IP.String()).SetTextColor(color))
		t.table.SetCell(row, 2, tview.NewTableCell(p.Hostname).SetTextColor(color))
		t.table.SetCell(row, 3, tview.NewTableCell(p.ConnectedAt.Format("15:04:05")).SetTextColor(color))
		t.table.SetCell(row, 4, tview.NewTableCell(time.Since(p.LastSeen).Round(time.Second).String()).SetTextColor(color))
		t.table.SetCell(row, 5, tview.NewTableCell(formatBytes(p.SentBytes)).SetTextColor(color))
		t.table.SetCell(row, 6, tview.NewTableCell(formatBytes(p.RecvBytes)).SetTextColor(color))
		t.table.SetCell(row, 7, tview.NewTableCell(formatPkts(p.SentPkts)).SetTextColor(color))
		t.table.SetCell(row, 8, tview.NewTableCell(formatPkts(p.RecvPkts)).SetTextColor(color))
		t.table.SetCell(row, 9, tview.NewTableCell(formatPkts(p.Errors)).SetTextColor(color))
	}
}

func (t *TUI) updateGraph(s stats.Stats) {
	t.rxHistory = append(t.rxHistory, s.TotalReceived)
	t.txHistory = append(t.txHistory, s.TotalForwarded)
	if len(t.rxHistory) > 7200 {
		t.rxHistory = t.rxHistory[1:]
		t.txHistory = t.txHistory[1:]
	}

	if len(t.rxHistory) < 2 {
		return
	}

	_, _, width, height := t.graphView.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	// Calculate points per column based on graphStep
	// Each point in history is 500ms
	// graphStep 1 = 500ms per column
	// graphStep 2 = 1s per column
	// etc.
	pointsNeeded := width * t.graphStep
	if pointsNeeded < 120 { // Ensure we always try to show at least 60s if possible (120 points)
		pointsNeeded = 120
	}

	// Calculate actual rates to display
	numCols := width
	displayRX := make([]uint64, numCols)
	displayTX := make([]uint64, numCols)
	var maxRate uint64 = 1

	for i := 0; i < numCols; i++ {
		// Calculate the range of history indices for this column
		// We work backwards from the end
		endIdx := len(t.rxHistory) - 1 - (numCols-1-i)*t.graphStep
		startIdx := endIdx - t.graphStep

		if startIdx < 0 {
			continue
		}

		// Sum the rates in this interval
		var rxSum, txSum uint64
		for j := startIdx; j < endIdx; j++ {
			rxSum += t.rxHistory[j+1] - t.rxHistory[j]
			txSum += t.txHistory[j+1] - t.txHistory[j]
		}
		displayRX[i] = rxSum
		displayTX[i] = txSum

		if rxSum > maxRate {
			maxRate = rxSum
		}
		if txSum > maxRate {
			maxRate = txSum
		}
	}

	// Update title with time range
	timeRange := time.Duration(numCols*t.graphStep) * 500 * time.Millisecond
	t.graphView.SetTitle(fmt.Sprintf("Traffic Graph (Last %v)", timeRange.Round(time.Second)))

	// Plot graph
	graph := ""
	for h := height - 1; h >= 0; h-- {
		line := ""
		for i := 0; i < numCols; i++ {
			rxVal := displayRX[i]
			txVal := displayTX[i]

			if rxVal == 0 && txVal == 0 {
				line += " "
				continue
			}

			rxLevel := int(rxVal * uint64(height) / maxRate)
			txLevel := int(txVal * uint64(height) / maxRate)

			char := " "
			color := ""
			if h < rxLevel && h < txLevel {
				char = "•"
				if rxVal+txVal > maxRate*2/3 {
					color = "magenta"
				} else {
					color = "darkmagenta"
				}
			} else if h < rxLevel {
				char = "•"
				if rxVal > maxRate*2/3 {
					color = "green"
				} else {
					color = "darkgreen"
				}
			} else if h < txLevel {
				char = "•"
				if txVal > maxRate*2/3 {
					color = "blue"
				} else {
					color = "darkblue"
				}
			}

			if color != "" {
				line += fmt.Sprintf("[%s]%s[-]", color, char)
			} else {
				line += " "
			}
		}
		graph += line + "\n"
	}
	t.graphView.SetText(graph)
}

func (t *TUI) zoomGraph(delta int) {
	t.graphStep += delta
	if t.graphStep < 1 {
		t.graphStep = 1
	}
	if t.graphStep > 120 { // Max 1 minute per column (1 hour total view approx if width is 60)
		t.graphStep = 120
	}
	t.app.QueueUpdateDraw(func() {
		t.update()
	})
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatPkts(p uint64) string {
	if p < 1000 {
		return fmt.Sprintf("%d", p)
	}
	if p < 1000000 {
		return fmt.Sprintf("%.1fK", float64(p)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(p)/1000000)
}

func (t *TUI) showInterfaceSelection() {
	ifaces, err := capture.ListInterfaces()
	if err != nil {
		t.showError("Failed to list interfaces: " + err.Error())
		return
	}

	list := tview.NewList()
	for _, iface := range ifaces {
		name := iface
		list.AddItem(name, "", 0, func() {
			t.cfg.Interface = name
			t.pages.RemovePage("iface_select")
			t.showError("Interface set to " + name + ". Restart required for changes to take effect.")
		})
	}
	list.AddItem("Cancel", "Go back", 'c', func() {
		t.pages.RemovePage("iface_select")
	})

	list.SetBorder(true).SetTitle("Select Interface")
	t.pages.AddPage("iface_select", t.center(list, 40, 20), true, true)
}

func (t *TUI) showConfigEditor() {
	form := tview.NewForm().
		AddInputField("Interface", t.cfg.Interface, 20, nil, func(text string) { t.cfg.Interface = text }).
		AddInputField("Listen Addr", t.cfg.ListenAddr, 20, nil, func(text string) { t.cfg.ListenAddr = text }).
		AddInputField("HTTP Listen", t.cfg.HTTPListenAddr, 20, nil, func(text string) { t.cfg.HTTPListenAddr = text }).
		AddCheckbox("Enable HTTP", t.cfg.EnableHTTP, func(checked bool) { t.cfg.EnableHTTP = checked }).
		AddCheckbox("Disable SSL", t.cfg.DisableSSL, func(checked bool) { t.cfg.DisableSSL = checked }).
		AddPasswordField("Admin Password", t.cfg.AdminPass, 20, '*', func(text string) { t.cfg.AdminPass = text }).
		AddInputField("Max Children", fmt.Sprintf("%d", t.cfg.MaxChildren), 5, tview.InputFieldInteger, func(text string) {
			fmt.Sscanf(text, "%d", &t.cfg.MaxChildren)
		}).
		AddButton("Save", func() {
			t.showSaveDialog()
		}).
		AddButton("Cancel", func() {
			t.pages.RemovePage("config_editor")
		})

	form.SetBorder(true).SetTitle("Edit Configuration")
	t.pages.AddPage("config_editor", t.center(form, 60, 15), true, true)
}

func (t *TUI) showSaveDialog() {
	modal := tview.NewModal().
		SetText("Where do you want to save the configuration?").
		AddButtons([]string{"System Wide (" + t.configPath + ")", "Other Location", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonIndex == 0 {
				err := config.SaveConfig(t.configPath, t.cfg)
				if err != nil {
					t.showError("Failed to save: " + err.Error())
				} else {
					t.pages.RemovePage("save_dialog")
					t.pages.RemovePage("config_editor")
				}
			} else if buttonIndex == 1 {
				t.pages.RemovePage("save_dialog")
				t.showFileBrowser()
			} else {
				t.pages.RemovePage("save_dialog")
			}
		})
	t.pages.AddPage("save_dialog", modal, true, true)
}

func (t *TUI) showFileBrowser() {
	cwd, _ := os.Getwd()

	t.fileList = tview.NewList().SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		path := secondaryText
		info, err := os.Stat(path)
		if err != nil {
			return
		}
		if info.IsDir() {
			t.updateFileBrowser(path)
		} else {
			// Select this file
			err := config.SaveConfig(path, t.cfg)
			if err != nil {
				t.showError("Failed to save: " + err.Error())
			} else {
				t.pages.RemovePage("file_browser")
				t.pages.RemovePage("config_editor")
			}
		}
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	header := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("Select directory to browse or file to overwrite")

	flex.AddItem(header, 1, 0, false)
	flex.AddItem(t.fileList, 0, 1, true)

	footer := tview.NewForm().AddButton("Save in current dir", func() {
		t.showFilenamePrompt(t.currentDir)
	}).AddButton("Cancel", func() {
		t.pages.RemovePage("file_browser")
	})
	flex.AddItem(footer, 3, 0, false)

	flex.SetBorder(true).SetTitle("File Browser")
	t.pages.AddPage("file_browser", t.center(flex, 80, 24), true, true)
	t.updateFileBrowser(cwd)
}

func (t *TUI) updateFileBrowser(path string) {
	t.currentDir = path
	t.fileList.Clear()

	// Add ".." entry
	parent := filepath.Dir(path)
	t.fileList.AddItem("..", parent, 'u', nil)

	files, err := os.ReadDir(path)
	if err != nil {
		t.showError("Failed to read dir: " + err.Error())
		return
	}

	// Sort: dirs first, then files
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir() != files[j].IsDir() {
			return files[i].IsDir()
		}
		return files[i].Name() < files[j].Name()
	})

	for _, f := range files {
		icon := " "
		if f.IsDir() {
			icon = "D"
		} else {
			icon = "F"
		}
		t.fileList.AddItem(fmt.Sprintf("[%s] %s", icon, f.Name()), filepath.Join(path, f.Name()), 0, nil)
	}
}

func (t *TUI) showFilenamePrompt(dir string) {
	form := tview.NewForm().
		AddInputField("Filename", "ipxtransporter.json", 30, nil, nil)

	form.AddButton("Save", func() {
		filename := form.GetFormItem(0).(*tview.InputField).GetText()
		path := filepath.Join(dir, filename)
		err := config.SaveConfig(path, t.cfg)
		if err != nil {
			t.showError("Failed to save: " + err.Error())
		} else {
			t.pages.RemovePage("filename_prompt")
			t.pages.RemovePage("file_browser")
			t.pages.RemovePage("config_editor")
		}
	}).
		AddButton("Cancel", func() {
			t.pages.RemovePage("filename_prompt")
		})
	form.SetBorder(true).SetTitle("Enter Filename")
	t.pages.AddPage("filename_prompt", t.center(form, 40, 7), true, true)
}

func (t *TUI) showError(msg string) {
	modal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			t.pages.RemovePage("error")
		})
	t.pages.AddPage("error", modal, true, true)
}

func (t *TUI) showWhois() {
	row, _ := t.table.GetSelection()
	if row <= 0 {
		return
	}
	s := t.statsFunc()
	if row > len(s.Peers) {
		return
	}
	// Peers were sorted by ID in update()
	sort.Slice(s.Peers, func(i, j int) bool {
		return s.Peers[i].ID < s.Peers[j].ID
	})
	p := s.Peers[row-1]

	childConsumption := 0.0
	if p.MaxChildren > 0 {
		childConsumption = float64(p.NumChildren) / float64(p.MaxChildren) * 100
	}

	whoisText := fmt.Sprintf("ID: %s\nIP: %s\nLocation: %s, %s\nLat/Lon: %.2f, %.2f\n\nConnections: %d/%d (%.1f%%)\n\n%s",
		p.ID, p.IP, p.City, p.Country, p.Lat, p.Lon, p.NumChildren, p.MaxChildren, childConsumption, p.Whois)

	modal := tview.NewModal().
		SetText(whoisText).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			t.pages.RemovePage("whois")
		})

	t.pages.AddPage("whois", modal, true, true)
}

func (t *TUI) showSettings() {
	options := []string{"id", "ip", "hostname", "connected", "last_seen", "children", "sent_bytes", "recv_bytes", "sent_pkts", "recv_pkts", "errors"}
	currentIndex := 0
	for i, opt := range options {
		if opt == t.cfg.SortField {
			currentIndex = i
			break
		}
	}

	form := tview.NewForm().
		AddDropDown("Sort By", options, currentIndex, func(option string, optionIndex int) {
			t.cfg.SortField = option
		}).
		AddCheckbox("Reverse Sort", t.cfg.SortReverse, func(checked bool) {
			t.cfg.SortReverse = checked
		}).
		AddButton("Close", func() {
			t.pages.RemovePage("settings")
		})

	form.SetBorder(true).SetTitle("UI Settings")
	t.pages.AddPage("settings", t.center(form, 40, 10), true, true)
}

func (t *TUI) showDemoSettings() {
	s := t.statsFunc()
	if s.DemoProps == nil || t.onDemoUpdate == nil {
		return
	}

	packetRate := s.DemoProps.PacketRate
	dropRate := s.DemoProps.DropRate
	errorRate := s.DemoProps.ErrorRate
	numPeers := s.DemoProps.NumPeers

	form := tview.NewForm().
		AddInputField("Packet Rate", fmt.Sprintf("%d", packetRate), 5, tview.InputFieldInteger, func(text string) {
			fmt.Sscanf(text, "%d", &packetRate)
		}).
		AddInputField("Drop Rate", fmt.Sprintf("%d", dropRate), 5, tview.InputFieldInteger, func(text string) {
			fmt.Sscanf(text, "%d", &dropRate)
		}).
		AddInputField("Error Rate", fmt.Sprintf("%d", errorRate), 5, tview.InputFieldInteger, func(text string) {
			fmt.Sscanf(text, "%d", &errorRate)
		}).
		AddInputField("Num Peers", fmt.Sprintf("%d", numPeers), 5, tview.InputFieldInteger, func(text string) {
			fmt.Sscanf(text, "%d", &numPeers)
		}).
		AddButton("Apply", func() {
			t.onDemoUpdate(packetRate, dropRate, errorRate, numPeers)
			t.pages.RemovePage("demo_settings")
		}).
		AddButton("Cancel", func() {
			t.pages.RemovePage("demo_settings")
		})

	form.SetBorder(true).SetTitle("Demo Mode Settings")
	t.pages.AddPage("demo_settings", t.center(form, 40, 15), true, true)
}

func (t *TUI) showPeerActions(row int) {
	if row <= 0 {
		return
	}
	s := t.statsFunc()
	if row > len(s.Peers) {
		return
	}

	p := s.Peers[row-1]

	list := tview.NewList()
	list.AddItem("Disconnect", "Close connection", 'd', func() {
		if t.onDisconnect != nil {
			t.onDisconnect(p.ID)
		}
		t.pages.RemovePage("peer_actions")
	})
	list.AddItem("Ban Host & ID", "Disconnect and ban forever", 'b', func() {
		if t.onBan != nil {
			t.onBan(p.ID, p.IP.String())
		}
		t.pages.RemovePage("peer_actions")
	})
	list.AddItem("WHOIS Info", "Show detailed information", 'w', func() {
		t.pages.RemovePage("peer_actions")
		t.showWhois()
	})
	list.AddItem("Cancel", "Go back", 'c', func() {
		t.pages.RemovePage("peer_actions")
	})

	list.SetBorder(true).SetTitle(fmt.Sprintf("Actions for %s", p.ID))
	t.pages.AddPage("peer_actions", t.center(list, 40, 12), true, true)
}

func (t *TUI) drawMap(peers []stats.PeerStat) {
	// Node Topology Map
	byParent := make(map[string][]stats.PeerStat)
	for _, p := range peers {
		parent := p.ParentID
		if parent == "" {
			parent = "Local"
		}
		byParent[parent] = append(byParent[parent], p)
	}

	var buildTree func(string, string) string
	buildTree = func(id string, indent string) string {
		label := id
		for _, p := range peers {
			if p.ID == id {
				if p.Hostname != "" {
					label = p.Hostname
				}
				break
			}
		}
		if id == "Local" {
			label = "[green]Local Node[-]"
		}

		res := indent + "• " + label + "\n"
		children := byParent[id]
		for _, child := range children {
			res += buildTree(child.ID, indent+"  ")
		}
		return res
	}

	t.mapView.SetText(buildTree("Local", ""))
}

func (t *TUI) center(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewGrid().
		SetColumns(0, width, 0).
		SetRows(0, height, 0).
		AddItem(p, 1, 1, 1, 1, 0, 0, true)
}
