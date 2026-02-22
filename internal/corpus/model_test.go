package corpus

import "testing"

func TestCategoryConstants(t *testing.T) {
	t.Parallel()

	if CategoryReference == "" || CategoryOther == "" {
		t.Fatal("category constants must be non-empty")
	}
	if CategoryReference == CategoryOther {
		t.Fatal("category constants must differ")
	}
}
