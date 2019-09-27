// eval_functions.go contains our in-built functions.

package evalfilter

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/skx/evalfilter/object"
)

// fnLen is the implementation of our `len` function.
func fnLen(args []object.Object) object.Object {
	sum := 0

	for _, e := range args {
		switch e := e.(type) {
		case *object.String:
			sum += utf8.RuneCountInString(e.Value)
		}
	}
	return &object.Integer{Value: int64(sum)}
}

// fnMatch is the implementation of our regex `match` function.
func fnMatch(args []object.Object) object.Object {

	// We expect two arguments
	if len(args) != 2 {
		return &object.Boolean{Value: false}
	}

	str := args[0].Inspect()
	reg := args[1].Inspect()

	// Compile the regular expression
	r, _ := regexp.Compile(reg)

	// Split the input by newline.
	for _, s := range strings.Split(str, "\n") {

		// Strip leading-trailing whitespace
		s = strings.TrimSpace(s)

		if r.MatchString(s) {
			return &object.Boolean{Value: true}
		}
	}
	return &object.Boolean{Value: false}

}

// fnTrim is the implementation of our `trim` function.
func fnTrim(args []object.Object) object.Object {
	str := ""
	for _, e := range args {
		str += fmt.Sprintf("%v", (e.Inspect()))
	}
	return &object.String{Value: strings.TrimSpace(str)}
}

// fnPrint is the implementation of our `print` function.
func fnPrint(args []object.Object) object.Object {
	for _, e := range args {
		fmt.Printf("%s", e.Inspect())
	}
	return &object.Integer{Value: 0}
}
