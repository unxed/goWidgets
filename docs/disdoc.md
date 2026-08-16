# goWidgets

Этот дизайн-документ (Design Document) составлен специально как исчерпывающее техническое задание. Он структурирован так, чтобы даже базовая LLM без доступа к интернету могла шаг за шагом реализовывать архитектуру, не сбиваясь с курса.

Документ строго следует **итеративной методологии RUP (Rational Unified Process)**: мы не пишем весь фреймворк целиком, а развиваем его через вертикальные срезы (от Inception до Transition).

---

# Design Document: `goWidgets` (Cross-Platform Go Native UI)

## 1. Концепция и Позиционирование
**`goWidgets`** — это кроссплатформенная GUI-библиотека для Go.
**Главная цель:** дать Go-разработчикам инструмент для создания десктопных и веб-приложений без боли с `CGO` и зависимостями, сохраняя нативный опыт и высокую производительность на ключевых платформах.

### 1.1. Вдохновение и интеграция с QML / `vtui`
Мы вдохновляемся сильными сторонами QML: реактивные свойства (`Property[T]`), автоматические биндинги (`Bind`/`Computed`), декларативные анимации (`Behavior`) и состояния (`StateMachine`).
Публичный API `goWidgets` проектируется максимально близким к TUI-библиотеке `vtui`. Один и тот же декларативный код UI должен с минимальными изменениями работать в терминале (`vtui`), на десктопе (`goWidgets` Win32/GTK/Cocoa) и в браузере (`goWidgets` Web).

### 1.2. Базовые принципы (Строгие правила для ИИ)
1. **Никакого `CGO` (`CGO_ENABLED=0`).** Это абсолютное правило. Весь FFI реализуется через `syscall` (Windows, Web/WASM) или `purego` (macOS, Linux/GTK). Библиотека `qt.go` **исключается** из проекта.
2. **Публичный API скрывает платформу и построен на реактивных свойствах.** Пользователь работает с `goWidgets.Button`, у которой свойства (`Text`, `Enabled`, `Width`, `Height`) являются реактивными объектами (`vreactive.Property[T]`).
3. **Единый Layout-движок.** Платформенные менеджеры компоновки (Sizers в GTK, AutoLayout в Cocoa) **не используются**. Ядро `goWidgets` само считает координаты через решатель Cassowary (библиотека `kiwi-go`) и передает бэкенду готовые абсолютные координаты `(X, Y, Width, Height)`.
4. **Приоритет нативных контролов.** На всех платформах (Win32, GTK, Cocoa, Web/DOM) используются родные системные элементы управления.
5. **Canvas / Ebiten — равноправная опция.** Используется как fallback, для создания графических canvas-виджетов и в ситуациях, когда нативные контролы недоступны.
6. **Реактивность и анимации — часть Core.** Все биндинги, цепочки пересчётов и контроллеры анимаций живут в ядре (`vreactive` / Core), а бэкенды получают уже готовые высчитанные значения.
7. **Итеративность (Showcase-driven).** Мы пишем только те виджеты и функции, которые нужны для работы целевого приложения-демонстратора (клона FastStone Image Viewer).

---

## 2. Архитектура (Уровни системы)

Система делится на 3 строгих слоя + общий реактивный пакет. LLM не должна допускать протекания абстракций между ними.

### Реактивный пакет: `vreactive`
Вынесен в отдельный пакет и используется совместно `goWidgets` и `vtui`:
* **`Property[T]`:** Реактивное свойство с подписками и уведомлением об изменениях.
* **`Bind` / `Computed`:** Автоматический пересчёт зависимостей.
* **`Behavior`:** Перехват изменения свойства с запуском анимации (интерполяции).
* **`StateMachine`:** Декларативные состояния виджета и правила переходов.
* **`Animator`:** Интерфейс анимаций (дискретный для TUI, плавный для GUI/Web).

### Уровень 1: Frontend (Public API)
Декларативный/компонентный интерфейс, унифицированный с `vtui`. Ничего не знает о бэкендах, FFI или `syscall`.
* **Сущности:** `App`, `Window`, виджеты (`Button`, `Label`, `TextInput`, `CheckBox`, `Splitter`, `TreeView`, `GridView`, `Canvas`).
* **Layout:** Пользователь задает констрейнты и контейнеры (`VBox`, `HBox`, `Anchor`, `AutoLayout`), транслируемые в Cassowary.
* **События:** Гибридный подход — колбэки (`OnClick`) + команды.

### Уровень 2: Core (Диспетчер, Реактивность и Вычислитель)
Скрыт от пользователя. Связывает Frontend и Backend.
* **Layout Solver:** Интеграция алгоритма Cassowary (`github.com/unxed/kiwi-go`). При изменении свойств окна или виджетов решает систему уравнений, отдает плоские координаты.
* **Animation Engine & Behavior Manager:** Управляет тиками анимаций, вычисляет промежуточные значения `Behavior` и обновляет реактивные свойства.
* **Event Router:** Транслирует платформенные события в абстрактные (`goWidgets.MouseClick`, `goWidgets.KeyPress`).
* **App Context:** Управляет Event Loop, гарантирует thread-safety и потокобезопасные обновления UI через Main Thread (`QueueUpdate`).

### Уровень 3: Backends (Драйверы платформ)
Слой реализации `PlatformDriver`. Каждая кнопка или окно в Frontend имеет своего двойника в Backend.
* **Win32 Driver (`syscall`):** Для Windows. Нативные HWND контролы (`BUTTON`, `EDIT`, `SysTreeView32`).
* **GTK Driver (`puregotk`):** Для Linux. Динамическая загрузка `libgtk-3.so` через `purego`.
* **Cocoa Driver (`purego`):** Для macOS. Взаимодействие с `libobjc.dylib` через `objc_msgSend`.
* **Web Driver (`syscall/js`):** Для Web/WASM. Создаёт нативные DOM-элементы (`<button>`, `<input>`, etc.). Core передаёт абсолютные координаты, бэкенд выставляет CSS-стили (`position: absolute; left: ...; top: ...; width: ...; height: ...`).
* **Ebiten / Custom Renderer:** Отрисовка с нуля. Fallback-драйвер и бэкенд для графического `Canvas`.

---

## 3. Интерфейсы ядра (Contract API для LLM)

ИИ должен опираться на эти интерфейсы при проектировании слоев.

```go
// --- REACTIVE LAYER (vreactive) ---

type Property[T any] interface {
    Get() T
    Set(val T)
    OnChange(handler func(newVal T)) func() // возвращает unsubscribe
}

// --- PUBLIC API (Уровень 1) ---

type Widget interface {
    SetID(id string)
    ID() string
    SetConstraints(c ...Constraint)
}

type Button struct {
    IDVal   string
    Text    vreactive.Property[string]
    Width   vreactive.Property[float64]
    Height  vreactive.Property[float64]
    Enabled vreactive.Property[bool]
    OnClick func()
}

type Window interface {
    Widget
    Title   vreactive.Property[string]
    Content vreactive.Property[Widget]
    Show()
}

type App interface {
    Run(mainWindow Window) error
    QueueUpdate(f func()) // Безопасное обновление UI из горутин
}


// --- BACKEND API (Уровень 3 - Скрыто от пользователя) ---

type PlatformDriver interface {
    Init() error
    RunMainLoop() error
    CreateWindow(title string, width, height int) (BackendWindow, error)
}

type BackendWindow interface {
    SetTitle(title string)
    Show()
    // Метод, через который Core передает готовые координаты
    UpdateLayout(rects map[string]Rect)
}

type Rect struct { X, Y, W, H int }

// BackendWidget — обертка над HWND, GtkWidget*, NSView* или DOM HTMLElement
type BackendWidget interface {
    SetBounds(r Rect)
    SetVisible(v bool)
}
```

---

## 4. План реализации: Итерации RUP

Работу с LLM нужно выстраивать **строго по этим фазам**. Запрещается переходить к следующей итерации, пока не закрыта текущая.

### Фаза 1: Inception & Reactive Core (`vreactive`)
**Цель:** Создать базовый реактивный слой и доказать жизнеспособность связки "Реактивное свойство -> Core -> Драйвер".
* **Задача 1:** Выделить и реализовать пакет `vreactive` (`Property[T]`, `Bind`, базовые подписки).
* **Задача 2:** Создать скелет проекта (`core`, `goWidgets`, `backends/ebiten`).
* **Задача 3:** Реализовать `goWidgets.Button` с реактивными свойствами `Text`, `Width`, `Height`, `Enabled`.
* **Задача 4:** Написать Ebiten-бэкенд для первого виджета и проверить отклик на клик и изменение `Text.Set(...)`.
* **Результат:** Приложение "Hello World", открывающее окно Ebiten с реактивной кнопкой.

### Фаза 2: Elaboration 1 (GTK Backend и FFI)
**Цель:** Проверить работу реактивных свойств на нативном тулките Linux.
* **Задача 1:** Добавить `backends/gtk` с использованием `puregotk`.
* **Задача 2:** Настроить автовыбор бэкенда на Linux (GTK -> при ошибке fallback на Ebiten).
* **Задача 3:** Реализовать `goWidgets.Button` через `puregotk`, связав реактивные свойства `Property[T]` с нативными свойствами `GtkButton`.
* **Результат:** Тот же `main.go` компилируется и запускается на GTK без изменения кода UI.

### Фаза 3: Elaboration 2 (Layout & Animation Engine)
**Цель:** Внедрить решатель констрейнтов Cassowary и систему анимаций `Behavior`.
* **Задача 1:** Интегрировать `kiwi-go` (Cassowary solver) в Core.
* **Задача 2:** Реализовать `VBox`, `HBox` и `Anchor`. При изменении размеров или свойств виджета Core автоматически пересчитывает координаты.
* **Задача 3:** Реализовать `Behavior[T]` и `Animator` в Core для плавного/дискретного изменения свойств (ширина, цвет, прозрачность).
* **Задача 4:** Изменить логику бэкендов: при обновлении `UpdateLayout` все `BackendWidget` получают `SetBounds(x, y, w, h)`. GTK использует `GtkFixed`.
* **Результат:** Окно с 3 реактивными кнопками, меняющими размер и положение с анимациями при растягивании окна и кликах.

### Фаза 4: Construction 1 (Windows MVP)
**Цель:** Нативный опыт на Windows без CGO.
* **Задача 1:** Написать `backends/win32` (пакет `golang.org/x/sys/windows`).
* **Задача 2:** Регистрация `WNDCLASSEX`, цикл `GetMessage`/`DispatchMessage`.
* **Задача 3:** Обернуть системные классы `BUTTON`, реагировать на `WM_COMMAND`. Интегрировать `MoveWindow` под управление Cassowary solver.
* **Результат:** Приложение работает нативно на Windows (`CGO_ENABLED=0`).

### Фаза 5: Construction 2 (Базовые элементы и StateMachine)
**Цель:** Виджеты для диалога настроек и поддержка декларативных состояний.
* **Задача 1:** Добавить виджеты: `Label`, `TextInput`, `CheckBox`, `Splitter` (сплиттер генерирует новые констрейнты при перетаскивании).
* **Задача 2:** Реализовать `StateMachine` в `vreactive` для описания состояний UI (Active/Inactive, Expanded/Collapsed).
* **Задача 3:** Реализовать новые виджеты для GTK, Win32 и Ebiten.
* **Результат:** "Окно настроек" с реактивными чекбоксами, инпутами и состояниями на 3 бэкендах.

### Фаза 6: Construction 3 (Тяжелые виджеты)
**Цель:** Каркас FastStone (Дерево, Грид, Канвас).
* **Задача 1:** `TreeView` (Win32: `SysTreeView32`, GTK: `GtkTreeView`).
* **Задача 2:** `GridView` / `IconView` (для отображения миниатюр).
* **Задача 3:** `Canvas` — виджет для прямой отрисовки пикселей/картинок (Win32: `BitBlt`/GDI+, GTK: `cairo_paint`, Ebiten).
* **Результат:** Готовый UI-каркас: слева дерево каталогов, справа иконки, внизу превью.

### Фаза 7: Construction 4 (macOS Cocoa)
**Цель:** Нативный бэкенд для macOS.
* **Задача 1:** Написать `backends/cocoa` через `purego` и `objc_msgSend`.
* **Задача 2:** Инициализация `NSApplication`, `NSWindow`, `NSButton`.
* **Задача 3:** Отключить AutoLayout (`setTranslatesAutoresizingMaskIntoConstraints:NO`) и двигать контролы через `setFrame:` согласно Cassowary solver.
* **Результат:** Нативная работа на macOS без CGO.

### Фаза 8: Construction 5 (Web Backend / WASM)
**Цель:** Выход в браузер с сохранением нативной модели.
* **Задача 1:** Написать `backends/web` через `syscall/js`.
* **Задача 2:** Маппинг виджетов `goWidgets` на нативные DOM-элементы (`<button>`, `<input>`, `<select>`).
* **Задача 3:** Применение координат из Core через CSS-стили (`position: absolute`).
* **Задача 4:** Опциональная отрисовка через Canvas/Ebiten WebGL.
* **Результат:** Тот же самый UI работает в браузере под WASM.

### Фаза 9: Transition (Полировка Showcase и Унификация)
**Цель:** Релиз-кандидат и финальная унификация с `vtui`.
* **Задача 1:** Сборка showcase-приложения (клон FastStone Lite). Полноценная работа с файловой системой через `Canvas` и `TreeView`.
* **Задача 2:** Реализация механизма "Авто-fallback на Linux" (GTK -> Диалог установки / Fallback на Ebiten).
* **Задача 3:** Финальная сверка API виджетов и реактивного слоя между `vtui` и `goWidgets`.
* **Результат:** Проект полностью кроссплатформенный (Win, Mac, Lin, Web, Terminal) с единым QML-подобным реактивным API.

---

## 5. Инструкции по промптингу для слабых моделей

Когда вы будете давать задачи модели без доступа к сети, используйте следующий формат:

1. **Контекст фазы:** "Мы сейчас находимся в *Фазе 4: Construction 1*. Наш фокус — только Windows Win32 API. Не используй CGO. Используй `golang.org/x/sys/windows`."
2. **Изоляция:** "Не пытайся сейчас реализовать macOS или GTK. Твоя задача реализовать только файл `backend_windows.go` интерфейса `PlatformDriver`."
3. **Ограничение скоупа:** "Не пиши сразу всю систему окон. Напиши только функцию `RegisterClassEx` и `CreateWindowEx` для главного окна."
4. **Контроль архитектуры:** "Помни про Cassowary и `vreactive`. Win32-код не должен сам вычислять размеры или хранить бизнес-логику. Он должен подписываться на изменение `vreactive.Property` и вызывать `MoveWindow` при обновлении `Rect`."

---

## 6. Риски и их локализация (Для ИИ)

* **Риск:** Циклические зависимости в реактивных подписках `vreactive.Property`.
* **Решение:** `vreactive` должен содержать защиту от зацикливания при уведомлениях (guard flag / evaluation depth limit).
* **Риск:** Ebiten (игровой движок) сожрет батарею на перерисовках.
* **Решение:** Ebiten бэкенд должен использовать `ebiten.SetRunGameMode(ebiten.RunGameModeEventDriven)` (отрисовка только при наступлении событий, а не 60 FPS).
* **Риск:** Избыточная нагрузка на DOM при частых анимациях в Web-бэкенде.
* **Решение:** Web-бэкенд должен группировать DOM-обновления через `requestAnimationFrame`.
* **Риск:** Objective-C runtime слишком сложен для LLM.
* **Решение (для Фазы 7):** Разбить фазу 7 на микро-шаги. Сначала написать хелперы `msgSend` для строк, потом для классов, потом только создавать `NSWindow`.
