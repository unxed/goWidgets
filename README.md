# goWidgets

Кроссплатформенный GUI для Go на **нативных контролах ОС**, без CGO.
Один и тот же код открывает окно с настоящими Win32-контролами на Windows и
настоящими GTK-виджетами на Linux.

Нормативный дизайн: [`docs/disdoc.md`](docs/disdoc.md). Решения, отклоняющиеся
от него, фиксируются в [`docs/adr/`](docs/adr/).

> **Статус: ранняя итерация.** Работают окно, `Label`, `Button`, `CheckBox`,
> `Entry`, `TextView`, иконка в трее с меню, layout на констрейнтах (Cassowary) и
> реактивные свойства. Нет: анимаций, фокуса и tab-order, Cocoa, Web.

## Пример

```go
package main

import (
	"github.com/unxed/goWidgets"
	_ "github.com/unxed/goWidgets/backends/gtk"
	_ "github.com/unxed/goWidgets/backends/headless"
	_ "github.com/unxed/goWidgets/backends/win32"
)

func main() {
	app, _ := goWidgets.NewApp()
	win, _ := app.NewWindow("Привет", 400, 200)

	status, _ := win.AddLabel("Готов")
	btn, _ := win.AddButton("Нажми")
	btn.Clicked.On(app.Scope(), func(goWidgets.ClickInfo) {
		status.Text.Set("Нажато")
	})

	app.Run(win)
}
```

Текстовое поле — `Entry`: `Text` двусторонний, `Changed` на каждую правку,
`Activated` на Enter:

```go
field, _ := win.AddEntry("")
field.Activated.On(app.Scope(), func(s string) { addGoal(s); field.Text.Set("") })
```

Фокус: `widget.Focus()` ставит клавиатурный фокус (можно до показа окна);
Tab и Shift+Tab дальше ходят сами, в порядке раскладки — сверху вниз, слева
направо — на обеих платформах.

Из фоновой горутины трогать виджеты нельзя — только через `app.QueueUpdate`:

```go
go func() {
	result := slowWork()
	app.QueueUpdate(func() { status.Text.Set(result) })
}()
```

## Layout

Диалог описывается констрейнтами между краями виджетов; прямоугольники
считает Cassowary ([kiwi-go](https://github.com/unxed/kiwi-go)). Нативных
sizer'ов нет, и других layout-механизмов тоже — только этот (ADR-0005).

```go
status, _ := win.AddLabel("Готов")
ok, _ := win.AddButton("OK")
cancel, _ := win.AddButton("Отмена")

win.Constrain(
	status.Left().Eq(win.Left().Plus(8)),
	status.Top().Eq(win.Top().Plus(8)),
	status.Right().Eq(win.Right().Minus(8)),

	ok.Right().Eq(win.Right().Minus(8)),
	ok.Bottom().Eq(win.Bottom().Minus(8)),
	cancel.Right().Eq(ok.Left().Minus(8)),
	cancel.Top().Eq(ok.Top()),
	cancel.Width().Eq(ok.Width()),
)
```

У каждого виджета и у окна восемь якорей: `Left`, `Top`, `Right`, `Bottom`,
`Width`, `Height`, `CenterX`, `CenterY`. Якорь можно сдвинуть (`Plus`,
`Minus`) и масштабировать (`Times`), связать с другим (`Eq`, `Ge`, `Le`) или
с числом (`Is`, `AtLeast`, `AtMost`). Что не зафиксировано — берёт
естественный размер, измеренный платформой.

Сила правил: минимальный размер виджета — `required`; правила пользователя —
`strong` по умолчанию; естественные размеры — `weak`. Когда strong-правила
не сходятся, solver выполняет сколько может — это не ошибка, а не та вёрстка,
которую вы имели в виду; `.Medium()` и `.Weak()` ставят приоритеты.
`required` против `required` — ошибка: правило отбрасывается,
`Constrain` возвращает `core.ErrUnsatisfiable`, конфликт виден в
`app.Diagnostics().LayoutConflicts`; сборка `-tags goWidgets_debug` паникует.

Виджет, о котором нет ни одного правила, укладывается «потоком»: под
предыдущим таким же, во всю ширину, с отступом 8. Первое правило о нём
(владелец левого якоря — субъект правила) выводит его из потока. Так окно без
единого констрейнта всё равно что-то показывает.

Кто растягивается, когда в строке два виджета, а места больше, чем им
нужно, — выбор solver'а, пока их предпочтения равны. `HugWidth()` (и
`HugHeight()`) делает естественный размер виджета strong-правилом: в строке
«поле + кнопка» кнопка с `HugWidth()` остаётся кнопкой, а поле берёт остаток.

Для типовых форм есть сахар, который собирает те же констрейнты:
`Column(gap, …)`, `Row(gap, …)`, `EqualWidths(…)`, `EqualHeights(…)`,
`Fill(item, container, inset)`. Первый элемент цепочки остаётся там, где его
поставили (в потоке или прижатым к окну), остальные следуют за ним:

```go
win.Constrain(goWidgets.Column(8, status, cb1, cb2, cb3)...)
win.Constrain(goWidgets.Row(8, run, drop, quit)...)
win.Constrain(goWidgets.EqualWidths(run, drop, quit)...)
```

Скрытый виджет (`Visible.Set(false)`) в цепочке схлопывается до нуля по
свободной оси, и всё под ним поднимается; зазор цепочки остаётся.

Вложенность — через `Guide`: прямоугольник, который есть только в solver'е
(без контрола, не рисуется, вне потока). Панель кнопок, две колонки, отступ
внутри окна — это guide, к которому привязаны виджеты:

```go
bar := win.NewGuide()
win.Constrain(
	bar.Left().Eq(win.Left().Plus(8)),
	bar.Right().Eq(win.Right().Minus(8)),
	bar.Bottom().Eq(win.Bottom().Minus(8)),
	bar.Height().Is(36),
)
win.Constrain(goWidgets.Fill(ok, bar, 0)...)
```

Производительность: resize окна с 500 связанными узлами — 1.1–1.5 мс
(`go test -run xxx -bench Layout .`), NFR 4 мс выполнен; первое построение
такой системы — 0.3–0.5 с, это внутри kiwi-go (ADR-0006).

## Сборка

```sh
go build ./...
go test ./...

# Windows: -H windowsgui убирает консольное окно
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" ./showcase/crescent
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w"               ./showcase/crescent
```

Бэкенд выбирается автоматически (§3.3); принудительно — через переменную
окружения, и тогда ошибка инициализации не заглушается фолбэком:

```sh
goWidgets_BACKEND=headless go run ./showcase/crescent
```

`app.Diagnostics()` показывает, какой драйвер выиграл и что было испробовано —
это ответ на вопрос «почему у меня не нативный вид».

## Трей

```go
show := goWidgets.NewMenuItem("Показать окно")
quit := goWidgets.NewMenuItem("Выход")
tray, err := app.NewTrayIcon("подсказка", show, goWidgets.Separator(), quit)
if err != nil {
	// Бэкенда без статусной области достаточно, чтобы жить дальше:
	// это не фатальная ошибка, а отсутствие возможности.
}
tray.Activated.On(app.Scope(), func(struct{}) { win.Show() })
```

Иконка принадлежит приложению, а не окну, поэтому спрятанное окно её не уносит —
ради этого трей и нужен. Закрытие окна отменяется через `Closing` и превращается
в `Hide`.

На Windows это `Shell_NotifyIcon`. На Linux — `GtkStatusIcon`: он объявлен
устаревшим в GTK 3.14, но присутствует и работает в 3.24, и панели Cinnamon,
XFCE, MATE и KDE его принимают. `StatusNotifierItem` формально правильнее, но
это D-Bus-протокол, который пришлось бы писать вручную, и на GNOME он без
расширения всё равно не показывается. Устаревшее и работающее победило
правильное и непроверяемое; замена затрагивает один файл.

Пристыковку иконки проверяет тест: Xvfb даёт дисплей, stalonetray — панель, и
`TrayIcon.Embedded()` отвечает, приняла ли она иконку. Без дисплея или панели
тест пропускается, в CI и то и другое есть.

## Как это устроено

```
   goWidgets (корень)      публичный API: Widget, Property, Event
        │                  Handle + Rect в DIP, батч-команды
   core                    node registry, scheduler, конвейер кадра
        │
   backends/*              headless | gtk | win32
```

Строгое правило: `backends/* → core` можно, `core → backends/*` нельзя.
Драйверы регистрируются через `init()`.

**Layout принадлежит Core.** Нативные sizer'ы не используются: Core считает
абсолютные прямоугольники и отдаёт их бэкенду батчем. На Win32 это `SetWindowPos`,
на GTK — `GtkFixed`. Естественные размеры при этом спрашиваются у платформы
(`MeasureIntrinsic`), иначе текст не разместить корректно.

**Без CGO.** На Windows — `golang.org/x/sys/windows` и `syscall.NewCallback`.
На Linux — динамическая загрузка GTK через
[`purego`](https://github.com/ebitengine/purego), подменённый на
[`pureffi`](https://github.com/unxed/pureffi) директивой `replace` в `go.mod`.
Если GTK на машине нет, `Init` вернёт ошибку, а не уронит процесс.

## Проверено

* GTK 3: окно с нативными контролами Adwaita рендерится под Xvfb; фоновая
  горутина обновляет UI через `QueueUpdate`.
* Golden-тесты layout (стек и диалог на констрейнтах), resize через
  edit-переменные, схлопывание скрытой строки, неразрешимая система → ошибка и
  диагностика, тест двусторонней привязки `CheckBox`, тест на утечки
  подписок, тест glitch-free `Batch`.
* `CGO_ENABLED=0` сборка под `windows/amd64`, `windows/arm64`, `linux/amd64`,
  `linux/arm64`, `darwin/arm64` (последняя уходит на headless — драйвера Cocoa ещё нет).
* GTK 3: `Entry` под Xvfb — текст в обе стороны, `Changed`, `Activated`,
  без эха; скриншот showcase с полем ввода.
* GTK 3: клавиатурное событие, построенное через `gdk_event_new` и поданное в
  `gtk_widget_event`, доходит до `KeyPressed` как Ctrl+A (тест под Xvfb).
* `go vet` чистый под обе платформы; debug-сборка (`-tags goWidgets_debug`)
  проходит тесты и проверяется в CI.

* Win32 под Wine (`backends/win32/win32test`: `GOOS=windows go test -c`, потом
  `wine`): фокус, набор, Enter, Tab против настоящего окна; showcase под Wine
  с клавиатуры.

На настоящем Windows пока не запускалось — только Wine и CI.

## Лицензия

См. [LICENSE](LICENSE).
