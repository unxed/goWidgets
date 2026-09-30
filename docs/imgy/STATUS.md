# STATUS

Обновляется в **каждом** ответе с патчем: правкой существующих строк, а не дописыванием журнала.
Не длиннее ~40 строк. История — в `git log`.

Обновлено: 2026-09-30 (Cocoa Canvas pixel показан; поправлена точка pointer-теста, CI запущен).

## Где мы

- Текущая итерация: **B5c — Canvas на Cocoa**. AppKit RGBA presenter, mouse events и Auto Layout screenshot-тест опубликованы в `goWidgets/main` (`d4b86e1`). Cocoa pixel/screenshot и типы событий прошли в CI, но coordinate assertion выявил неправильный `NSPoint`, переданный тестовым helper-ом; исправление в `f4f1401`, run `36726555566` проверяет координаты. Синхронный native mouse-down в тесте заменён прямым вызовом event-конвертера после зависания в AppKit tracking loop (`3134760`). B1–B5 уже в `main`; Qt run `36704317386` зелёный, screenshot просмотрен. Ручные тесты в этой среде не запускались.
- A1 завершена: установщик — NSIS 2, извлечён 7-Zip 26.03; обнаружена англоязычная справка CHM, оглавление `Table of Contents.hhc`, кодировка страниц заявлена ISO-8859-1. Точные URL/hash/временные пути — `spec/REFERENCE-NOTES.md`.
- A2 завершена: просмотрены все 7 разделов оглавления; 65 функциональных пунктов в `spec/INVENTORY.md`. `License Agreement` и `Contact Us` не добавили UI-возможностей; handling notes — в `spec/REFERENCE-NOTES.md`.
- B1: Canvas API/core/headless и терминальный Canvas в `vtui/main`.
- B2: GTK Canvas через `GtkDrawingArea`/Cairo; GitHub Linux/Xvfb проверяет пиксели и настоящие click/wheel.
- B3: Win32 presenter через top-down 32-bit DIB; сериализованная Windows CI проверяет Auto Layout, первый кадр, ввод и capture; PNG-артефакт проверен.
- B5: Qt Canvas реализован через QLabel/QPixmap. Qt 5/6 offsets wheel (72/88) и mouse button state (80/64) промерены в C++ probe; Auto Layout-тесты проверяют click/wheel, RGBA-пиксель и snapshot. CI `36704317386` полностью зелёный.
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
- Canvas parity для Qt 5/6 закрыт по Xvfb; Cocoa presenter и CI-тест есть, но паритет отмечается только после зелёного macOS CI.

## Как это узнать

- Canvas уже опубликован в `vtui/main` и `goWidgets/main`; устаревшие накопившиеся PR #6–8 закрыты после прямой публикации изменений в main.
- `goWidgets/main` commit `d4b86e1` содержит Cocoa Canvas presenter + pointer input и CI-тест с Auto Layout/screenshot.

## Следующий шаг

- Следующий шаг: дождаться run `36726555566`; при успехе закрыть B5c/D6 и начать B6 (viewport modes, cursor-centered zoom, pan, natural-sorted adjacent files). BMP отложен до явного разрешения `golang.org/x/image` (Q4).
