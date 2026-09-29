# goWidgets Architectural Considerations & TODO List

## vreactive Layer
- [ ] Implement loop/cycle detection in `Property[T]` notify chains (using evaluation context or depth counter).
- [ ] Provide thread-safe mutation primitives for `Property[T]` when updated from background goroutines.
- [ ] Implement `DiscreteAnimator` (for vtui frame-based updates) and `SmoothAnimator` (for vgui tick-based updates) sharing a unified `Animator` interface.

## Core & Backend Integration
- [ ] Ensure `UpdateLayout` batching during window resizes to avoid redundant Cassowary solver recalculations.
- [ ] Web backend: benchmark DOM tree updates using CSS `position: absolute` with `requestAnimationFrame` batching.
- [ ] Verify `puregotk` dynamic library loading error handling on Linux distributions without GTK3 preinstalled.

## Закрыто в итерации от 2026-09-08

- [x] `Property[T]`: защита от каскадов — `MaxPropagationDepth`, повторный `Set`
      равным значением не уведомляет, `Batch` glitch-free (тесты в `layout_golden_test.go`).
- [x] Мутация из фоновых горутин — только через `App.QueueUpdate`; проверка
      привязки к UI-потоку включается тегом `goWidgets_debug` (ADR-0002).
- [x] `puregotk`/GTK на дистрибутивах без GTK3 — драйвер честно возвращает ошибку
      из `Init`, и `core` уходит на следующий драйвер по цепочке §3.3;
      причина видна в `App.Diagnostics()`.

## Открыто

- [x] Трей сделан: на Windows `Shell_NotifyIcon`, на Linux `GtkStatusIcon`.
      Последний устарел с GTK 3.14, но присутствует и работает в 3.24, и панели
      Cinnamon, XFCE, MATE, KDE его принимают. `StatusNotifierItem` правильнее,
      но это D-Bus-протокол, который пришлось бы писать руками, ему нужен живой
      StatusNotifierWatcher, а на GNOME он без расширения всё равно не виден.
      Замена затрагивает один файл, если панель откажется.
- [x] Трей проверен на живой панели, и проверка автоматическая. Оказалось, что
      человек с рабочим столом для этого не нужен: Xvfb даёт дисплей, а
      stalonetray — панель, и пристыковка становится обычным утверждением в
      тесте через `gtk_status_icon_is_embedded`. Гоняется в CI.
- [ ] GTK нельзя инициализировать в процессе, где уже создавалось приложение на
      другом драйвере: вместе они падают, порознь работают. Пока обойдено тем,
      что GTK-тест живёт в своём пакете и получает свой бинарник. Если позже
      понадобится несколько `App` в одном процессе, это придётся решать.
- [ ] `ListView` с колонками и галочками — следующий виджет для crescent.
- [x] Cassowary (Фаза 2) — сделано, ADR-0005/0006.
- [x] Фокус и tab-order — сделано, ADR-0008; Qt и Cocoa следуют тому же правилу.
- [ ] `DiscreteAnimator` / `SmoothAnimator` — не начаты.

## Открыто после Qt- и Cocoa-бэкендов (2026-09-29)

Для следующей сессии: всё ниже — не сделанное или не проверенное, с тем, где
оно живёт. Обязательного незавершённого нет: GTK, Qt 6/5, Cocoa и Win32 на
паритете, CI зелёный на Linux, macOS и Windows.

Cocoa (`backends/cocoa`, ADR-0014)
- [ ] Intel-маки (amd64) не запускались: macos-latest — arm64. Отличие только в
      `objc_msgSend_stret` для возврата NSRect (выбирает pureffi). Проверка —
      джоба на `macos-13` (Intel) в `.github/workflows/ci.yml`.
- [ ] Подписи кнопок NSAlert (`appKitString` в `dialogs_darwin.go`) ищутся в
      таблице "Common" бандла AppKit — это догадка; на русском macOS не видено.
      Если не переводится — искать настоящий ключ/таблицу или брать строки у
      NSSavePanel.
- [ ] `NSOpenPanel` из теста на файл не направить (панель вне процесса, `ok:`
      не реализован) — `OpenFile` проверяется только отменой; `SaveFile` —
      через `stopModalWithCode:` (хук `EndModal`).
- [ ] Клавиши: Wine берёт vkey из раскладки (UCKeyTranslate) для всех
      нефиксированных клавиш, включая пунктуацию; здесь из раскладки — только
      буквы и цифры (`charactersIgnoringModifiers`), пунктуация — по
      US-позиции из `default_map`. Логика — `keymap.go`, тест — `keymap_test.go`.
- [ ] Трей: freedesktop-имя иконки (`TraySpec.IconName`) игнорируется, всегда
      SF Symbol `gearshape`.

Qt (`backends/qt`, ADR-0013)
- [ ] Проверено только на amd64 и только xcb под Xvfb (Ubuntu 24.04: Qt 6.4,
      Qt 5.15). Wayland и arm64 не запускались; для Wayland проверка дисплея в
      `abi_linux.go` смотрит только наличие плагина.
- [ ] Новые мажорные версии Qt 6 (6.7+) не проверялись: если там переехал
      какой-то символ, `Init` честно скажет «missing symbols: …», и имя надо
      добавить вторым вариантом в `bindAll`/`bindDialogs`/`bindTray`.

Общее
- [ ] Падение после PASS в тестах GTK/Qt (горутина, залоченная `NewApp`,
      завершается — fakecgo из goffi не переживает выход потока) обойдено
      `runtime.UnlockOSThread` в тестах. Сам дефект — в goffi
      (unxed/goffi): стоит воспроизвести отдельно и починить там.
- [ ] Win32: `WM_QUIT` под модальным диалогом теперь не теряется (повтор
      запроса); Wine этот случай не воспроизводит, ловит только
      windows-latest (`backends/win32/quittest`).
- [ ] Раскладка: метки держат естественную высоту на `weak` (метка бывает
      многострочной областью); однострочную метку в колонке с запасом нужно
      `HugHeight()`, иначе запас может уйти ей (ADR-0015, imgy).
