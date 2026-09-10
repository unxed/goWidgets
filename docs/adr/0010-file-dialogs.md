# ADR-0010. Файловые диалоги: OpenFile / SaveFile

* Статус: принято
* Дата: 2026-09-10
* Затрагивает: §4.3, §4.5

## Решение

`win.OpenFile(title, filters...) (path, ok)` и `win.SaveFile(title,
suggested, filters...) (path, ok)` — модальные, синхронные, как сообщения
(ADR-0009). `FileFilter{Name, Patterns}` — имя для пользователя и glob'ы.
Перезапись существующего файла — вопрос платформы, и она его задаёт
(`OFN_OVERWRITEPROMPT`, `gtk_file_chooser_set_do_overwrite_confirmation`).

* **GTK.** `gtk_file_chooser_dialog_new` вариадический (кнопки через
  varargs) — не через FFI. Диалог собирается из `gtk_dialog_new` +
  `gtk_file_chooser_widget_new` в content area + `gtk_dialog_add_button` с
  переводами GTK (`_Open`/`_Save`/`_Cancel` из домена gtk30). Это тот же
  виджет выбора файла, что у любого GTK-приложения (скриншот под Xvfb: поле
  имени с выделенной основой, фильтр «Текст», Cancel/Save). Путь из
  `gtk_file_chooser_get_filename` копируется и освобождается `g_free`;
  указатель приходит типизированным (`*byte`), без uintptr-кастов.
  `GtkFileChooserNative` (порталы) — не сейчас: его нельзя ответить снаружи в
  тесте, а без портала он всё равно разворачивается в тот же виджет.
* **Win32.** `GetOpenFileNameW` / `GetSaveFileNameW` из comdlg32 с
  `OPENFILENAMEW`, зеркальной структурой; её размер (152) и смещения полей
  проверяются юнит-тестом (`backends/win32/layout_windows_test.go`).
  `OFN_EXPLORER | OFN_NOCHANGEDIR | OFN_PATHMUSTEXIST`, для открытия ещё
  `OFN_FILEMUSTEXIST`.
* **Headless.** `AnswerFiles(paths...)` ("" — отмена), `FileDialogs()`.

## Проверено

* headless: скрипт, отмена, журнал.
* GTK под Xvfb (`dialogtest`): `OpenFile` с файлом, выбранным снаружи через
  `gtk_file_chooser_set_filename`, возвращает его путь; `SaveFile` с
  предложенным именем возвращает `<папка>/новые-цели.txt`.
* Win32 под Wine (`win32test`): диалог найден по заголовку, поле имени
  (`edt1` = 1152) заполнено настоящим файлом, `IDOK` — путь вернулся и
  совпал с файлом.
