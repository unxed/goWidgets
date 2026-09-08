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
		status.Text.Set(fmt.Sprintf("%s. Целей в очереди: %d", verb, live()))
		drop.Enabled.Set(anyChecked(goals))
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

	// Closing the window will eventually mean "hide to tray"; the veto path
	// that makes that possible is already in place.
	win.Closing.On(app.Scope(), func(req *goWidgets.CloseRequest) { app.Quit() })

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
