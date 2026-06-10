// Package all is a blank-import barrel that registers every
// production rule with the rule registry. Importing this package is
// equivalent to the side-effect imports in cmd/mdsmith/main.go;
// tests (notably internal/lsp/bench_test.go and any test that calls
// rule.All() expecting the production set) should pull this in
// instead of repeating the import list.
//
// The package itself exports nothing: callers blank-import it.
package all

// Each blank import below pulls in a rule package solely for its
// init() side effect — registering the rule with the global
// rule.Registry so rule.All() returns the production set. revive
// (with blank-imports enabled outside main/test packages) requires
// every blank import to carry a justifying comment, but adding one
// to every line would add no information beyond what this header
// already says. Keep the comments terse.
import (
	_ "github.com/jeduden/mdsmith/internal/rules/ambiguousemphasis"           // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/atxheadingwhitespace"        // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundfencedcode"   // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundheadings"     // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundlists"        // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/blockquotewhitespace"        // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/build"                       // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/callouttype"                 // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/catalog"                     // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/codeblockstyle"              // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/commandsshowoutput"          // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/concisenessscoring"          // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/crossfilereferenceintegrity" // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/descriptivelinktext"         // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/directorystructure"          // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/duplicatedcontent"           // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/emphasisstyle"               // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/emptysectionbody"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodelanguage"          // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/fencedcodestyle"             // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/firstlineheading"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/forbiddenparagraphstarts"    // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/forbiddentext"               // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/githooksync"                 // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/headingincrement"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/headingstyle"                // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/horizontalrulestyle"         // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/include"                     // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"                  // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/linkstyle"                   // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/linkvalidity"                // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/listindent"                  // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/listmarkerspace"             // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/listmarkerstyle"             // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/markdownflavor"              // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/maxfilelength"               // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/maxsectionlength"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/nobareurls"                  // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/noduplicateheadings"         // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"         // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/noemptyalttext"              // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/nohardtabs"                  // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/noinlinehtml"                // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/nomultipleblanks"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/noreferencestyle"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/nospaceincodespans"          // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/nospaceinlinktext"           // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingpunctuation"       // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/noundefinedreferencelabels"  // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/nounusedlinkdefinitions"     // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/orderedlistnumbering"        // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphreadability"        // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/paragraphstructure"          // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/propernames"                 // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/recipesafety"                // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/requiredmentions"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/requiredstructure"           // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/requiredtextpatterns"        // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/singleh1"                    // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/singletrailingnewline"       // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/tableformat"                 // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/tablereadability"            // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/toc"                         // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/tocdirective"                // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/tokenbudget"                 // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/unclosedcodeblock"           // registers rule
	_ "github.com/jeduden/mdsmith/internal/rules/uniquefrontmatter"           // registers rule
)
