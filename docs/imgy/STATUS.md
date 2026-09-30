# STATUS

Обновляется в **каждом** ответе с патчем: правкой существующих строк, а не дописыванием журнала.
Не длиннее ~40 строк. История — в `git log`.

Обновлено: 2026-09-30 (B1 core + headless реализован и проверен локально).

## Где мы

- Текущая итерация: **B1** (core + headless готовы; запись Canvas в vtui и публикация ещё впереди).
- A1 завершена: установщик — NSIS 2, извлечён 7-Zip 26.03; обнаружена англоязычная справка CHM, оглавление `Table of Contents.hhc`, кодировка страниц заявлена ISO-8859-1. Точные URL/hash/временные пути — `spec/REFERENCE-NOTES.md`.
- A2 завершена: просмотрены все 7 разделов оглавления; 65 функциональных пунктов в `spec/INVENTORY.md`. `License Agreement` и `Contact Us` не добавили UI-возможностей; handling notes — в `spec/REFERENCE-NOTES.md`.
- B1: добавлены Canvas API/core/headless, DIP mouse input, scale-aware reusable RGBA buffer, coalesced invalidation, optional presentation contract and channel-order conversion helper. Headless tests cover auto-flow layout, pixel presentation, coalescing, scale 1→2, mouse coordinates and event-scope cleanup. `go test ./...` passed locally with a task-specific temporary GOCACHE. Real backend parity remains open.
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
- Реальные GTK/Win32/Qt/Cocoa Canvas implementations and visual CI checks remain open.

## Как это узнать

- Добавить Canvas в vocabulary vtui, опубликовать B1; затем начать B2 GTK.
- Следующий локальный контроль: чистые диффы goWidgets/vtui и зелёные генераторные/unit-тесты перед публикацией.

## Следующий шаг

- Следующий шаг: запись Canvas в `unxed/vtui/vocabulary.json` отдельным PR, затем PR B1 core + headless; после CI начать B2 GTK.
