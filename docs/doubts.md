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

- [ ] Трей на Linux: `GtkStatusIcon` устарел, нужен `StatusNotifierItem` по D-Bus
      (на Windows — `Shell_NotifyIcon`). Блокирует showcase relay.
- [ ] `ListView` с колонками и галочками — следующий виджет для relay.
- [ ] Cassowary (Фаза 2): текущий layout — вертикальный стек с фиксированными
      размерами, интерфейс `func(*App) []BoundsChange` уже тот, что нужен солверу.
- [ ] Фокус и tab-order живут в Core, но пока не реализованы; на Win32 у контролов
      стоит `WS_TABSTOP`, то есть частично этим сейчас занимается платформа.
- [ ] `DiscreteAnimator` / `SmoothAnimator` — не начаты.
