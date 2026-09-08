# goWidgets

Кроссплатформенный GUI для Go на **нативных контролах ОС**, без CGO.
Один и тот же код открывает окно с настоящими Win32-контролами на Windows и
настоящими GTK-виджетами на Linux.

Нормативный дизайн: [`docs/disdoc.md`](docs/disdoc.md). Решения, отклоняющиеся
от него, фиксируются в [`docs/adr/`](docs/adr/).

> **Статус: ранняя итерация.** Работают окно, `Label`, `Button`, `CheckBox`,
> вертикальный стек и реактивные свойства. Нет: Cassowary, анимаций, фокуса и
> tab-order, трея, Cocoa, Web.

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

Из фоновой горутины трогать виджеты нельзя — только через `app.QueueUpdate`:

```go
go func() {
	result := slowWork()
	app.QueueUpdate(func() { status.Text.Set(result) })
}()
```

## Сборка

```sh
go build ./...
go test ./...

# Windows: -H windowsgui убирает консольное окно
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" ./showcase/relay
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w"               ./showcase/relay
```

Бэкенд выбирается автоматически (§3.3); принудительно — через переменную
окружения, и тогда ошибка инициализации не заглушается фолбэком:

```sh
goWidgets_BACKEND=headless go run ./showcase/relay
```

`app.Diagnostics()` показывает, какой драйвер выиграл и что было испробовано —
это ответ на вопрос «почему у меня не нативный вид».

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
* Golden-тест layout, тест двусторонней привязки `CheckBox`, тест на утечки
  подписок, тест glitch-free `Batch`.
* `CGO_ENABLED=0` сборка под `windows/amd64`, `windows/arm64`, `linux/amd64`,
  `linux/arm64`, `darwin/arm64` (последняя уходит на headless — драйвера Cocoa ещё нет).
* `go vet` чистый под обе платформы.

Win32-драйвер собирается и проходит `vet`, но вживую пока не запускался.

## Лицензия

См. [LICENSE](LICENSE).
