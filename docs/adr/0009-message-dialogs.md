# ADR-0009. Модальные сообщения: Message / Confirm / Ask

* Статус: принято
* Дата: 2026-09-10
* Затрагивает: §4.3, §4.5

## Контекст

Второе, чего не хватает простому приложению после ввода текста, — спросить
пользователя «точно?». В crescent — перед удалением отмеченных целей.

## Решение

1. **Три вызова, синхронные, модальные**: `Message(title, text)`,
   `Confirm(title, text) bool` (OK/Cancel), `Ask(title, text) bool` (Yes/No).
   Блокируют UI-горутину во вложенном цикле платформы; очередь `QueueUpdate`
   при этом продолжает обрабатываться (GTK — `g_idle_add`, Win32 — сообщения
   окну-почтальону идут и из цикла `MessageBox`). Любой выход из окна, кроме
   явного согласия — Escape, крестик, вторая кнопка — читается как
   осторожный ответ: Cancel/No.
2. **Кнопки — платформы, на языке пользователя.** Win32: `MessageBoxW`.
   GTK: `gtk_message_dialog_new` вариадический (printf-формат) — через FFI его
   не тащим; вместо него `gtk_dialog_new` + `gtk_dialog_add_button` +
   `gtk_dialog_run`, все не-вариадические, а подписи — переводы самого GTK:
   `g_dgettext("gtk30", "_Yes")` и т.д. Под Xvfb без локали это «No/Yes», на
   русском десктопе — «Нет/Да», как у любого GTK-диалога.
3. **Headless**: `AnswerDialogs(...)` скриптует ответы, `Dialogs()` перечисляет
   показанные окна; без скрипта — осторожный ответ.
4. В `core.BackendWindow` — один метод `Dialog(kind, title, text)
   DialogResult` с тремя видами; произвольные наборы кнопок не нужны и на
   Win32 стоили бы `TaskDialogIndirect` с манифестом comctl32 v6.

## Проверено

* headless: скриптованные ответы доходят, без скрипта — Cancel/No, журнал.
* GTK под Xvfb (`backends/gtk/dialogtest`, свой бинарник): `Ask` и `Confirm`
  подряд из одного `QueueUpdate`; ответ приходит снаружи через
  `gtk_dialog_response`, поданный тоже через `QueueUpdate` — то есть очередь
  реально работает под `gtk_dialog_run`. Accept → true, reject → false.
* Win32 под Wine (`win32test`): `Ask` из `QueueUpdate`, окно найдено по
  заголовку `FindWindowW`, отвечено `WM_COMMAND IDYES`, вернулось `true`.
* Скриншоты showcase: диалог на GTK и MessageBox под Wine, Enter → цель убрана,
  колонка схлопнулась.
