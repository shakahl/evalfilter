package object

import "fmt"

// Integer wraps int64 and implements the Object interface.
type Integer struct {
	// Value holds the integer value this object wraps
	Value int64
}

// Inspect returns a string-representation of the given object.
func (i *Integer) Inspect() string {
	return fmt.Sprintf("%d", i.Value)
}

// Type returns the type of this object.
func (i *Integer) Type() Type {
	return INTEGER
}

// True returns whether this object wraps a true-like value.
//
// Used when this object is the conditional in a comparison, etc.
func (i *Integer) True() bool {
	return (i.Value > 0)
}

// ToInterface converts this object to a go-interface, which will allow
// it to be used naturally in our sprintf/printf primitives.
//
// It might also be helpful for embedded users.
func (i *Integer) ToInterface() interface{} {
	return i.Value
}

// Increase implements the Increment interface, and allows the postfix
// "++" operator to be applied to integer-objects
func (i *Integer) Increase() {
	i.Value++
}

// Decrease implements the Decrement interface, and allows the postfix
// "--" operator to be applied to integer-objects
func (i *Integer) Decrease() {
	i.Value--
}

// HashKey returns a hash key for the given object.
func (i *Integer) HashKey() HashKey {
	return HashKey{Type: i.Type(), Value: uint64(i.Value)}
}

// JSON converts this object to a JSON string.
func (i *Integer) JSON() (string, error) {
	return fmt.Sprintf("%d", i.Value), nil
}

// Ensure this object implements the expected interfaces.
var _ Decrement = &Integer{}
var _ Hashable = &Integer{}
var _ Increment = &Integer{}
var _ JSONAble = &Integer{}
