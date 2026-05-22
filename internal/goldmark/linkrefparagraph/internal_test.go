package linkrefparagraph

import "testing"

func TestSameSlice(t *testing.T) {
	a := []byte("hello")
	b := a[:]
	if !sameSlice(a, b) {
		t.Error("aliased slices should be sameSlice")
	}
	c := append([]byte(nil), a...)
	if sameSlice(a, c) {
		t.Error("distinct backing arrays with equal content must differ")
	}
	if sameSlice(a, []byte("hi")) {
		t.Error("differing lengths must short-circuit to false")
	}
	if !sameSlice(nil, nil) {
		t.Error("two empty slices should be considered same")
	}
	if !sameSlice([]byte{}, nil) {
		t.Error("empty and nil slice should be considered same")
	}
}
