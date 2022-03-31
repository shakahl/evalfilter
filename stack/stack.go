// Package stack implements a stack which is used for our virtual machine.
package stack

import (
	"errors"
	"strings"

	"github.com/skx/evalfilter/v2/object"
)

// Stack implements a stack which can hold an arbitrary number
// of objects.  It is used by the virtual-machine to perform
// calculations, etc.
//
// The stack may grow to any size, as it is not capped.
type Stack struct {

	// entries hold our stack entries.
	//
	// We store them in a list which is less
	// efficient than explicitly setting up a
	// size - but the advantage is that we don't
	// need to worry about exhausting our stack
	// size at any point, except due to OOM errors!
	entries []object.Object
}

// New creates a new stack object.
func New() *Stack {
	return &Stack{}
}

// Clear removes all data from the stack
func (s *Stack) Clear() {
	s.entries = []object.Object{}
}

// Empty returns true if the stack is empty.
func (s *Stack) Empty() bool {
	return (len(s.entries) == 0)
}

// Export returns copy of the stack-contents, in string-form.
//
// This is used when tracing execution of programs.
func (s *Stack) Export() []string {
	var ret []string

	for _, ent := range s.entries {
		s := ent.Inspect()
		s = strings.ReplaceAll(s, "\n", "\\n")

		ret = append(ret, s)
	}
	return ret
}

// Size retrieves the number of entries stored upon the stack.
func (s *Stack) Size() int {
	return (len(s.entries))
}

// Push appends the specified value to the stack.
func (s *Stack) Push(value object.Object) {
	s.entries = append(s.entries, value)
}

// Pop removes a value from the stack.
//
// If the stack is currently empty then an error will be returned.
func (s *Stack) Pop() (object.Object, error) {
	if s.Empty() {
		return nil, errors.New("Pop from an empty stack")
	}

	// get the last entry.
	result := s.entries[len(s.entries)-1]

	// remove it
	s.entries = s.entries[:len(s.entries)-1]

	return result, nil
}
