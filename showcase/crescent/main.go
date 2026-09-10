// Command crescent is the goWidgets showcase.
//
// It is the window shell of crescent (github.com/unxed/crescent): the manager
// that keeps Codex goal sessions moving, restarting them once a usage limit
// resets. Per §2.7 the library only grows what this showcase needs — right now
// that is a status line, a run toggle, and a list of goals to tick off.
//
//	go run ./showcase/crescent                            # native: GTK 3 or Win32
//	goWidgets_BACKEND=headless go run ./showcase/crescent # no window system
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
	_ "github.com/unxed/goWidgets/backends/win32"
)

// goal is a Codex session crescent watches. The real thing reads these out of
// ~/.codex/sessions; here they are literals so the showcase stays a UI probe.
type goal struct {
	title string
	box   *goWidgets.CheckBox
}

func main() {
	app, err := goWidgets.NewApp()
	if err != nil {
		log.Fatal(err)
	}
	d := app.Diagnostics()
	log.Printf("driver=%s native=%v attempts=%v", d.Name, d.Caps.NativeControls, d.Attempts)

	win, err := app.NewWindow("crescent", 460, 320)
	if err != nil {
		log.Fatal(err)
	}

	status, _ := win.AddLabel("")
	goals := []*goal{
		{title: "Цель: починить флаки в CI"},
		{title: "Цель: рефакторинг парсера rollout-файлов"},
		{title: "Цель: дописать тесты бэкенда"},
	}
	for _, g := range goals {
		box, err := win.AddCheckBox(g.title, false)
		if err != nil {
			log.Fatal(err)
		}
		g.box = box
	}

	run, _ := win.AddButton("Гнать все цели")
	drop, _ := win.AddButton("Убрать отмеченные")
	quit, _ := win.AddButton("Выход")

	// The dialog: status line across the top, goals stacked under it, three
	// equal buttons along the bottom edge. Only edges are stated; widths and
	// heights the platform does not fix come out of the solver.
	boxes := []goWidgets.Box{status}
	for _, g := range goals {
		boxes = append(boxes, g.box)
	}
	const pad = 8
	if err := win.Constrain(
		status.Left().Eq(win.Left().Plus(pad)),
		status.Top().Eq(win.Top().Plus(pad)),
		status.Right().Eq(win.Right().Minus(pad)),
	); err != nil {
		log.Fatal(err)
	}
	if err := win.Constrain(goWidgets.Column(pad, boxes...)...); err != nil {
		log.Fatal(err)
	}
	if err := win.Constrain(
		run.Left().Eq(win.Left().Plus(pad)),
		run.Bottom().Eq(win.Bottom().Minus(pad)),
		quit.Right().Eq(win.Right().Minus(pad)),
	); err != nil {
		log.Fatal(err)
	}
	if err := win.Constrain(goWidgets.Row(pad, run, drop, quit)...); err != nil {
		log.Fatal(err)
	}
	if err := win.Constrain(goWidgets.EqualWidths(run, drop, quit)...); err != nil {
		log.Fatal(err)
	}

	// Declared before the closures that use them: refresh() updates the
	// tooltip, and Closing decides between hiding and quitting.
	var tray *goWidgets.TrayIcon
	hasTray := false

	live := func() int {
		n := 0
		for _, g := range goals {
			if g.box.Visible.Get() {
				n++
			}
		}
		return n
	}
	running := false
	refresh := func() {
		verb := "Простаиваю"
		if running {
			verb = "Работаю"
		}
		line := fmt.Sprintf("%s. Целей в очереди: %d", verb, live())
		status.Text.Set(line)
		drop.Enabled.Set(anyChecked(goals))
		if hasTray {
			tray.Tooltip.Set(line)
		}
	}

	for _, g := range goals {
		g.box.Toggled.On(app.Scope(), func(bool) { refresh() })
	}
	run.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		running = !running
		if running {
			run.Text.Set("Стоп")
		} else {
			run.Text.Set("Гнать все цели")
		}
		refresh()
	})
	drop.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		for _, g := range goals {
			if g.box.Checked.Get() {
				g.box.Visible.Set(false) // leaves the layout flow entirely
			}
		}
		refresh()
	})
	quit.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { app.Quit() })

	// Closing hides to the tray rather than quitting — the veto path exists
	// exactly for this. Without an icon there would be no way back, so the
	// window only hides when one was actually created.
	win.Closing.On(app.Scope(), func(req *goWidgets.CloseRequest) {
		if hasTray {
			req.CancelClose()
			win.Hide()
			return
		}
		app.Quit()
	})

	show := goWidgets.NewMenuItem("Показать окно")
	quitItem := goWidgets.NewMenuItem("Выход")
	tray, err = app.NewTrayIcon("crescent", show, goWidgets.Separator(), quitItem)
	if err != nil {
		log.Printf("трей недоступен (%v) — работаем окном", err)
	} else {
		hasTray = true
		show.Clicked.On(app.Scope(), func(struct{}) { win.Show() })
		quitItem.Clicked.On(app.Scope(), func(struct{}) { app.Quit() })
		tray.Activated.On(app.Scope(), func(struct{}) { win.Show() })
	}

	// A background goroutine touching the UI only through QueueUpdate (§4.5.3),
	// standing in for the poller that watches for the usage-limit reset.
	go func() {
		for t := range time.Tick(time.Second) {
			stamp := t.Format("15:04:05")
			app.QueueUpdate(func() {
				if running {
					status.Text.Set(fmt.Sprintf("Работаю. Целей: %d. Опрос: %s", live(), stamp))
				}
			})
		}
	}()

	refresh()
	if err := app.Run(win); err != nil {
		log.Fatal(err)
	}
}

func anyChecked(goals []*goal) bool {
	for _, g := range goals {
		if g.box.Checked.Get() && g.box.Visible.Get() {
			return true
		}
	}
	return false
}
