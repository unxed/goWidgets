# Design Document: `goWidgets` (Cross-Platform Go Native UI) — v2

| | |
|---|---|
| **Статус** | Draft v2 (замещает v1 «goWidgets») |
| **Репозиторий** | `goWidgets`; модуль и импорт-путь — `.../goWidgets` (см. ADR-0001) |
| **Аудитория** | LLM-исполнители (в т.ч. без доступа к сети) + ревьюер-человек |
| **Методология** | RUP, итеративно, риск-ориентированно (сначала — самый рискованный слой) |

> **Как читать этот документ.** Разделы 3–7 — **нормативные контракты**. LLM не имеет права
> их менять; при конфликте задачи и контракта — задача останавливается и фиксируется в
> «Открытых вопросах» (§12). Разделы 8–9 — процесс. Раздел 11 — реестр рисков.

---

## 0. Глоссарий, условия, журнал решений

**Глоссарий (обязателен к единообразному использованию):**

* **Node** — внутреннее представление виджета в Core (не путать с публичным `Widget`).
* **Handle** — непрозрачный `uint64`-идентификатор Node, общий язык Core↔Backend.
* **DIP** — device-independent pixel (логическая единица, `float64`). Все координаты в
  Core — в DIP. Перевод в физические пиксели — **только в бэкенде**.
* **Intrinsic size** — «естественный» размер виджета, который умеет посчитать **только**
  платформа (метрики шрифта, паддинги темы).
* **Frame** — один проход конвейера §6.
* **Driver** — реализация `PlatformDriver` (win32 / gtk / cocoa / web / ebiten / headless).

**Изначально заданные условия:**

* `CGO_ENABLED=0` как жёсткий инвариант. Внешние библиотеки - только через ffi.
* Cassowary вместо нативных layout-менеджеров.
* единицы измерения — DIP `float64`, DPI-скейлинг в бэкенде.

**ADR-лог (Architecture Decision Records).** Каждое архитектурное решение фиксируется файлом
`docs/adr/NNNN-title.md` (контекст → решение → последствия → альтернативы).

---

## 1. Концепция, цели и границы

**`goWidgets`** — кроссплатформенная GUI-библиотека для Go: десктоп и веб без `CGO`, с нативными
контролами и реактивным QML-подобным API, унифицированным с TUI-библиотекой [vtui](https://github.com/unxed/vtui).

### 1.1. Цели (что считаем успехом)

1. Один и тот же декларативный UI-код работает в терминале (`vtui`), на Win32/GTK/Cocoa и в
   браузере (WASM) — **без `//go:build`-веток в пользовательском коде**.
2. Сборка `CGO_ENABLED=0` для всех целевых платформ.
3. Реактивность (`Property`, `Bind`, `Computed`, `Behavior`, `StateMachine`) — в ядре.

### 1.2. Non-goals (явно вне скоупа v1.0)

Чтобы LLM не расползалась: в рамках этого документа **не делаем** мобильные платформы (Android/iOS),
3D, встроенный HTML-рендер, темизацию «как в Qt Style Sheets», i18n/RTL-раскладку, плагинную систему,
собственный векторный текстовый движок. Доступность (a11y) — минимальный уровень (§4.6),
не полный.

### 1.3. Нефункциональные требования (измеримые — иначе не проверить)

| NFR | Целевое значение | Как измеряем |
|---|---|---|
| Cold start (пустое окно) | < 150 мс на desktop | `bench/startup` |
| Размер бинаря showcase | < 25 МБ (desktop), < 8 МБ gzip (WASM) | CI-артефакт |
| Пересчёт layout, 500 узлов | < 4 мс (p95) | `bench/layout` |
| Простой UI (нет событий) | 0% CPU, 0 перерисовок | `bench/idle` |
| Утечки подписок | 0 после уничтожения окна | `TestNoLeakedSubscriptions` |

> **Правило:** любая фаза, ухудшающая NFR более чем на 20%, не закрывается.

---

## 2. Базовые принципы (строгие правила для ИИ)

1. **Никакого `CGO`.** Весь FFI — `syscall`/`golang.org/x/sys/windows` (Windows),
   `syscall/js` (Web), `purego` (macOS, Linux/GTK). `qt.go` исключён.
2. **Публичный API скрывает платформу** и построен на `vreactive.Property[T]`.
3. **Единый layout-движок.** Нативные sizers/AutoLayout не используются; Core решает
   Cassowary и отдаёт абсолютные прямоугольники. **Но** — Core обязан спрашивать у бэкенда
   intrinsic-размеры (§5.1), иначе текстовые виджеты нельзя разместить корректно.
4. **Приоритет нативных контролов** там, где это не ломает контракт абсолютного позиционирования.
5. **Canvas / Ebiten — равноправная опция** (fallback + графические виджеты).
6. **Реактивность и анимации — в Core**; бэкенды получают готовые значения.
7. **Итеративность (showcase-driven):** пишем только то, что нужно клону FastStone Image Viewer.
8. **Ничего не изобретаем молча.** Любое отклонение от контрактов §4 — сначала ADR.

---

## 3. Архитектура

Три слоя + два общих пакета. Протечки абстракций запрещены; проверяются линтером
`depguard` (правила в `.golangci.yml`) — **архитектура должна быть исполняемой**.

```
   goWidgets (Public API, Уровень 1)      ← пользователь видит только это
        │  Widget-дерево, декларативные констрейнты, Property
   core (Уровень 2)                  ← Node-граф, Solver, Animator, EventRouter, Scheduler
        │  Handle + Rect (DIP), батч-команды
   backends/* (Уровень 3)            ← win32 | gtk | cocoa | web | ebiten | headless
```

Общие пакеты: **`vreactive`** (реактивность, разделяется с `vtui`) и **`vcontract`**
(конформанс-тесты и общий словарь виджетов `vtui`↔`goWidgets`).

### 3.1. `vreactive`

* `Property[T]` — значение + подписки + батчинг.
* `Bind` / `Computed` — производные значения с **явным** объявлением зависимостей
  (авто-трекинг зависимостей запрещён в v1: слабая LLM его не отладит).
* `Behavior[T]` — перехват изменения свойства и запуск интерполяции.
* `StateMachine` — декларативные состояния и переходы.
* `Animator` — интерфейс анимации (дискретный для TUI, плавный для GUI/Web).
* `Event[T]` — мультикаст-сигнал c unsubscribe (заменяет одиночный `OnClick func()`).
* `Scope` — владелец подписок; `scope.Close()` отписывает всё разом (борьба с утечками).

### 3.2. Core

* **Node Registry** — `Handle → Node`, дерево, порядок детей, z-order, жизненный цикл.
* **Layout Solver** — Cassowary (`kiwi-go`), инкрементальный (edit-переменные), §5.
* **Animation Engine** — тики, интерполяция, объединение с батчем свойств.
* **Event Router** — платформенные события → абстрактные; фокус, tab-order, hit-test.
* **Scheduler / App Context** — event loop, main-thread affinity, `QueueUpdate`, §6.

### 3.3. Backends

Каждый драйвер реализует `PlatformDriver` + `Capabilities()`. Обязательный минимум для
всех: создание окна, создание/уничтожение виджета, `SetBounds`, `MeasureIntrinsic`,
доставка событий. Остальное — опционально и объявляется через capability-флаги.

**Порядок выбора драйвера** (детерминированный, тестируемый):

1. `goWidgets_BACKEND=<name>` — жёсткий выбор; при неудаче **ошибка**, а не тихий fallback.
2. Иначе — платформенный дефолт (windows→win32, darwin→cocoa, js→web, linux→gtk).
3. При ошибке инициализации — `ebiten`, затем `headless` (в CI).
4. Каждый шаг логируется в `goWidgets.Diagnostics()` — пользователь должен уметь ответить
   на вопрос «почему у меня не нативный вид».

---

## 4. Контракты API (нормативно)

### 4.1. Единицы измерения и DPI

```go
type Point struct{ X, Y float64 }   // DIP
type Size  struct{ W, H float64 }   // DIP
type Rect  struct{ X, Y, W, H float64 }
type Insets struct{ L, T, R, B float64 }

// Масштаб окна. Меняется при переносе окна между мониторами.
type ScaleInfo struct {
    Scale     float64 // 1.0, 1.5, 2.0 ...
    FontScale float64 // системное укрупнение шрифта
}
```

> В v1 `Rect` был `int` без DPI — это ломало Retina, per-monitor DPI на Windows и
> `devicePixelRatio` в вебе. Округление до физических пикселей делает **бэкенд**.

### 4.2. Реактивный слой

```go
type Property[T any] interface {
    Get() T
    Set(v T)
    Update(f func(T) T)                       // атомарное чтение-модификация
    OnChange(s *Scope, h func(new, old T))    // подписка живёт в Scope
}

type Event[T any] interface {
    Emit(v T)
    On(s *Scope, h func(T))
}

// Батч: подписчики уведомляются один раз, после Commit — нет «глитчей».
func Batch(f func())
```

**Инварианты `vreactive`:**
* `Set` вне main-thread — паника в debug-сборке (`-tags goWidgets_debug`), см. §4.5.
* Уведомления — после завершения батча, в топологическом порядке зависимостей.
* Глубина каскада ограничена (`MaxPropagationDepth = 64`) → ошибка, а не зависание.
* Повторный `Set` тем же значением (`==`-сравнимые типы) не порождает уведомления.

### 4.3. Публичный API

```go
type Widget interface {
    ID() string
    SetID(string)
    Constraints() []Constraint
    SetConstraints(...Constraint)
    Children() []Widget
    Scope() *vreactive.Scope
}

type Button struct {
    Text    Property[string]
    Enabled Property[bool]
    Visible Property[bool]
    Clicked Event[ClickInfo]   // вместо OnClick func()
}

type Window interface {
    Widget
    Title   Property[string]
    Content Property[Widget]
    Scale   Property[ScaleInfo] // read-only для пользователя
    Show()
    Close()
    Closing Event[*CloseRequest] // возможность отменить закрытие
}

type App interface {
    Run(main Window) error
    QueueUpdate(f func())              // из любой горутины
    Post(d time.Duration, f func()) Cancel
    Diagnostics() DriverInfo
}
```

### 4.4. Backend API (скрыт от пользователя)

```go
type Handle uint64

type PlatformDriver interface {
    Init() error
    Capabilities() Caps
    RunMainLoop(ctx context.Context) error
    CreateWindow(spec WindowSpec) (BackendWindow, error)
    Shutdown()
}

type BackendWindow interface {
    SetTitle(string)
    Show(); Close()
    Scale() ScaleInfo

    // Жизненный цикл виджетов — в v1 этого не было вовсе.
    CreateWidget(kind WidgetKind, parent Handle) (Handle, error)
    DestroyWidget(h Handle)
    SetParent(child, parent Handle, index int)

    // Свойства: типизированный, но узкий канал (без reflect).
    SetString(h Handle, p PropKey, v string)
    SetBool(h Handle, p PropKey, v bool)
    SetFloat(h Handle, p PropKey, v float64)

    // Измерение: единственный источник правды о «естественном» размере.
    MeasureIntrinsic(h Handle, avail Size) (min, natural Size)

    // Применение layout — батчем и только для изменившихся узлов.
    ApplyLayout(changes []BoundsChange)

    Events() <-chan BackendEvent
}

type BoundsChange struct { H Handle; R Rect; Visible bool }

type Caps struct {
    NativeControls bool
    TreeView, GridView, FileDialog, Menus, Clipboard, IME, A11y bool
    SmoothAnimation bool
    MaxCallbacks   int
}
```

> Ключевые исправления против v1: появились `CreateWidget`/`DestroyWidget`,
> `MeasureIntrinsic`, `Caps`; `UpdateLayout(map[string]Rect)` заменён на батч-диф по
> `Handle` (строковые ID хрупки и требуют уникальности во всём приложении).

### 4.5. Потоковая модель (в v1 отсутствовала — источник 90% будущих багов)

1. Main goroutine пинится `runtime.LockOSThread()` до `Init()` — обязательно для Cocoa и Win32.
2. **Весь** доступ к `Property` и виджетам — только из main-thread.
3. Горутины взаимодействуют через `App.QueueUpdate`. Очередь — неблокирующая, с бэкпрешером.
4. WASM однопоточен: `QueueUpdate` = микротаск; блокирующие вызовы запрещены.
5. Debug-сборка проверяет affinity и падает с понятным сообщением.

### 4.6. Ввод, фокус и минимальная доступность

Обязательный минимум v1: клавиатурный фокус и tab-order (Core, не платформа), Enter/Escape
как default/cancel, hit-test с учётом z-order, колесо мыши/скролл, буфер обмена (текст),
системные диалоги открытия файла (нужны showcase). IME и полноценный a11y — за
capability-флагом, допускается «не поддерживается» в v1.

---

## 5. Layout: контракт вычислений

### 5.1. Двухфазность (measure → solve)

Cassowary не знает, сколько места занимает надпись «Отмена» шрифтом системной темы при
150% DPI. Поэтому цикл строго двухфазный:

1. **Measure.** Для узлов с флагом `DirtyMeasure` Core вызывает `MeasureIntrinsic` и
   кэширует `(min, natural)`. Кэш инвалидируется при смене текста, шрифта, темы, DPI.
2. **Solve.** `min`/`natural` входят в систему как констрейнты:
   `w >= min.W` (required), `w == natural.W` (strength: weak).
3. **Apply.** Диф прямоугольников → `ApplyLayout`.

### 5.2. Инкрементальность и производительность

* Размеры окна и анимируемые величины подаются через **edit-переменные** (`suggestValue`),
  без пересборки системы уравнений.
* Анимация свойства, не влияющего на геометрию (цвет, прозрачность), **не запускает** solve.
* Поддеревья с фиксированной геометрией кэшируются (`LayoutBoundary`).

### 5.3. Конфликты констрейнтов

Иерархия strength: `required > strong > medium > weak`. Пользовательские констрейнты по
умолчанию `strong`, естественные размеры — `weak`. При неразрешимой системе:
в debug — паника с текстовым дампом конфликтующего набора; в release — конфликтующий
констрейнт отбрасывается, событие уходит в `Diagnostics()`. **Молчаливое зависание запрещено.**

### 5.4. Скролл и клиппинг

Абсолютное позиционирование + нативные контролы плохо дружат с прокруткой. Решение:
`ScrollArea` — узел-граница layout (`LayoutBoundary`), внутри которого координаты
считаются относительно контента, а бэкенд применяет клиппинг (Win32: child-окно;
GTK: `GtkScrolledWindow`+`GtkFixed`; Web: `overflow:hidden` + `transform: translate`).
Виртуализация (только видимые элементы) обязательна для `GridView` миниатюр.

---

## 6. Конвейер кадра (нормативный порядок)

```
1. Backend events      → EventRouter → абстрактные события
2. Обработчики         → Property.Set внутри Batch
3. Animation tick      → Behavior интерполирует → Property.Set
4. Propagate           → Computed/Bind, dirty-флаги (Value|Measure|Layout|Paint)
5. Measure             → MeasureIntrinsic только для DirtyMeasure
6. Solve               → Cassowary, только если есть DirtyLayout
7. Apply               → ApplyLayout(diff)
8. Paint               → только Canvas-узлы
```

**Idle-правило:** нет событий и нет активных анимаций → шагов 3–8 не происходит,
цикл блокируется на ожидании события (не 60 FPS).

---

## 7. Расположение кода

```
/vreactive         — общий с vtui, без импортов goWidgets
/vcontract         — конформанс-тесты и общий словарь виджетов
/core              — node registry, solver, router, scheduler
/goWidgets              — публичный API
/backends/headless — референс + тесты (Фаза 0!)
/backends/{win32,gtk,cocoa,web,ebiten}
/showcase/faststone
/docs/adr
/bench
```

Правило импортов: `backends/* → core` разрешено, `core → backends/*` **запрещено**
(регистрация драйверов через `init()` + build-tags).

---

## 8. План реализации: итерации RUP с критериями выхода

> **Изменение против v1:** добавлена Фаза 0; порядок пересобран по риску — самый опасный
> элемент (FFI без CGO + intrinsic-измерения) проверяется спайком **до** массовой разработки,
> а не в Фазе 7. Каждая фаза имеет проверяемый Definition of Done.

### Фаза 0. Inception: каркас, CI и разведка рисков
* Репозиторий, модуль, `.golangci.yml` с правилами слоёв, CI-матрица (win/mac/linux/wasm).
* **Спайк-проверки** (по 50–100 строк, результат — ADR): Ebiten без CGO на каждой ОС;
  версия GTK у `puregotk`; `objc_msgSend` через `purego` — создать `NSWindow` и закрыть;
  лимиты `windows.NewCallback`; статус и лицензия `kiwi-go`.
* **`backends/headless`** — эталонный драйвер: детерминированные `MeasureIntrinsic`,
  запись всех `ApplyLayout` в лог для golden-тестов.
* **DoD:** CI зелёный на 4 платформах; ADR-0002 подтверждает или корректирует «no CGO».

### Фаза 1. Reactive Core
* `vreactive`: `Property`, `Event`, `Scope`, `Batch`, `Bind`, `Computed`, защита от циклов.
* `core`: node registry, dirty-флаги, scheduler, конвейер §6 (без Cassowary — фиксированные размеры).
* `goWidgets.Button`, Ebiten-бэкенд.
* **DoD:** `Text.Set()` из горутины через `QueueUpdate` меняет надпись; тест на утечки
  подписок зелёный; headless-golden-тест конвейера.

### Фаза 2. Layout Engine
* `kiwi-go`, двухфазность measure→solve, edit-переменные, `VBox`/`HBox`/`Anchor`,
  стратегия конфликтов §5.3.
* **DoD:** `bench/layout` укладывается в NFR; тест «неразрешимая система → диагностика, не паника».

### Фаза 3. Первый нативный бэкенд (Win32 **или** GTK — выбрать по доступности CI)
* `PlatformDriver`, `WNDCLASSEX`/`GetMessage` или GTK4 + `GtkFixed`, `MeasureIntrinsic`
  через реальные метрики темы, DPI-скейлинг.
* **DoD:** тот же `main.go`, что в Фазе 1, запускается нативно; `ApplyLayout` совпадает с
  headless-golden в пределах допуска; смена DPI перерисовывает корректно.

### Фаза 4. Второй нативный бэкенд + анимации
* `Behavior`, `Animator`, `StateMachine`; второй драйвер из пары Win32/GTK.
* **DoD:** анимация ширины 60 FPS без пересборки системы уравнений; idle = 0% CPU.

### Фаза 5. Базовые виджеты
* `Label`, `TextInput`, `CheckBox`, `Splitter`, фокус/tab-order, буфер обмена, файловый диалог.
* **DoD:** «Окно настроек» идентично на 3 бэкендах (скриншот-диф + golden layout).

### Фаза 6. Тяжёлые виджеты
* `TreeView`, `GridView` (виртуализация), `Canvas`, `ScrollArea`.
* **DoD:** 10 000 миниатюр — плавный скролл, память < заданного порога.

### Фаза 7. Cocoa (микро-шагами)
* Хелперы `msgSend` (строки → классы → селекторы) → `NSApplication` → `NSWindow` → `NSButton`
  → отключение AutoLayout → `setFrame:`.
* **DoD:** showcase запускается на macOS, `CGO_ENABLED=0`.

### Фаза 8. Web / WASM
* `syscall/js`, DOM-маппинг, координаты через `transform: translate3d` (дешевле, чем `left/top`),
  батчинг в `requestAnimationFrame`, `devicePixelRatio`.
* **DoD:** размер бандла в пределах NFR; тот же UI работает в браузере.

### Фаза 9. Transition
* Showcase FastStone Image Viewer, унификация API с `vtui` через `vcontract`, документация,
  политика fallback §3.3, релизная сборка.
* **DoD:** конформанс-набор `vcontract` проходит одинаково на `vtui` и `goWidgets`.

---

## 9. Тестирование (в v1 раздела не было)

1. **Golden layout tests** — headless-драйвер пишет `[]BoundsChange`, сравнение с эталоном.
   Это главный инструмент самопроверки LLM: изменение поведения layout видно мгновенно.
2. **Конформанс-набор `vcontract`** — один и тот же сценарий гоняется на всех драйверах;
   отличия допустимы только там, где `Caps` объявляет отсутствие фичи.
3. **Property-based тесты `vreactive`** — случайные графы биндингов: нет глитчей, нет циклов,
   нет утечек `Scope`.
4. **Fuzz на Cassowary-вход** — случайные наборы констрейнтов не должны вешать solver.
5. **CI-матрица** — сборка `CGO_ENABLED=0` для всех GOOS/GOARCH на каждом PR.

---

## 10. Инструкции по промптингу слабых моделей

Шаблон задачи (заполнять целиком, не сокращать):

```
[ФАЗА]      Фаза 3, задача 2.
[ФАЙЛЫ]     Разрешено менять только backends/win32/window.go. Остальное — read-only.
[КОНТРАКТ]  Реализуй PlatformDriver.CreateWindow и BackendWindow.CreateWidget (см. §4.4).
            Сигнатуры менять запрещено.
[ЗАПРЕТЫ]   Без CGO. Без вычисления размеров внутри бэкенда. Без хранения бизнес-логики.
            Не трогай macOS/GTK/Web.
[ИНВАРИАНТЫ] Координаты приходят готовыми в DIP; масштабирование в px — здесь.
            Все вызовы — из main-thread.
[ПРИЁМКА]   go build CGO_ENABLED=0 GOOS=windows; тест TestWin32CreateWindow зелёный;
            golden-тест layout не изменился.
[ФОРМАТ]    Верни только полный текст изменённого файла и список новых импортов.
```

Дополнительные правила:
* **Одна задача — один файл.** Если модель хочет менять контракт — она обязана остановиться
  и написать заявку в §12, а не «немного поправить интерфейс».
* Перед кодом модель формулирует 3–5 строк плана; после кода — самопроверку по «Приёмке».
* Запрещено вводить новые зависимости без ADR.

---

## 11. Реестр рисков

| # | Риск | Триггер / как заметим | Митигация | План B |
|---|---|---|---|---|
| R1 | Зависимость тянет CGO | CI-сборка `CGO_ENABLED=0` падает | Спайк в Фазе 0 | Замена библиотеки / build-tag `goWidgets_cgo`, драйвер вне fallback-цепочки |
| R2 | Циклы в реактивных подписках | `MaxPropagationDepth` превышен | Батчинг + топологический порядок + лимит глубины | Ошибка с дампом цепочки, а не зависание |
| R3 | Нет intrinsic-размеров → «поехавший» текст | Golden-тесты расходятся между драйверами | `MeasureIntrinsic` как обязательный контракт §5.1 | Fallback-метрики моноширинного шрифта |
| R4 | Cassowary тормозит на анимациях | `bench/layout` вне NFR | Edit-переменные, `LayoutBoundary`, отделение не-геометрических анимаций | Прямой `SetBounds` мимо solver для анимируемых поддеревьев |
| R5 | Ebiten жрёт батарею | `bench/idle` > 0% | Event-driven режим отрисовки | Ограничение FPS + ручная инвалидация |
| R6 | DOM-шторм в Web | Профиль браузера | Батч в `requestAnimationFrame`, `transform` вместо `left/top` | Переход на Canvas-рендер для «горячих» областей |
| R7 | Objective-C runtime слишком сложен для LLM | Фаза 7 буксует > 2 итераций | Микро-шаги §8 | Временный Ebiten-драйвер на macOS |
| R8 | Абсолютное позиционирование ломает скролл/a11y | Ручная проверка с клавиатуры и скринридером | `ScrollArea` §5.4, фокус в Core §4.6 | Явное объявление ограничений в `Caps` |
| R9 | API `vtui` и `goWidgets` расходятся | Конформанс-тесты `vcontract` краснеют | Общий пакет + тесты с Фазы 1 | Слой адаптеров |
| R10 | Утечки подписок при уничтожении виджетов | `TestNoLeakedSubscriptions` | `Scope` владеет подписками | — |
| R11 | Незрелость `puregotk`/`kiwi-go` (форк, мало коммитов) | Баг без апстрим-фикса | Пин версий + vendoring + ADR | Вендоринг и локальный форк в `third_party/` |

---

## 12. Открытые вопросы (заполняется по ходу, не игнорируется)

1. GTK3 или GTK4 — зависит от результата спайка Фазы 0.
- GTK4

2. Нужна ли темизация/стилизация нативных контролов в v1 или это v2?
- не в первую очередь

3. Формат ресурсов (иконки, изображения) и их встраивание в WASM-бандл.
4. Модель рисования `Canvas`: единый immediate-mode API или per-backend?
5. Стратегия многооконности и модальных диалогов.
