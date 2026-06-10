package cuelite_test

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Validate marshalled front matter against a schema: compile both
// sides, unify them, and validate the merged value.
func Example() {
	schema, err := cuelite.Compile("title: string\nstatus: \"draft\" | \"final\"")
	if err != nil {
		panic(err)
	}
	doc, err := cuelite.CompileJSON([]byte(`{"title": "Roadmap", "status": "final"}`))
	if err != nil {
		panic(err)
	}
	fmt.Println(schema.Unify(doc).Validate())
	// Output: <nil>
}

// A rejection decomposes into one PathError per failing field. The
// example prints only the paths: the message text belongs to the
// underlying engine and is not part of the package's contract.
func ExampleErrors() {
	schema, _ := cuelite.Compile("title: string\ncount: int")
	doc, _ := cuelite.CompileJSON([]byte(`{"title": 7, "count": "many"}`))
	err := schema.Unify(doc).Validate()
	for _, pathErr := range cuelite.Errors(err) {
		fmt.Println(strings.Join(pathErr.Path(), "."))
	}
	// Output:
	// title
	// count
}

// The input must be strict JSON: a duplicate object key is rejected
// before the CUE lift, naming the offending key.
func ExampleCompileJSON() {
	_, err := cuelite.CompileJSON([]byte(`{"status": "draft", "status": "final"}`))
	fmt.Println(err)
	// Output: duplicate JSON key "status"
}
