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
	edit, _ := win.AddEdit("")
	model, err := win.AddComboBox([]string{"gpt-5.6-luna", "gpt-5.6", "gpt-5.6-mini"}, true)
	if err != nil {
		log.Fatal(err)
	}
	model.Select(0)
	add, _ := win.AddButton("Добавить")
	run, _ := win.AddButton("Гнать все цели")
	drop, _ := win.AddButton("Убрать отмеченные")
	quit, _ := win.AddButton("Выход")

	// The dialog: status line across the top, the new-goal field with the
	// model to run it on and its button under it, goals stacked under that, three equal buttons along
	// the bottom edge. Only edges are stated; widths and heights the platform
	// does not fix come out of the solver.
	const pad = 8
	if err := win.Constrain(
		status.Left().Eq(win.Left().Plus(pad)),
		status.Top().Eq(win.Top().Plus(pad)),
		status.Right().Eq(win.Right().Minus(pad)),

		edit.Left().Eq(status.Left()),
		edit.Top().Eq(status.Bottom().Plus(pad)),
		add.Right().Eq(status.Right()),
	); err != nil {
		log.Fatal(err)
	}
	if err := win.Constrain(goWidgets.Row(pad, edit, model, add)...); err != nil {
		log.Fatal(err)
	}
	model.HugWidth() // the field takes the slack; the list and the button
	add.HugWidth()   // keep their natural widths
	edit.Focus()     // typing a goal is the first thing to do here

	// Goals are a column that grows at run time: each new box is chained
	// under the previous one (or under the field, for the first). Removing
	// one hides it, and the column closes up by its height.
	var goals []*goal
	addGoal := func(title string) {
		box, err := win.AddCheckBox(title, false)
		if err != nil {
			log.Fatal(err)
		}
		var above goWidgets.Box = edit
		if n := len(goals); n > 0 {
			above = goals[n-1].box
		}
		if err := win.Constrain(
			box.Top().Eq(above.Bottom().Plus(pad)),
			box.Left().Eq(status.Left()),
			box.Right().Eq(status.Right()),
		); err != nil {
			log.Fatal(err)
		}
		goals = append(goals, &goal{title: title, box: box})
	}
	for _, title := range []string{
		"Цель: починить флаки в CI",
		"Цель: рефакторинг парсера rollout-файлов",
		"Цель: дописать тесты бэкенда",
	} {
		addGoal(title)
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

	// Toggling a box only changes what "drop" applies to; the box list is
	// dynamic, so the subscription is made where the box is.
	submit := func() {
		title := edit.Text.Get()
		if title == "" {
			return
		}
		addGoal(title + " [" + model.Text.Get() + "]")
		goals[len(goals)-1].box.Toggled.On(app.Scope(), func(bool) { refresh() })
		edit.Text.Set("")
		refresh()
	}
	for _, g := range goals {
		g.box.Toggled.On(app.Scope(), func(bool) { refresh() })
	}
	edit.Activated.On(app.Scope(), func(string) { submit() })
	add.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) { submit() })
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
		n := 0
		for _, g := range goals {
			if g.box.Checked.Get() {
				n++
			}
		}
		// A modal question: the platform's own box, its own Yes/No labels,
		// blocking here until answered while the app keeps ticking underneath.
		if !win.Ask("crescent", fmt.Sprintf("Убрать отмеченные цели (%d)?", n)) {
			return
		}
		for _, g := range goals {
			if g.box.Checked.Get() {
				g.box.Visible.Set(false) // collapses out of its column
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
