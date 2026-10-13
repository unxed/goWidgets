# ADR-0018. Web/WASM бэкенд: DOM-контролы через syscall/js, Go→WASM без CGO

* Статус: предложено
* Дата: 2026-10-13
* Затрагивает: §1.3 (non-goals: «нет мобильных»), §3.3 (js→web), §4.4, §4.5 (WASM однопоточен), §8 (Фаза 8), §11 (R6)

## Контекст

Философия goWidgets — **нативные контролы на каждой платформе**. Для веба «нативными» являются DOM-элементы браузера (buttons, inputs, canvas, dialogs). Go умеет собираться в WebAssembly (`GOOS=js GOARCH=wasm`), runtime однопоточен, CGO недоступен — идеально вписывается в `CGO_ENABLED=0`.

Цель: **один и тот же UI-код** (showcase `pixy`, `crescent`, будущие приложения) работает в браузере без `//go:build` веток. Электрон/Node.js — отдельный таргет: тот же WASM + Node.js bindings для FS/process/трея.

## Решение

### 1. Компиляция и рантайм

| Таргет | Компилятор | Runtime | Взаимодействие с платформой |
|--------|------------|---------|----------------------------|
| **Браузер** | стандартный `go build -target=js/wasm` | `wasm_exec.js` | `syscall/js` → DOM API |
| **Electron (Main)** | стандартный `go build -target=js/wasm` + `wasm2js` или `wazero` | Node.js `vm` / `wazero` | Node.js API через `syscall/js` shim или CGO-free FFI |
| **Electron (Renderer)** | как браузер | Chromium | тот же DOM API |

> Для Electron Main предпочтительнее **wazero** (WASM runtime на Go) — никакого JS-шелла, прямые импорты Node.js API через host functions. Но v1 можно начать с **browser-only**; Electron как отдельный драйвер `electron-main` добавится по требованию showcase.

### 2. Архитектура бэкенда (`backends/web`)

```
backends/web/
├── driver.go         # PlatformDriver implementation
├── window.go         # BackendWindow: создаёт <div id="gw-root">, управляет document.body
├── widget.go         # CreateWidget → document.createElement + мэппинг WidgetKind → тег
├── layout.go         # ApplyLayout → style.transform = translate3d(x,y,0) + width/height
├── intrinsic.go      # MeasureIntrinsic → getBoundingClientRect после временного append
├── events.go         # EventRouter: addEventListener → BackendEvent → Core
├── key.go            # KeyboardEvent → Key (таблица как в ADR-0014, но для браузерных кодов)
├── dialogs.go        # FileDialog: <input type=file> + FileReader API
├── clipboard.go      # navigator.clipboard API
├── tray.go           # заглушка (Caps.TrayIcon=false) или Electron IPC при наличии
├── a11y.go           # aria-* атрибуты, role, tabindex — минимальный доступность
└── abi.go            # 헬перы: js.Value → Go типы, batch DOM updates в requestAnimationFrame
```

**Ключевые принципы:**

* **DIP ↔ CSS px**: 1 DIP = 1 CSS px. `devicePixelRatio` учитывается в `ScaleInfo` (как Retina/per-monitor DPI).
* **Layout**: Core отдаёт `Rect` в DIP → бэкенд ставит `style.transform: translate3d(${x}px, ${y}px, 0)` + `width: ${w}px; height: ${h}px`. `translate3d` — дешевле `left/top`, включает GPU-компоновку.
* **Batching**: все DOM-мутации на кадре собираются и применяются в одном `requestAnimationFrame` callback (аналог `ApplyLayout` батча).
* **Intrinsic sizes**: `MeasureIntrinsic` создаёт невидимый элемент, измеряет `getBoundingClientRect`, удаляет. Кэшируется до изменения текста/шрифта/темы.
* **Потоковая модель**: WASM однопоточен → `QueueUpdate` = `queueMicrotask`; блокирующие вызовы (alert, prompt, synchronous XHR) **запрещены**. Все диалоги — асинхронные (`Promise`-based API в Core).

### 3. Мэппинг виджетов (WidgetKind → DOM)

| WidgetKind | DOM | Примечание |
|------------|-----|------------|
| Window | `<div class="gw-window">` (root) | `position: fixed; inset: 0` для fullscreen; `Content` — дочерний контейнер |
| Label | `<span class="gw-label">` | `white-space: nowrap` |
| Button | `<button class="gw-button">` | `type="button"`, `:focus-visible` стили |
| CheckBox | `<input type="checkbox" class="gw-checkbox">` + `<span>` | native appearance, `accent-color` через CSS var |
| Edit | `<input type="text" class="gw-edit">` | `input` event → `Changed`, `keydown Enter` → `Activated` |
| ComboBox | `<select class="gw-combo">` (dropdownOnly) / `<input>` + `<datalist>` | editable= false/true |
| ListBox | `<div class="gw-listbox" role="listbox">` + `<div role="option">` | виртуализация через `IntersectionObserver` при >100 items |
| TextView | `<div class="gw-textview" contenteditable="false">` | моноширинный, `white-space: pre` |
| Canvas | `<canvas class="gw-canvas">` | 2D context, `devicePixelRatio` учёт в `Scale` |
| Splitter | `<div class="gw-splitter">` + drag handling | `pointer-events` + CSS resize |
| FileDialog | `<input type="file" style="display:none">` | programmatic click, `multiple`, `accept` |
| Message/Confirm/Ask | кастомный модальный `<dialog>` | `showModal()`, `::backdrop`, focus trap |

**CSS**: встроенный минимальный reset + CSS variables для темы (light/dark через `@media (prefers-color-scheme)`). Никаких внешних CSS-фреймворков.

### 4. Capabilities (`Caps`)

```go
Caps{
    NativeControls: true,   // DOM = нативные для веба
    TreeView:       true,   // через <div role="tree">
    GridView:       true,   // виртуализированный список
    FileDialog:     true,   // <input type=file>
    Menus:          true,   // кастомные <menu> / contextmenu
    Clipboard:      true,   // navigator.clipboard
    IME:            true,   // compositionstart/update/end
    A11y:           true,   // aria-*, role, tabindex
    SmoothAnimation: true,  // CSS transitions / Web Animations API
    TrayIcon:       false,  // только в Electron
    MaxCallbacks:   10000,  // без жёсткого лимита, но батчим
}
```

### 5. Electron / Node.js — как отдельный драйвер

Приложение может собраться **дважды**:

1. `GOOS=js GOARCH=wasm` → `pixy.wasm` + `wasm_exec.js` → статический сайт / GitHub Pages.
2. **Electron**: тот же `pixy.wasm` запускается в Renderer-процессе (через `<script src="wasm_exec.js">`); Main-процесс на Go через **wazero** (или TinyGo `wasm2wasm` + WASI) получает `fs`, `process`, `nativeImage`, `Menu`, `Tray`.

**Общий код**: 100% UI-логики (Core + виджеты) общий. Различия только в `PlatformDriver` и `Capabilities`. `goWidgets_BACKEND=web` / `electron-main` / `electron-renderer`.

### 6. Размер бандла (NFR: < 8 МБ gzip)

* Go 1.26 WASM минимальный hello-world ~2.3 МБ gzip.
* goWidgets: Core + reactive + layout + базовые виджеты ~300 КБ Go-кода → ~5-6 МБ gzip (с `wasm_exec.js` ~100 КБ gzip).
* Оптимизации: `-ldflags="-s -w"`, `//go:build ignore` для неиспользуемых виджетов, `tinygo` как опция (меньше рантайм, но не все пакеты stdlib поддерживаются).

## Последствия

* **Фаза 8** (из disdoc.md §8) становится активной: реализация `backends/web`.
* `syscall/js` — единственная внешняя зависимость (в stdlib).
* Никакого CGO, никакого `emscripten` — стандартный тулчейн Go.
* `Caps` честно объявляет, чего нет (TrayIcon в браузере).
* `vcontract` конформанс-тесты гоняются в headless Chrome (Playwright/Puppeteer) в CI.
* Electron Main-процесс на Go — отдельная итерация после браузерного MVP.

## Альтернативы

| Вариант | Почему нет |
|---------|------------|
| **GopherJS** | Не развивается; Go 1.26+ не поддерживает; размер бандла больше. |
| **TinyGo** | Не весь stdlib; нет `plugin`, `reflect` ограничен; для v1 — риск. |
| **Vugu / Go+WASM фреймворки** | Имеют свой реактивный слой и layout — дублируют Core. |
| **WebView2 / Tauri / Wails** | Это «вебвью внутри нативного окна», а не нативный веб. Философия: нативные контролы = DOM в браузере. |
| **Ebiten на WASM** | Уже есть как `backends/ebiten` (fallback), но это Canvas-рендер, не DOM-контролы. |

## План реализации (итерации внутри Фазы 8)

| Итерация | Входы | Делаем | Не делаем | Готово, когда |
|----------|-------|--------|-----------|---------------|
| **W1** | Go 1.26, `syscall/js`, `wasm_exec.js` | `driver.go`: `Init` → проверка `js.Global()`, `RunMainLoop` = `select{}` (event-driven), `Wake` = `queueMicrotask` | виджеты, layout | `go build -target=js/wasm` собирается, `Init` не паникует |
| **W2** | W1 | `window.go` + `widget.go`: root `<div>`, `CreateWidget` для Button/Label/Edit, `SetString/Bool`, `ApplyLayout` через `transform3d` | сложные виджеты | Button кликается, Label показывает текст, Edit вводит текст |
| **W3** | W2 | `intrinsic.go`: `MeasureIntrinsic` через временный append + `getBoundingClientRect`; CSS reset + light/dark тема | ScrollArea, виртуализация | Golden layout tests проходят на web (headless Chrome) |
| **W4** | W3 | ComboBox, CheckBox, ListBox, TextView, FileDialog (`<input type=file>`), Clipboard | TreeView, GridView, Canvas | Конформенс `vcontract` для базовых виджетов зелёный в Playwright |
| **W5** | W4 | Canvas (2D context, `devicePixelRatio`), ScrollArea, Splitter, модальные диалоги (`<dialog>`) | виртуализация GridView | `pixy` в браузере: открыть картинку, зум, панель миниатюр |
| **W6** | W5 | Виртуализация (IntersectionObserver), анимации (Web Animations API), фокус/tab-order, IME | Electron | CI: Playwright тесты на PR; размер бандла в NFR |
| **W7** | W6 | Electron Renderer: тот же WASM, доступ к `navigator.clipboard`, `FileReader`; Electron Main на wazero (опционально) | нативные меню macOS/Windows в Electron | `pixy` запускается в Electron, трей работает |

## Риски (добавить в §11)

| # | Риск | Митигация |
|---|------|-----------|
| R12 | DOM-шторм при частых layout-обновлениях | Батч в `requestAnimationFrame`, `transform3d` вместо `left/top`, `will-change: transform` |
| R13 | `MeasureIntrinsic` фликкерит / ломает layout | Создаём элемент с `visibility:hidden; position:fixed; pointer-events:none`, измеряем, удаляем — в одном макротаске |
| R14 | WASM stack overflow на глубоких виджетовых деревьях | Go WASM stack = 10 МБ (настраиваемо `-ldflags="-Wl,-z,stack-size=..."`); лимит глубины каскада в Core |
| R15 | Браузерные диалоги (`alert`, `prompt`) блокируют поток | Только асинхронные API в Core; кастомные модальные `<dialog>` вместо `alert` |

## Открытые вопросы

1. **CSS-in-Go vs внешний CSS**: минимальный reset встроить в `driver.go` как `const cssReset = `...`` или генерировать `.css` файл при сборке?
2. **SSR / SEO**: не в скоупе v1 (SPA only).
3. **Service Workers / PWA**: отдельной итерацией после W6.
4. **`vcontract` тесты в CI**: Playwright на `ubuntu-latest` + `chromium` — добавить в `ci-gui.yml` path-trigger.