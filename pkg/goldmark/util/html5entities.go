package util

import (
	"sync"
)

// The html5entities.gen.go data table is generated upstream by
// github.com/yuin/goldmark/_tools (not vendored in this fork).
// To regenerate, sync the upstream _tools directory locally, run
// `go generate` against it, and copy the result back.

var _html5entitiesOnce sync.Once
var _html5entitiesMap map[string]*HTML5Entity

func buildHTML5Entities() {
	_html5entitiesOnce.Do(func() {
		entities := make([]HTML5Entity, _html5entitiesLength)
		_html5entitiesMap = make(map[string]*HTML5Entity, _html5entitiesLength)

		cName := 0
		cCharacters := 0
		for i := range _html5entitiesLength {
			tName := cName + int(_html5entitiesNameIndex[i])
			tCharacters := cCharacters + int(_html5entitiesCharactersIndex[i])

			name := _html5entitiesName[cName:tName]
			e := &entities[i]
			e.Name = name
			e.Characters = _html5entitiesCharacters[cCharacters:tCharacters]
			_html5entitiesMap[name] = e

			cName = tName
			cCharacters = tCharacters
		}
	})
}

// HTML5Entity struct represents HTML5 entities.
type HTML5Entity struct {
	Name       string
	Characters []byte
}

// LookUpHTML5EntityByName returns (an HTML5Entity, true) if an entity named
// given name is found, otherwise (nil, false).
func LookUpHTML5EntityByName(name string) (*HTML5Entity, bool) {
	buildHTML5Entities()
	v, ok := _html5entitiesMap[name]
	return v, ok
}
