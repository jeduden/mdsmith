package punkt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetString_AddHas(t *testing.T) {
	ss := SetString{}
	assert.False(t, ss.Has("x"), "empty set must not contain anything")
	ss.Add("x")
	assert.True(t, ss.Has("x"), "Add must make Has return true")
	// Zero-value entries (`ss[k] = 0`) must not be reported as present —
	// that is the upstream contract callers rely on.
	ss["y"] = 0
	assert.False(t, ss.Has("y"),
		"a zero-valued entry must not count as set membership")
}

func TestNewStorage(t *testing.T) {
	s := NewStorage()
	require.NotNil(t, s)
	assert.NotNil(t, s.AbbrevTypes)
	assert.NotNil(t, s.Collocations)
	assert.NotNil(t, s.SentStarters)
	assert.NotNil(t, s.OrthoContext)
}

func TestLoadTraining_Happy(t *testing.T) {
	raw := []byte(`{
        "AbbrevTypes": {"dr": 1, "mr": 1},
        "Collocations": {"u.s,supreme": 1, "p,m": 1, "skip": 0},
        "SentStarters": {"however": 1},
        "OrthoContext": {"the": 64}
    }`)
	s, err := LoadTraining(raw)
	require.NoError(t, err)

	assert.True(t, s.AbbrevTypes.Has("dr"))
	assert.True(t, s.SentStarters.Has("however"))
	assert.Equal(t, 64, s.OrthoContext["the"])

	t.Run("collocations keyed by the upstream comma-joined string", func(t *testing.T) {
		// The runtime path in tokenAnnotation builds the same
		// `typ + "," + nextTyp` key and looks it up via
		// Collocations[string(buf)]. The Storage test pins the
		// data shape that path depends on.
		assert.True(t, s.Collocations.Has("u.s,supreme"),
			"trained pair must look up positive")
		assert.True(t, s.Collocations.Has("p,m"))
		assert.False(t, s.Collocations.Has("supreme,u.s"),
			"order matters — reversed key must miss")
		assert.False(t, s.Collocations.Has("missing,key"),
			"unknown pair must miss")
	})

	t.Run("zero-valued collocation reported as absent", func(t *testing.T) {
		// "skip": 0 in the JSON is present in the SetString map but
		// SetString.Has returns false, which is what the runtime
		// `s.Collocations[k] != 0` check relies on.
		assert.False(t, s.Collocations.Has("skip"),
			"zero-valued entries must not count as set members")
	})
}

func TestLoadTraining_MalformedJSON(t *testing.T) {
	_, err := LoadTraining([]byte("not json"))
	require.Error(t, err)
}

func TestStorage_IsAbbr(t *testing.T) {
	s := NewStorage()
	s.AbbrevTypes.Add("dr")
	s.AbbrevTypes.Add("mr")

	t.Run("known token matches", func(t *testing.T) {
		assert.True(t, s.IsAbbr("dr"))
		assert.True(t, s.IsAbbr("mr"))
	})
	t.Run("unknown token misses", func(t *testing.T) {
		assert.False(t, s.IsAbbr("doctor"))
	})
	t.Run("variadic — any known token wins", func(t *testing.T) {
		assert.True(t, s.IsAbbr("unknown", "mr", "other"),
			"a single match anywhere in the args must return true")
	})
	t.Run("no args — false", func(t *testing.T) {
		assert.False(t, s.IsAbbr(),
			"the empty variadic case must not claim a match")
	})
}

func TestStorage_addOrthoContext(t *testing.T) {
	s := NewStorage()
	s.addOrthoContext("foo", 1<<1)
	s.addOrthoContext("foo", 1<<2)
	// Bits compose by OR.
	assert.Equal(t, (1<<1)|(1<<2), s.OrthoContext["foo"])
}
