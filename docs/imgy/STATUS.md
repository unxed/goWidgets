# STATUS

Обновляется в **каждом** ответе с патчем: правкой существующих строк, а не дописыванием журнала.
Не длиннее ~40 строк. История — в `git log`.

Обновлено: 2026-09-30 (B4 CI зелёные на всех платформах; GTK screenshot приложения проверен).

## Где мы

- Текущая итерация: **B5 — спайк Canvas на Qt 5/6**. B4 реализован и PR #7 CI зелёный на Windows/Linux/macOS; screenshot окна GTK проверен. Начат Qt Canvas presenter на QLabel/QPixmap с purego event filter; добавлен C++ probe, чтобы определить Qt 5/6 QWheelEvent offset перед включением ввода. Отладочная Windows-сборка Qt не запускается локально.
- A1 завершена: установщик — NSIS 2, извлечён 7-Zip 26.03; обнаружена англоязычная справка CHM, оглавление `Table of Contents.hhc`, кодировка страниц заявлена ISO-8859-1. Точные URL/hash/временные пути — `spec/REFERENCE-NOTES.md`.
- A2 завершена: просмотрены все 7 разделов оглавления; 65 функциональных пунктов в `spec/INVENTORY.md`. `License Agreement` и `Contact Us` не добавили UI-возможностей; handling notes — в `spec/REFERENCE-NOTES.md`.
- B1: Canvas API/core/headless прошёл `go test ./...` локально и Linux/Windows/macOS CI в goWidgets PR #4. vtui PR #183 добавляет `Canvas` в vocabulary и реализует терминальный Canvas через `ImageSurface`/`GraphicsLayer`; полный CI PR #183 зелёный.
- B2: GTK Canvas реализован в goWidgets PR #5 через `GtkDrawingArea` и Cairo-owned ARGB32 surface; GitHub Linux/Xvfb проверил отрисовку пикселей и настоящие click/wheel события. Windows и macOS jobs PR #5 зелёные. Визуальная проверка доступна как CI screenshot-артефакт.
- B3: Win32 presenter на top-down 32-bit DIB; локальный `go test ./...`/`go vet ./...` и сериализованная Windows CI зелёные. Проверяются Auto Layout, первый кадр до ввода, click/move/release/wheel, модификаторы и capture; Windows PNG CI-артефакт проверен.
- План включает требования владельца: Auto Layout для нового UI и кода размещения виджетов; регрессионную проверку, что контролы целиком видны сразу после показа окна; паритет каждого нового контрола во всех драйверах; локальную проверку Windows и CI со скриншотами для Linux/macOS; инструкции для продолжения из чистого диалога.

## Что мы узнали на предыдущем шаге

- Автор плана опирался на README репозитория; код `core/` и `backends/*` он не читал и дистрибутив эталона не открывал.
- Из README известно: есть окно, Label, Button, CheckBox, Edit, ComboBox, ListBox, TextView, модальные
  сообщения, файловые диалоги, трей с меню (`NewMenuItem`, `Separator`), layout на констрейнтах,
  реактивные свойства; бэкенды headless/gtk/qt(6,5)/win32/cocoa; тесты под Xvfb и Wine
  (`backends/qt/qttest`, `backends/win32/win32test`). На настоящем Windows не запускалось.

## Чего мы ещё не знаем

- Точные приоритеты и сроки возможностей ещё не согласованы; A2 фиксирует их без исключения из охвата.
- Применимость указанной в справке поддержки XP–10 и минимальных требований к новому приложению не решена (`SYS-001`).
- Wine не найден; он нужен только для необязательной A3 автоматизации эталона.
- Canvas parity для Qt 5/6 и Cocoa остаётся незавершённой; B5 начинает Qt-спайк.

## Как это узнать

- Добавить Canvas в vocabulary vtui, опубликовать B1; затем начать B2 GTK.
- Следующий шаг: опубликовать GTK ветку и проверить Xvfb input/pixel screenshot на GitHub CI; не объявлять GTK parity до зелёных тестов.

## Следующий шаг

- Следующий шаг: по B5 проверить показ RGBA и мышиный ввод на Qt 5 и Qt 6 без cgo, затем оформить ADR и закрыть паритет; после — Cocoa. BMP отложен до явного разрешения `golang.org/x/image` (Q4).
