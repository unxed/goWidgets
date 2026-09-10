package core

import (
	"errors"
	"fmt"
	"math"

	kiwi "github.com/unxed/kiwi-go"
)

// Layout is Cassowary (§5): every widget is four solver variables, every rule
// about where it goes is a linear constraint, and the window size enters as
// edit variables so a resize is an incremental re-solve, not a rebuild.
//
// Strength hierarchy of §5.3, from strongest to weakest:
//
//	required        widget minimum sizes, and whatever the user marks required
//	window          the window's own size — nothing a user writes may resize it
//	strong          user constraints (the default)
//	medium          the flow (see below), a chosen PrefHeight, and the collapse
//	                of a hidden widget to zero along every axis no strong rule
//	                holds — so a hidden row in a Column loses its height but
//	                keeps the width the column aligns it to
//	weak            natural sizes: a widget wants to be as big as its content
//
// The flow: a widget that is the subject of no constraint at all is stacked
// under the previous such widget, full width, FlowPad from the edges — so an
// unconstrained window still shows everything, and one constraint on a widget
// is enough to take it out of the stack. The flow is itself a set of
// medium-strength constraints in the same solver: there is one layout engine,
// not a stack plus a solver.
var (
	StrengthRequired = kiwi.StrengthRequired
	StrengthStrong   = kiwi.StrengthStrong
	StrengthMedium   = kiwi.StrengthMedium
	StrengthWeak     = kiwi.StrengthWeak

	strengthWindow   = kiwi.CreateStrength(1000, 0, 0)
	strengthCollapse = kiwi.StrengthMedium
	// strengthStretchy is below weak: a scrolling widget (list, text view)
	// holds its natural size less firmly than a button or a label does, so
	// when a column has slack the list takes it, not the button. The
	// content-hugging default of every desktop toolkit, in one number.
	strengthStretchy = kiwi.CreateStrength(0, 0, 0.5)
)

// FlowPad is the padding the flow keeps around and between stacked widgets.
const FlowPad = 8.0

// Vars are a node's solver variables. Right, Bottom and the centres are
// expressions over these four rather than variables of their own, so nothing
// can ever disagree about where the right edge is.
type Vars struct{ Left, Top, Width, Height *kiwi.Variable }

// Right is Left + Width.
func (v *Vars) Right() *kiwi.Expression { return v.Left.Plus(v.Width) }

// Bottom is Top + Height.
func (v *Vars) Bottom() *kiwi.Expression { return v.Top.Plus(v.Height) }

// CenterX is Left + Width/2.
func (v *Vars) CenterX() *kiwi.Expression { return v.Left.Plus(v.Width.Multiply(0.5)) }

// CenterY is Top + Height/2.
func (v *Vars) CenterY() *kiwi.Expression { return v.Top.Plus(v.Height.Multiply(0.5)) }

func newVars(h Handle) *Vars {
	p := fmt.Sprintf("n%d.", h)
	return &Vars{
		Left:   kiwi.NewVariable(p + "left"),
		Top:    kiwi.NewVariable(p + "top"),
		Width:  kiwi.NewVariable(p + "width"),
		Height: kiwi.NewVariable(p + "height"),
	}
}

// ErrUnsatisfiable reports a constraint the solver could not accept because
// it contradicts the required ones already there. Per §5.3 the constraint is
// dropped and the conflict is recorded in DriverInfo.LayoutConflicts; a debug
// build panics instead, and silently hanging is never an option.
var ErrUnsatisfiable = errors.New("layout: unsatisfiable constraint")

// layoutEngine owns the solver and the two sets of constraints core generates
// on its own: per-node size constraints and the flow.
type layoutEngine struct {
	solver *kiwi.Solver

	flow    []*kiwi.Constraint
	flowKey []Handle // participants the current flow was built for
	sized   Size     // the window size last suggested to the solver
	seeded  bool
}

func newLayoutEngine() *layoutEngine {
	return &layoutEngine{solver: kiwi.NewSolver()}
}

// sizeKey is what a node's size constraints were built from; when the next
// measurement produces the same key nothing is touched.
type sizeKey struct {
	min, nat   Size
	prefHeight float64
	visible    bool
	hugW, hugH bool
	stretchy   bool
}

// initRoot seeds the content area: its origin is pinned, its size is edited.
func (a *App) initRoot(root *Node, spec Size) {
	s := a.lay.solver
	v := root.Vars
	must(s.AddConstraint(kiwi.NewConstraint(v.Left, kiwi.OpEq, 0, StrengthRequired)))
	must(s.AddConstraint(kiwi.NewConstraint(v.Top, kiwi.OpEq, 0, StrengthRequired)))
	must(s.AddEditVariable(v.Width, strengthWindow))
	must(s.AddEditVariable(v.Height, strengthWindow))
	a.lay.suggestRoot(root, spec)
}

// must is for constraints that cannot fail: a fresh variable pinned to a
// constant. An error here is a bug in core, not a user mistake.
func must(err error) {
	if err != nil {
		panic("layout: " + err.Error())
	}
}

func (l *layoutEngine) suggestRoot(root *Node, size Size) {
	if l.seeded && l.sized == size {
		return
	}
	l.seeded, l.sized = true, size
	must(l.solver.SuggestValue(root.Vars.Width, size.W))
	must(l.solver.SuggestValue(root.Vars.Height, size.H))
}

// AddConstraint hands a user constraint to the solver. subject is the widget
// the constraint is about — the owner of its left-hand anchor — and having
// one takes that widget out of the flow. A nil subject (a constraint on the
// window itself) affects nobody's flow membership.
func (a *App) AddConstraint(subject *Node, c *kiwi.Constraint) error {
	if err := a.lay.solver.AddConstraint(c); err != nil {
		return a.conflict(c, err)
	}
	if subject != nil {
		subject.constraints++
	}
	a.dirtyLayout = true
	a.driver.Wake()
	return nil
}

// RemoveConstraint takes a user constraint back out. Removing one the solver
// does not hold is not an error: a dropped conflicting constraint and an
// already-removed one both end up here.
func (a *App) RemoveConstraint(subject *Node, c *kiwi.Constraint) {
	if a.lay.solver.RemoveConstraint(c) != nil {
		return
	}
	if subject != nil && subject.constraints > 0 {
		subject.constraints--
	}
	a.dirtyLayout = true
	a.driver.Wake()
}

// conflict implements §5.3 for one rejected constraint.
func (a *App) conflict(c *kiwi.Constraint, err error) error {
	msg := fmt.Sprintf("%s: %v", c, err)
	if DebugBuild {
		panic("layout: unsatisfiable constraint (§5.3): " + msg)
	}
	a.info.LayoutConflicts = append(a.info.LayoutConflicts, msg)
	return fmt.Errorf("%w: %s", ErrUnsatisfiable, msg)
}

// RootNode is the window's content area as a layout parent.
func (a *App) RootNode() *Node { return a.nodes[a.root] }

// solveLayout is steps 5–7 of §6: measure what is dirty, refresh the
// constraints core derives from measurements and flow membership, solve, and
// diff the rectangles.
func (a *App) solveLayout() []BoundsChange {
	root := a.nodes[a.root]
	if root == nil {
		return nil
	}
	l := a.lay
	l.suggestRoot(root, Size{W: root.Bounds.W, H: root.Bounds.H})

	// Measurement asks for the width a full-width row would get; a backend
	// that wraps text needs a width to wrap at.
	avail := Size{W: root.Bounds.W - 2*FlowPad, H: root.Bounds.H}
	if avail.W < 0 {
		avail.W = 0
	}
	for _, ch := range root.Children {
		n := a.nodes[ch]
		if n == nil {
			continue
		}
		if n.Visible && !n.measured {
			n.minSize, n.natSize = a.win.MeasureIntrinsic(n.H, avail)
			n.measured = true
		}
		a.syncSize(n)
	}
	a.syncFlow(root)

	l.solver.UpdateVariables()

	var changes []BoundsChange
	for _, ch := range root.Children {
		n := a.nodes[ch]
		if n == nil {
			continue
		}
		var r Rect
		if n.Visible {
			r = Rect{
				X: dip(n.Vars.Left.Value()), Y: dip(n.Vars.Top.Value()),
				W: dip(n.Vars.Width.Value()), H: dip(n.Vars.Height.Value()),
			}
		}
		if r != n.Bounds {
			n.Bounds = r
			changes = append(changes, BoundsChange{H: n.H, R: r, Visible: n.Visible})
		}
	}
	return changes
}

// dip rounds a solved value to a thousandth of a DIP: the simplex leaves
// float noise around exact answers, and that noise must not read as a change.
func dip(v float64) float64 {
	r := math.Round(v*1000) / 1000
	if r == 0 {
		return 0 // no negative zero in golden files
	}
	return r
}

// syncSize keeps a node's own size constraints (§5.1) in step with its last
// measurement: w ≥ min required, w = natural weak, a PrefHeight medium. A
// hidden node instead prefers zero size, so a chain through it closes up
// along the axis the chain runs in, while alignment rules (strong) still hold.
func (a *App) syncSize(n *Node) {
	key := sizeKey{min: n.minSize, nat: n.natSize, prefHeight: n.PrefHeight, visible: n.Visible, hugW: n.HugW, hugH: n.HugH, stretchy: n.Stretchy}
	if n.sizeCns != nil && key == n.sizeKey {
		return
	}
	s := a.lay.solver
	for _, c := range n.sizeCns {
		_ = s.RemoveConstraint(c)
	}
	n.sizeCns = n.sizeCns[:0]
	n.sizeKey = key
	v := n.Vars

	add := func(c *kiwi.Constraint) {
		if err := s.AddConstraint(c); err != nil {
			// A measurement that contradicts a required user constraint. The
			// user's rule wins and the conflict is reported, not hidden.
			_ = a.conflict(c, err)
			return
		}
		n.sizeCns = append(n.sizeCns, c)
	}
	if !n.Visible {
		add(kiwi.NewConstraint(v.Width, kiwi.OpEq, 0, strengthCollapse))
		add(kiwi.NewConstraint(v.Height, kiwi.OpEq, 0, strengthCollapse))
		return
	}
	add(kiwi.NewConstraint(v.Width, kiwi.OpGe, n.minSize.W, StrengthRequired))
	add(kiwi.NewConstraint(v.Height, kiwi.OpGe, n.minSize.H, StrengthRequired))
	add(kiwi.NewConstraint(v.Width, kiwi.OpEq, n.natSize.W, hugStrength(n.HugW, n.Stretchy)))
	if n.PrefHeight > 0 {
		add(kiwi.NewConstraint(v.Height, kiwi.OpEq, n.PrefHeight, StrengthMedium))
	} else {
		add(kiwi.NewConstraint(v.Height, kiwi.OpEq, n.natSize.H, hugStrength(n.HugH, n.Stretchy)))
	}
}

// syncFlow rebuilds the stack constraints when its membership changes:
// visible widgets that are the subject of no user constraint, in creation
// order. Membership is a short list, so comparing it is cheaper than tracking
// every event that could alter it.
func (a *App) syncFlow(root *Node) {
	var members []Handle
	for _, ch := range root.Children {
		if n := a.nodes[ch]; n != nil && n.Visible && n.constraints == 0 {
			members = append(members, ch)
		}
	}
	l := a.lay
	if l.flow != nil && equalHandles(members, l.flowKey) {
		return
	}
	for _, c := range l.flow {
		_ = l.solver.RemoveConstraint(c)
	}
	l.flow = l.flow[:0]
	l.flowKey = members

	rv := root.Vars
	var prev *Node
	for _, h := range members {
		n := a.nodes[h]
		v := n.Vars
		top := rv.Top.Plus(FlowPad)
		if prev != nil {
			top = prev.Vars.Bottom().Plus(FlowPad)
		}
		for _, c := range []*kiwi.Constraint{
			kiwi.NewConstraint(v.Left, kiwi.OpEq, rv.Left.Plus(FlowPad), StrengthMedium),
			kiwi.NewConstraint(v.Right(), kiwi.OpEq, rv.Right().Minus(FlowPad), StrengthMedium),
			kiwi.NewConstraint(v.Top, kiwi.OpEq, top, StrengthMedium),
		} {
			must(l.solver.AddConstraint(c)) // medium never conflicts with required
			l.flow = append(l.flow, c)
		}
		prev = n
	}
	if l.flow == nil {
		l.flow = []*kiwi.Constraint{} // non-nil: "built, and empty"
	}
}

// hugStrength is how firmly a natural size is held: a preference by
// default, less than that for a scrolling widget, a rule when the widget
// hugs its content.
func hugStrength(hug, stretchy bool) float64 {
	switch {
	case hug:
		return StrengthStrong
	case stretchy:
		return strengthStretchy
	}
	return StrengthWeak
}

func equalHandles(a, b []Handle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// NewGuide creates a rectangle that exists only in the solver: no backend
// widget, no flow membership, nothing drawn. It is the nesting primitive of
// a single-solver layout — a container is just a Box other boxes are
// constrained to, and since every coordinate is absolute already there is
// no tree to keep, no relative-to-parent conversion, no second engine. A
// guide only refuses to be negative in size.
func (a *App) NewGuide() *Vars {
	a.nextGuide++
	v := newVars(Handle(1<<62 + a.nextGuide)) // names only; never a backend handle
	s := a.lay.solver
	must(s.AddConstraint(kiwi.NewConstraint(v.Width, kiwi.OpGe, 0, StrengthRequired)))
	must(s.AddConstraint(kiwi.NewConstraint(v.Height, kiwi.OpGe, 0, StrengthRequired)))
	return v
}
