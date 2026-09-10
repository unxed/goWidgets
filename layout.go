package goWidgets

import (
	"errors"

	"github.com/unxed/goWidgets/core"
	kiwi "github.com/unxed/kiwi-go"
)

// Layout is declared, not computed: a dialog is a set of linear constraints
// between the edges of its widgets, and Cassowary finds the rectangles (§5).
//
//	win.Constrain(
//		ok.Right().Eq(win.Right().Minus(8)),
//		ok.Bottom().Eq(win.Bottom().Minus(8)),
//		cancel.Right().Eq(ok.Left().Minus(8)),
//		cancel.Top().Eq(ok.Top()),
//	)
//
// Every widget and the window's content area is a Box with eight anchors. A
// widget that is the subject of no constraint — the owner of the left-hand
// anchor of none — is stacked by the flow: full width under the previous such
// widget, 8 DIP apart. So a window with no constraints still shows everything,
// and the first constraint on a widget takes it out of the stack. Edges a
// constraint does not fix keep their natural (measured) size.
//
// Strengths: a widget's minimum size is required; user constraints are strong
// by default; natural sizes are weak. When strong rules disagree, the solver
// satisfies as many as it can — there is no error, only a layout you did not
// mean, which is where Medium() and Weak() come in. A required rule that
// contradicts another required one is rejected outright (core.ErrUnsatisfiable).
//
// Hiding a constrained widget collapses it to zero size but keeps it in its
// chains, so the widgets below move up by its height — the gap constraint
// itself stays. Hiding a flow widget removes it from the flow entirely.

// Box is anything with layout anchors: every widget, and the window.
type Box interface {
	Left() Anchor
	Top() Anchor
	Right() Anchor
	Bottom() Anchor
	Width() Anchor
	Height() Anchor
	CenterX() Anchor
	CenterY() Anchor
}

// Anchor is one edge, dimension or centre line of a Box, or an expression
// built from one: `a.Right().Plus(8)`, `win.Width().Times(0.5)`.
type Anchor struct {
	app   *App
	owner *core.Node // nil for the window
	expr  *kiwi.Expression
}

// Plus offsets the anchor by v DIP.
func (a Anchor) Plus(v float64) Anchor { return Anchor{a.app, a.owner, a.expr.Plus(v)} }

// Minus offsets the anchor by -v DIP.
func (a Anchor) Minus(v float64) Anchor { return Anchor{a.app, a.owner, a.expr.Minus(v)} }

// Times scales the anchor: `win.Width().Times(0.5)` is half the window.
func (a Anchor) Times(k float64) Anchor { return Anchor{a.app, a.owner, a.expr.Multiply(k)} }

// Eq relates two anchors: a == b.
func (a Anchor) Eq(b Anchor) *Constraint { return a.rel(kiwi.OpEq, b.expr) }

// Ge relates two anchors: a >= b.
func (a Anchor) Ge(b Anchor) *Constraint { return a.rel(kiwi.OpGe, b.expr) }

// Le relates two anchors: a <= b.
func (a Anchor) Le(b Anchor) *Constraint { return a.rel(kiwi.OpLe, b.expr) }

// Is pins the anchor to a value: `btn.Width().Is(120)`.
func (a Anchor) Is(v float64) *Constraint { return a.rel(kiwi.OpEq, kiwi.NewExpression(v)) }

// AtLeast bounds the anchor from below: a >= v.
func (a Anchor) AtLeast(v float64) *Constraint { return a.rel(kiwi.OpGe, kiwi.NewExpression(v)) }

// AtMost bounds the anchor from above: a <= v.
func (a Anchor) AtMost(v float64) *Constraint { return a.rel(kiwi.OpLe, kiwi.NewExpression(v)) }

func (a Anchor) rel(op kiwi.Operator, rhs *kiwi.Expression) *Constraint {
	return &Constraint{
		app:      a.app,
		subject:  a.owner,
		lhs:      a.expr,
		rhs:      rhs,
		op:       op,
		strength: core.StrengthStrong,
	}
}

// Constraint is one linear rule. It does nothing until passed to
// Window.Constrain, so strength can be set first:
//
//	win.Constrain(lbl.Width().Is(200).Weak())
//
// Its subject — the widget owning the left-hand anchor — is the widget it
// takes out of the flow.
type Constraint struct {
	app      *App
	subject  *core.Node
	lhs, rhs *kiwi.Expression
	op       kiwi.Operator
	strength float64

	cn *kiwi.Constraint // non-nil while held by the solver
}

// Required makes the rule inviolable; two required rules that disagree are an
// error rather than a compromise.
func (c *Constraint) Required() *Constraint { return c.setStrength(core.StrengthRequired) }

// Strong is the default: satisfied unless required rules forbid it.
func (c *Constraint) Strong() *Constraint { return c.setStrength(core.StrengthStrong) }

// Medium yields to strong rules — the strength of the flow itself.
func (c *Constraint) Medium() *Constraint { return c.setStrength(core.StrengthMedium) }

// Weak is a preference, the strength of a widget's natural size.
func (c *Constraint) Weak() *Constraint { return c.setStrength(core.StrengthWeak) }

func (c *Constraint) setStrength(s float64) *Constraint {
	c.strength = s
	if c.cn != nil {
		// Changing the strength of a held constraint re-adds it: kiwi has no
		// in-place strength edit.
		c.app.eng.RemoveConstraint(c.subject, c.cn)
		c.cn = nil
		_ = c.add()
	}
	return c
}

func (c *Constraint) add() error {
	if c.cn != nil {
		return nil
	}
	cn := kiwi.NewConstraint(c.lhs, c.op, c.rhs, c.strength)
	if err := c.app.eng.AddConstraint(c.subject, cn); err != nil {
		return err
	}
	c.cn = cn
	return nil
}

// Active reports whether the solver currently holds the rule.
func (c *Constraint) Active() bool { return c.cn != nil }

// Constrain hands rules to the solver. Every rule is tried; the ones the
// solver rejects as unsatisfiable (a required rule against a required rule)
// are dropped, recorded in Diagnostics().LayoutConflicts and returned as one
// joined error wrapping core.ErrUnsatisfiable — the rest stay in force.
func (w *Window) Constrain(cs ...*Constraint) error {
	var errs []error
	for _, c := range cs {
		if c == nil {
			continue
		}
		if err := c.add(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Unconstrain takes rules back out. A widget whose last rule is removed
// returns to the flow.
func (w *Window) Unconstrain(cs ...*Constraint) {
	for _, c := range cs {
		if c == nil || c.cn == nil {
			continue
		}
		w.app.eng.RemoveConstraint(c.subject, c.cn)
		c.cn = nil
	}
}

// anchors builds the eight anchors of a node; nil owner means the window.
type anchors struct {
	app   *App
	owner *core.Node
	vars  *core.Vars
}

func (x anchors) at(e *kiwi.Expression) Anchor { return Anchor{x.app, x.owner, e} }

func (x anchors) Left() Anchor    { return x.at(kiwi.NewExpression(x.vars.Left)) }
func (x anchors) Top() Anchor     { return x.at(kiwi.NewExpression(x.vars.Top)) }
func (x anchors) Width() Anchor   { return x.at(kiwi.NewExpression(x.vars.Width)) }
func (x anchors) Height() Anchor  { return x.at(kiwi.NewExpression(x.vars.Height)) }
func (x anchors) Right() Anchor   { return x.at(x.vars.Right()) }
func (x anchors) Bottom() Anchor  { return x.at(x.vars.Bottom()) }
func (x anchors) CenterX() Anchor { return x.at(x.vars.CenterX()) }
func (x anchors) CenterY() Anchor { return x.at(x.vars.CenterY()) }

func (w *widget) anchors() anchors { return anchors{w.app, w.node, w.node.Vars} }

// HugWidth makes the widget hold its natural width as a strong rule instead
// of a weak preference: in a row where something has to stretch, the hugged
// widget is not it. HugHeight is the same for height.
//
//	win.Constrain(goWidgets.Row(8, entry, add)...)
//	add.HugWidth() // the field grows, the button stays a button
func (w *widget) HugWidth() { w.app.eng.SetHug(w.node, true, w.node.HugH) }

// HugHeight makes the widget hold its natural height as a strong rule.
func (w *widget) HugHeight() { w.app.eng.SetHug(w.node, w.node.HugW, true) }

// Left is the widget's left edge.
func (w *widget) Left() Anchor { return w.anchors().Left() }

// Top is the widget's top edge.
func (w *widget) Top() Anchor { return w.anchors().Top() }

// Right is the widget's right edge (left + width).
func (w *widget) Right() Anchor { return w.anchors().Right() }

// Bottom is the widget's bottom edge (top + height).
func (w *widget) Bottom() Anchor { return w.anchors().Bottom() }

// Width is the widget's width.
func (w *widget) Width() Anchor { return w.anchors().Width() }

// Height is the widget's height.
func (w *widget) Height() Anchor { return w.anchors().Height() }

// CenterX is the widget's horizontal centre line.
func (w *widget) CenterX() Anchor { return w.anchors().CenterX() }

// CenterY is the widget's vertical centre line.
func (w *widget) CenterY() Anchor { return w.anchors().CenterY() }

func (w *Window) anchors() anchors {
	root := w.app.eng.RootNode()
	return anchors{w.app, nil, root.Vars}
}

// Left is the content area's left edge (always 0).
func (w *Window) Left() Anchor { return w.anchors().Left() }

// Top is the content area's top edge (always 0).
func (w *Window) Top() Anchor { return w.anchors().Top() }

// Right is the content area's right edge: its current width.
func (w *Window) Right() Anchor { return w.anchors().Right() }

// Bottom is the content area's bottom edge: its current height.
func (w *Window) Bottom() Anchor { return w.anchors().Bottom() }

// Width is the content area's width. Nothing a constraint says can change
// it; it follows the native window.
func (w *Window) Width() Anchor { return w.anchors().Width() }

// Height is the content area's height, likewise owned by the native window.
func (w *Window) Height() Anchor { return w.anchors().Height() }

// CenterX is the content area's horizontal centre line.
func (w *Window) CenterX() Anchor { return w.anchors().CenterX() }

// CenterY is the content area's vertical centre line.
func (w *Window) CenterY() Anchor { return w.anchors().CenterY() }

// Guide is a rectangle that takes part in layout and nothing else: no native
// control, not drawn, not in the flow. It is how dialogs nest — a button bar,
// a two-column area, an inset panel — without a container widget or a second
// layout engine. Constrain the guide to the window and the widgets to the
// guide:
//
//	bar := win.NewGuide()
//	win.Constrain(
//		bar.Left().Eq(win.Left().Plus(8)),
//		bar.Right().Eq(win.Right().Minus(8)),
//		bar.Bottom().Eq(win.Bottom().Minus(8)),
//		bar.Height().Is(36),
//	)
//	win.Constrain(goWidgets.Fill(ok, bar, 0)...)
//
// A guide has no size of its own: what its edges are not pinned to, the
// widgets inside it (or nothing) decide.
type Guide struct{ anchors }

// NewGuide creates a layout guide in the window.
func (w *Window) NewGuide() *Guide {
	return &Guide{anchors{w.app, nil, w.app.eng.NewGuide()}}
}

// ---------------------------------------------------------------- helpers
//
// These build ordinary constraints — nothing here the caller could not write
// by hand. The first item of a chain is never a subject, so it stays wherever
// it already is (in the flow, or pinned by the caller); the rest follow it.

// Column stacks items top to bottom, gap DIP apart, with left and right edges
// aligned to the first item.
func Column(gap float64, items ...Box) []*Constraint {
	var cs []*Constraint
	for i := 1; i < len(items); i++ {
		cs = append(cs,
			items[i].Top().Eq(items[i-1].Bottom().Plus(gap)),
			items[i].Left().Eq(items[0].Left()),
			items[i].Right().Eq(items[0].Right()),
		)
	}
	return cs
}

// Row lines items up left to right, gap DIP apart, with top and bottom edges
// aligned to the first item.
func Row(gap float64, items ...Box) []*Constraint {
	var cs []*Constraint
	for i := 1; i < len(items); i++ {
		cs = append(cs,
			items[i].Left().Eq(items[i-1].Right().Plus(gap)),
			items[i].Top().Eq(items[0].Top()),
			items[i].Bottom().Eq(items[0].Bottom()),
		)
	}
	return cs
}

// EqualWidths gives every item the width of the first.
func EqualWidths(items ...Box) []*Constraint {
	var cs []*Constraint
	for i := 1; i < len(items); i++ {
		cs = append(cs, items[i].Width().Eq(items[0].Width()))
	}
	return cs
}

// EqualHeights gives every item the height of the first.
func EqualHeights(items ...Box) []*Constraint {
	var cs []*Constraint
	for i := 1; i < len(items); i++ {
		cs = append(cs, items[i].Height().Eq(items[0].Height()))
	}
	return cs
}

// Fill pins an item to all four edges of a container, inset DIP inside it.
func Fill(item, container Box, inset float64) []*Constraint {
	return []*Constraint{
		item.Left().Eq(container.Left().Plus(inset)),
		item.Top().Eq(container.Top().Plus(inset)),
		item.Right().Eq(container.Right().Minus(inset)),
		item.Bottom().Eq(container.Bottom().Minus(inset)),
	}
}
