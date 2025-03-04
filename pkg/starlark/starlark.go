package starlark

import (
	libstarlark "go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

type Bool libstarlark.Bool

const (
	False Bool = false
	True  Bool = true
)

func (b Bool) String() string {
	if b {
		return "true"
	} else {
		return "false"
	}
}

func (b Bool) Hash() (uint32, error) { return uint32(b2i(bool(b))), nil }
func (x Bool) CompareSameType(op syntax.Token, y_ libstarlark.Value, depth int) (bool, error) {
	y := y_.(Bool)
	return threeway(op, b2i(bool(x))-b2i(bool(y))), nil
}
func (b Bool) Type() string            { return "bool" }
func (b Bool) Freeze()                 {} // immutable
func (b Bool) Truth() libstarlark.Bool { return libstarlark.Bool(b) }

func b2i(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

func threeway(op syntax.Token, cmp int) bool {
	switch op {
	case syntax.EQL:
		return cmp == 0
	case syntax.NEQ:
		return cmp != 0
	case syntax.LE:
		return cmp <= 0
	case syntax.LT:
		return cmp < 0
	case syntax.GE:
		return cmp >= 0
	case syntax.GT:
		return cmp > 0
	}
	panic(op)
}
