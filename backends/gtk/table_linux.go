//go:build linux

package gtk

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/unxed/goWidgets/core"
)

var (
	gtkTreeViewNew                func() uintptr
	gtkTreeViewSetModel           func(view, model uintptr)
	gtkTreeViewGetSelection       func(view uintptr) uintptr
	gtkTreeViewColumnNew          func() uintptr
	gtkTreeViewAppendColumn       func(view, column uintptr) int32
	gtkTreeViewRemoveColumn       func(view, column uintptr) int32
	gtkTreeViewGetColumn          func(view uintptr, index int32) uintptr
	gtkTreeSelectionSetMode       func(selection uintptr, mode int32)
	gtkTreeSelectionGetSelected   func(selection uintptr, model *uintptr, iter *gtkTreeIter) int32
	gtkTreeSelectionSelectPath    func(selection, path uintptr)
	gtkTreeSelectionUnselectAll   func(selection uintptr)
	gtkTreePathNewFromString      func(path string) uintptr
	gtkTreePathGetIndices         func(path uintptr) *int32
	gtkTreePathFree               func(path uintptr)
	gtkTreeModelGetPath           func(model uintptr, iter *gtkTreeIter) uintptr
	gtkListStoreNewv              func(columns int32, types *uintptr) uintptr
	gtkListStoreClear             func(store uintptr)
	gtkListStoreAppend            func(store uintptr, iter *gtkTreeIter)
	gtkListStoreSetValue          func(store uintptr, iter *gtkTreeIter, column int32, value *gtkGValue)
	gtkTreeViewColumnSetTitle     func(column uintptr, title string)
	gtkTreeViewColumnSetClickable func(column uintptr, clickable int32)
	gtkTreeViewColumnSetAlignment func(column uintptr, alignment float32)
	gtkTreeViewColumnSetSortInd   func(column uintptr, setting int32)
	gtkTreeViewColumnSetSortOrder func(column uintptr, order int32)
	gtkTreeViewColumnPackStart    func(column, renderer uintptr, expand int32)
	gtkTreeViewColumnAddAttribute func(column, renderer uintptr, attribute string, modelColumn int32)
	gtkCellRendererTextNew        func() uintptr
	gtkCellRendererSetAlignment   func(renderer uintptr, xalign, yalign float32)
	gtkTreeViewRowActivated       func(view, path, column uintptr)
	gtkTreeViewColumnClicked      func(column uintptr)
	gTypeFromName                 func(name string) uintptr
	gValueInit                    func(value *gtkGValue, typ uintptr) *gtkGValue
	gValueSetString               func(value *gtkGValue, text string)
	gValueUnset                   func(value *gtkGValue)
	gObjectUnref                  func(object uintptr)
)

// gtkTreeIter and gtkGValue mirror GTK's pointer-free public ABI structs.
// GTK retains neither value: list-store setters copy the string into the model.
type gtkTreeIter struct {
	Stamp     int32
	UserData  uintptr
	UserData2 uintptr
	UserData3 uintptr
}

type gtkGValue struct {
	Type uintptr
	Data [2]uintptr
}

func (d *driver) initTable(gtkLib, gobjectLib uintptr) error {
	purego.RegisterLibFunc(&gtkTreeViewNew, gtkLib, "gtk_tree_view_new")
	purego.RegisterLibFunc(&gtkTreeViewSetModel, gtkLib, "gtk_tree_view_set_model")
	purego.RegisterLibFunc(&gtkTreeViewGetSelection, gtkLib, "gtk_tree_view_get_selection")
	purego.RegisterLibFunc(&gtkTreeViewColumnNew, gtkLib, "gtk_tree_view_column_new")
	purego.RegisterLibFunc(&gtkTreeViewAppendColumn, gtkLib, "gtk_tree_view_append_column")
	purego.RegisterLibFunc(&gtkTreeViewRemoveColumn, gtkLib, "gtk_tree_view_remove_column")
	purego.RegisterLibFunc(&gtkTreeViewGetColumn, gtkLib, "gtk_tree_view_get_column")
	purego.RegisterLibFunc(&gtkTreeSelectionSetMode, gtkLib, "gtk_tree_selection_set_mode")
	purego.RegisterLibFunc(&gtkTreeSelectionGetSelected, gtkLib, "gtk_tree_selection_get_selected")
	purego.RegisterLibFunc(&gtkTreeSelectionSelectPath, gtkLib, "gtk_tree_selection_select_path")
	purego.RegisterLibFunc(&gtkTreeSelectionUnselectAll, gtkLib, "gtk_tree_selection_unselect_all")
	purego.RegisterLibFunc(&gtkTreePathNewFromString, gtkLib, "gtk_tree_path_new_from_string")
	purego.RegisterLibFunc(&gtkTreePathGetIndices, gtkLib, "gtk_tree_path_get_indices")
	purego.RegisterLibFunc(&gtkTreePathFree, gtkLib, "gtk_tree_path_free")
	purego.RegisterLibFunc(&gtkTreeModelGetPath, gtkLib, "gtk_tree_model_get_path")
	purego.RegisterLibFunc(&gtkListStoreNewv, gtkLib, "gtk_list_store_newv")
	purego.RegisterLibFunc(&gtkListStoreClear, gtkLib, "gtk_list_store_clear")
	purego.RegisterLibFunc(&gtkListStoreAppend, gtkLib, "gtk_list_store_append")
	purego.RegisterLibFunc(&gtkListStoreSetValue, gtkLib, "gtk_list_store_set_value")
	purego.RegisterLibFunc(&gtkTreeViewColumnSetTitle, gtkLib, "gtk_tree_view_column_set_title")
	purego.RegisterLibFunc(&gtkTreeViewColumnSetClickable, gtkLib, "gtk_tree_view_column_set_clickable")
	purego.RegisterLibFunc(&gtkTreeViewColumnSetAlignment, gtkLib, "gtk_tree_view_column_set_alignment")
	purego.RegisterLibFunc(&gtkTreeViewColumnSetSortInd, gtkLib, "gtk_tree_view_column_set_sort_indicator")
	purego.RegisterLibFunc(&gtkTreeViewColumnSetSortOrder, gtkLib, "gtk_tree_view_column_set_sort_order")
	purego.RegisterLibFunc(&gtkTreeViewColumnPackStart, gtkLib, "gtk_tree_view_column_pack_start")
	purego.RegisterLibFunc(&gtkTreeViewColumnAddAttribute, gtkLib, "gtk_tree_view_column_add_attribute")
	purego.RegisterLibFunc(&gtkCellRendererTextNew, gtkLib, "gtk_cell_renderer_text_new")
	purego.RegisterLibFunc(&gtkCellRendererSetAlignment, gtkLib, "gtk_cell_renderer_set_alignment")
	purego.RegisterLibFunc(&gtkTreeViewRowActivated, gtkLib, "gtk_tree_view_row_activated")
	purego.RegisterLibFunc(&gtkTreeViewColumnClicked, gtkLib, "gtk_tree_view_column_clicked")
	purego.RegisterLibFunc(&gTypeFromName, gobjectLib, "g_type_from_name")
	purego.RegisterLibFunc(&gValueInit, gobjectLib, "g_value_init")
	purego.RegisterLibFunc(&gValueSetString, gobjectLib, "g_value_set_string")
	purego.RegisterLibFunc(&gValueUnset, gobjectLib, "g_value_unset")
	purego.RegisterLibFunc(&gObjectUnref, gobjectLib, "g_object_unref")

	d.cbTableSelected = purego.NewCallback(func(selection, data uintptr) uintptr {
		h := core.Handle(data)
		var model uintptr
		var iter gtkTreeIter
		if gtkTreeSelectionGetSelected(selection, &model, &iter) == 0 || model == 0 {
			return 0
		}
		path := gtkTreeModelGetPath(model, &iter)
		if path == 0 {
			return 0
		}
		indices := gtkTreePathGetIndices(path)
		if indices != nil {
			emit(core.BackendEvent{Kind: core.EventSelected, H: h, Int: int(*indices)})
		}
		gtkTreePathFree(path)
		return 0
	})
	d.cbTableActivated = purego.NewCallback(func(view, path, column, data uintptr) uintptr {
		indices := gtkTreePathGetIndices(path)
		if indices != nil {
			emit(core.BackendEvent{Kind: core.EventItemActivated, H: core.Handle(data), Int: int(*indices)})
		}
		return 0
	})
	d.cbTableHeader = purego.NewCallback(func(column, data uintptr) uintptr {
		h := core.Handle(data)
		if d.win == nil {
			return 0
		}
		if n := d.win.nodes[h]; n != nil {
			for i, native := range n.tableColumns {
				if native == column {
					emit(core.BackendEvent{Kind: core.EventHeaderClicked, H: h, Int: i})
					break
				}
			}
		}
		return 0
	})
	return nil
}

// TableViewHandle returns the native GtkTreeView for GTK backend integration tests.
func TableViewHandle() uintptr {
	if current == nil || current.win == nil {
		return 0
	}
	for _, n := range current.win.nodes {
		if n.kind == core.KindTable {
			return n.inner
		}
	}
	return 0
}

func (w *window) SetTableColumns(h core.Handle, cols []core.TableColumn) {
	n := w.nodes[h]
	if n == nil || n.kind != core.KindTable {
		return
	}
	for len(n.tableColumns) > 0 {
		gtkTreeViewRemoveColumn(n.inner, n.tableColumns[0])
		n.tableColumns = n.tableColumns[1:]
	}
	if n.tableModel != 0 {
		gtkTreeViewSetModel(n.inner, 0)
		gObjectUnref(n.tableModel)
		n.tableModel = 0
	}
	if len(cols) == 0 {
		return
	}
	stringType := gTypeFromName("gchararray")
	if stringType == 0 {
		return
	}
	types := make([]uintptr, len(cols))
	for i := range types {
		types[i] = stringType
	}
	n.tableModel = gtkListStoreNewv(int32(len(types)), &types[0])
	if n.tableModel == 0 {
		return
	}
	gtkTreeViewSetModel(n.inner, n.tableModel)
	n.tableColumns = make([]uintptr, len(cols))
	for i, col := range cols {
		native := gtkTreeViewColumnNew()
		if native == 0 {
			continue
		}
		renderer := gtkCellRendererTextNew()
		gtkCellRendererSetAlignment(renderer, alignment(col.Align), 0.5)
		gtkTreeViewColumnPackStart(native, renderer, 1)
		gtkTreeViewColumnAddAttribute(native, renderer, "text", int32(i))
		gtkTreeViewColumnSetTitle(native, col.Title)
		gtkTreeViewColumnSetAlignment(native, alignment(col.Align))
		gtkTreeViewColumnSetClickable(native, 1)
		gtkTreeViewAppendColumn(n.inner, native)
		gSignalConnect(native, "clicked", w.drv.cbTableHeader, uintptr(h), 0, 0)
		n.tableColumns[i] = native
	}
	gtkTreeViewSetModel(n.inner, n.tableModel)
	gtkWidgetShow(n.inner)
}

func alignment(align core.Align) float32 {
	if align == core.AlignRight {
		return 1
	}
	return 0
}

func (w *window) SetTableRows(h core.Handle, rows [][]string) {
	n := w.nodes[h]
	if n == nil || n.kind != core.KindTable || n.tableModel == 0 {
		return
	}
	gtkListStoreClear(n.tableModel)
	n.tableRows = len(rows)
	for _, row := range rows {
		var iter gtkTreeIter
		gtkListStoreAppend(n.tableModel, &iter)
		for col := range n.tableColumns {
			text := ""
			if col < len(row) {
				text = row[col]
			}
			var value gtkGValue
			gValueInit(&value, gTypeFromName("gchararray"))
			gValueSetString(&value, text)
			gtkListStoreSetValue(n.tableModel, &iter, int32(col), &value)
			gValueUnset(&value)
		}
	}
}

func (w *window) SetSortIndicator(h core.Handle, col int, descending bool) {
	n := w.nodes[h]
	if n == nil || n.kind != core.KindTable {
		return
	}
	order := int32(0) // GTK_SORT_ASCENDING
	if descending {
		order = 1 // GTK_SORT_DESCENDING
	}
	for i, native := range n.tableColumns {
		gtkTreeViewColumnSetSortInd(native, boolInt(i == col))
		if i == col {
			gtkTreeViewColumnSetSortOrder(native, order)
		}
	}
}

func boolInt(v bool) int32 {
	if v {
		return 1
	}
	return 0
}

func (n *node) tableIntrinsic() (core.Size, core.Size) {
	var mn, nat requisition
	gtkPreferred(n.inner, unsafe.Pointer(&mn), unsafe.Pointer(&nat))
	height := float64(nat.Height)
	if n.tableRows > listRows {
		height = height / float64(n.tableRows) * listRows
	}
	return core.Size{W: float64(mn.Width), H: float64(mn.Height)},
		core.Size{W: float64(nat.Width) + scrollbarW, H: height}
}
