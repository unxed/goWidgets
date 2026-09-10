# ADR-0011. ComboBox, и имена по словарю vtui

* Статус: принято
* Дата: 2026-09-10
* Затрагивает: §1 (унификация с vtui), §4.3, §8 (vcontract)

## Контекст

Дизайн-док обещает единый словарь виджетов с `vtui` через `vcontract`
(§8, отдельная фаза). Словарь уже существует — `vocabulary.json` в vtui:
имена виджетов, свойств и сигналов. goWidgets ему не следовал буквально;
в частности, текстовое поле здесь называлось `Entry` (GTK-шное слово), а в
словаре это `Edit` с сигналами `changed`/`activated`.

## Решение

1. **`Entry` → `Edit`** сейчас, пока это дёшево: единственный пользователь —
   showcase, патчи применяются сразу. `AddEdit`, `Edit`, headless-хелперы
   `TypeIntoEdit`/`PressEnterInEdit`/`EditText`, каталог теста `edittest`.
   GTK-шные имена внутри бэкенда (`gtkEntryNew`, `cbEntryChanged`) остаются —
   это имена GTK.
2. **`ComboBox` по словарю**: свойства `text` (двусторонний `Text`), `items`
   (`SetItems`/`Items`), `dropdownOnly` (аргумент `AddComboBox`); сигналы
   `changed` (`Changed(string)` — набор и выбор одинаково) и `selected`
   (`Selected(int)` — выбор из списка). Сверх словаря: `Select(i)` и
   `SelectedIndex()` — индекс нужен приложению, а не только текст.
3. **`Selected` — только на смену выбора пользователем.** GTK сообщает
   программный `set_active` тем же `changed` (наблюдалось: тест получил
   `[0 2]`); как и с текстом, платформа, держащая то, что ей записали, —
   не выбор. Headless теперь отвечает событием на программный `Select`, чтобы
   тест это видел.
4. **Платформы.** GTK — `GtkComboBoxText` с entry или без; вариант
   выбирается при создании, поэтому `PropDropdownOnly` пересоздаёт виджет (до
   элементов и раскладки). Один сигнал `changed` покрывает и набор, и выбор:
   `gtk_combo_box_get_active` = -1 значит набор. Win32 — `COMBOBOX`
   `CBS_DROPDOWN`/`CBS_DROPDOWNLIST`; `CBN_SELCHANGE`/`CBN_EDITCHANGE`.
   Высота COMBOBOX — это высота *раскрытого* списка, поэтому `ApplyLayout`
   добавляет к высоте раскладки восемь строк списка; закрытый контрол
   остаётся одной строкой. Ширина — по самому широкому элементу.

## Что ещё расходится со словарём (для прохода vcontract, не сейчас)

* `CheckBox` vs словарное `Checkbox`; `Checked`/`Toggled` vs `state`/`changed`
  (и `threeState`). Существовало до этой серии патчей.
* `Button.command`, `Widget.help`, `Label.buddy`, `Edit.password/historyId` —
  свойства словаря, которых здесь нет; `HugWidth`, констрейнты — наоборот.
* Конструкторы: в vtui `NewComboBox(x, y, width, items)` — императивный
  `ScreenObject`; здесь `win.AddComboBox(items, dropdownOnly)` и раскладка
  констрейнтами. Единый *код* UI (§1.1) — это про декларативный слой поверх
  словаря, а не про совпадение Go-сигнатур.

## Проверено

* headless: обе стороны, `Select` не репортится, `SetItems` сбрасывает
  выбор, golden.
* GTK под Xvfb (`combotest`): выбор снаружи `gtk_combo_box_set_active` →
  `Selected`/`Text`/`Changed`; набор в дочернем `GtkEntry` → `Changed`, без
  `Selected`; программный `Select(0)` дошёл до виджета и не вернулся выбором.
* Win32 под Wine (`win32test`): `CB_SETCURSEL` + `CBN_SELCHANGE` → `Selected`
  [2], `Text`; showcase с клавиатуры: Tab на список, Down, цель добавлена с
  `[gpt-5.6]`.
* Showcase на GTK — то же.
