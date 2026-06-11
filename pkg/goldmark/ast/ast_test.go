package ast

import (
	"reflect"
	"testing"
)

func TestWalk(t *testing.T) {
	tests := []struct {
		name   string
		node   Node
		want   []NodeKind
		action map[NodeKind]WalkStatus
	}{
		{
			"visits all in depth first order",
			node(NewDocument(), node(NewHeading(1), NewText()), NewLink()),
			[]NodeKind{KindDocument, KindHeading, KindText, KindLink},
			map[NodeKind]WalkStatus{},
		},
		{
			"stops after heading",
			node(NewDocument(), node(NewHeading(1), NewText()), NewLink()),
			[]NodeKind{KindDocument, KindHeading},
			map[NodeKind]WalkStatus{KindHeading: WalkStop},
		},
		{
			"skip children",
			node(NewDocument(), node(NewHeading(1), NewText()), NewLink()),
			[]NodeKind{KindDocument, KindHeading, KindLink},
			map[NodeKind]WalkStatus{KindHeading: WalkSkipChildren},
		},
	}
	for _, tt := range tests {
		var kinds []NodeKind
		collectKinds := func(n Node, entering bool) (WalkStatus, error) {
			if entering {
				kinds = append(kinds, n.Kind())
			}
			if status, ok := tt.action[n.Kind()]; ok {
				return status, nil
			}
			return WalkContinue, nil
		}
		t.Run(tt.name, func(t *testing.T) {
			if err := Walk(tt.node, collectKinds); err != nil {
				t.Errorf("Walk() error = %v", err)
			} else if !reflect.DeepEqual(kinds, tt.want) {
				t.Errorf("Walk() expected = %v, got = %v", tt.want, kinds)
			}
		})
	}
}

func node(n Node, children ...Node) Node {
	for _, c := range children {
		n.AppendChild(n, c)
	}
	return n
}

func TestNodeKindCount(t *testing.T) {
	count := NodeKindCount()
	if count != int(kindMax)+1 {
		t.Errorf("NodeKindCount() = %d, want kindMax+1 = %d", count, int(kindMax)+1)
	}
	// Every registered kind must be a valid index into a table of size
	// NodeKindCount, including the highest one.
	if int(KindDocument) >= count || int(kindMax) >= count {
		t.Errorf("registered kinds must index a NodeKindCount-sized table")
	}
	k := NewNodeKind("nodeKindCountProbe")
	if got := NodeKindCount(); got != count+1 {
		t.Errorf("NodeKindCount() after NewNodeKind = %d, want %d", got, count+1)
	}
	if int(k) != NodeKindCount()-1 {
		t.Errorf("new kind %d must be the last valid index %d", int(k), NodeKindCount()-1)
	}
}
