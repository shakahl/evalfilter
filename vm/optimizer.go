// This file contains some simple code which optimizes previously-generated bytecode operations.
//
// There are a couple of basic things we do:
//
// 1. The first thing we do is collapse maths which uses (integer) constants
// to directly contain the results - rather than using the stack as
// expected.
//
// 2. Once we've done that we can convert some jumping operations which might
// use those results into unconditional jumps, or NOPs as appropriate.
//
// Brief discussion in this blog post:
//
// https://blog.steve.fi/adventures_optimizing_a_bytecode_based_scripting_language.html

package vm

import (
	"encoding/binary"
	"fmt"

	"github.com/skx/evalfilter/v2/code"
)

// optimize optimizes our bytecode by working over the program
// simplifying where it can.
//
// Mostly this is designed to collapse "maths", simple comparisons,
// and remove dead-code in cases where that can be proven safe.
//
// This function returns the number of opcodes/operands removed from
// the bytecode.  i.e. 0 for no changes, 10 for the removal of ten
// instructions, etc.
func (vm *VM) optimizeBytecode() int {

	// Starting length of bytecode.
	sz := len(vm.bytecode)

	// Attempt to collapse maths until we
	// can do so no more - or until we see
	// an error.
	for {

		changed, err := vm.optimizeMaths()

		// error?  failed to change?
		//
		// Then stop trying.
		if err != nil || !changed {
			break
		}

	}

	// Attempt to collapse jumps
	for vm.optimizeJumps() {
	}

	// Remove NOPs
	vm.removeNOPs()

	// Finally kill dead code
	vm.removeDeadCode()

	// And return the changes.
	return (sz - len(vm.bytecode))
}

// optimizeMaths updates simple mathematical operations in-place.
//
// Given an expression such as "2 * 3" we would expect that to be encoded as:
//
//  000000 OpPush 2
//  000003 OpPush 3
//  000006 OpMul
//
// That can be replaced by "OpPush 6", "NOP", "NOP", "NOP", & "NOP".
//
func (vm *VM) optimizeMaths() (bool, error) {

	//
	// Constants we've seen - and their offsets within the
	// bytecode array.
	//
	type Constants struct {
		// offset is where we found this constant instruction.
		offset int

		// value is the (integer) constant value referred to.
		value int
	}

	//
	// Keep track of adjacent values here.
	//
	var args []Constants

	//
	// Did we make changes?
	//
	changed := false

	//
	// Walk over the bytecode
	//
	vm.WalkBytecode(func(offset int, opCode code.Opcode, opArg interface{}) (bool, error) {

		//
		// Now we do the magic.
		//
		switch opCode {

		case code.OpPush:

			//
			// If we see a constant being pushed we
			// add that to our list tracking such things.
			//
			args = append(args, Constants{offset: offset, value: opArg.(int)})

		case code.OpNop:

			//
			// If we see a OpNop instruction that might
			// be as a result of previous optimization
			//
			// We're going to pretend we didn't see a
			// thing.
			//

		case code.OpEqual, code.OpNotEqual:

			//
			// Comparison-tests.
			//
			// If we have two (constant) arguments then
			// we can collapse the test into a simple "True"
			// or "False"
			//
			// If we didn't then it is something we
			// should leave alone.
			//
			if len(args) >= 2 {

				// Get the arguments to the comparison
				a := args[len(args)-1]
				b := args[len(args)-2]

				// Replace the first argument with nop
				vm.bytecode[a.offset] = byte(code.OpNop)
				vm.bytecode[a.offset+1] = byte(code.OpNop)
				vm.bytecode[a.offset+2] = byte(code.OpNop)

				// Replace the second argument with nop
				vm.bytecode[b.offset] = byte(code.OpNop)
				vm.bytecode[b.offset+1] = byte(code.OpNop)
				vm.bytecode[b.offset+2] = byte(code.OpNop)

				//
				// Now we can replace the comparison
				// instruction with either "True" or "False"
				// depending on whether the constant values
				// match.
				//
				if opCode == code.OpEqual {
					if a.value == b.value {
						vm.bytecode[offset] = byte(code.OpTrue)
					} else {
						vm.bytecode[offset] = byte(code.OpFalse)
					}
				}
				if opCode == code.OpNotEqual {
					if a.value != b.value {
						vm.bytecode[offset] = byte(code.OpTrue)
					} else {
						vm.bytecode[offset] = byte(code.OpFalse)
					}
				}

				// Made a change to the bytecode.
				changed = true
				return false, nil
			}

			// reset our argument counters.
			args = nil

		case code.OpMul, code.OpAdd, code.OpSub, code.OpDiv:

			//
			// Primitive maths operation.
			//
			// If we have two (constant) arguments then
			// we can collapse the maths operation into
			// the result directly.
			//
			// i.e. "OpPush 1", "OpPush 3", "OpAdd" can
			// become "OpPush 4" with a series of NOps.
			//
			// If we didn't then it is something we
			// should leave alone.
			//
			if len(args) >= 2 {

				// Get the two arguments
				a := args[len(args)-1]
				b := args[len(args)-2]

				// Calculate the result.
				//
				// We only allow integers in the range
				// 0x0000-0xFFFF to be stored inline
				// so not all maths can be collapsed.
				//
				result := 0

				if opCode == code.OpMul {
					result = a.value * b.value
				}
				if opCode == code.OpAdd {
					result = a.value + b.value
				}
				if opCode == code.OpSub {
					result = b.value - a.value
				}
				if opCode == code.OpDiv {

					// found division by zero
					if a.value == 0 {
						return false, fmt.Errorf("attempted division by zero")
					}
					result = b.value / a.value
				}

				if result%1 == 0 && result >= 0 && result <= 65534 {
					// Make a buffer for the argument
					data := make([]byte, 2)
					binary.BigEndian.PutUint16(data, uint16(result))

					// Replace the argument
					vm.bytecode[a.offset+1] = data[0]
					vm.bytecode[a.offset+2] = data[1]

					// Replace the second argument-load with nop
					vm.bytecode[b.offset] = byte(code.OpNop)
					vm.bytecode[b.offset+1] = byte(code.OpNop)
					vm.bytecode[b.offset+2] = byte(code.OpNop)

					// and finally replace the math-operation
					// itself with a Nop.
					vm.bytecode[offset] = byte(code.OpNop)

					// We changed something, so we stop now.
					changed = true
					return false, nil
				}

				// The result was not something we can
				// replace.  Keep going.
			}

			// reset our argument counters.
			args = nil

		default:

			//
			// If we get here then we've found an instruction
			// that wasn't a constant load, and wasn't something
			// we can fold.
			//
			// So we have to reset our list of constants
			// because we've found something we can't
			// optimize, rewrite, or improve.
			//
			// Shame.
			//
			args = nil
		}

		// no error, keep going
		return true, nil
	})

	//
	// If we get here we walked all the way over our bytecode
	// and made zero changes.
	//
	return changed, nil
}

// optimizeJumps updates simple jump operations in-place.
//
// This is only possible if a script used some simple integer-maths
// operations as a conditional.  But if that were true we'd end up
// with code like this:
//
//   OpTrue
//   OpJumpIfFalse 0x1234
//
// In this case we push a `TRUE` value to the stack, but only jump
// if the stack-top is `FALSE`.  In this case the jump will never be
// taken.  So it is removed.
//
// The same happens in reverse.  This code:
//
//   OpFalse
//   OpJumpIfFalse 0x1234
//
// Can be rewritten to `OpJump 0x1234` as it will always be taken.
//
func (vm *VM) optimizeJumps() bool {

	//
	// Previous opcode.
	//
	prevOp := code.OpNop

	//
	// Did we make changes?
	//
	changed := false

	//
	// Walk the bytecode.
	//
	vm.WalkBytecode(func(offset int, opCode code.Opcode, opArg interface{}) (bool, error) {

		//
		// Now we do the magic.
		//
		switch opCode {

		case code.OpJumpIfFalse:

			//
			// If the previous opcode was "OpTrue" then
			// the jump is pointless.
			//
			if prevOp == code.OpTrue {

				// wipe the previous instruction, (OpTrue)
				vm.bytecode[offset-1] = byte(code.OpNop)

				// wipe this jump
				vm.bytecode[offset] = byte(code.OpNop)
				vm.bytecode[offset+1] = byte(code.OpNop)
				vm.bytecode[offset+2] = byte(code.OpNop)

				// We made a change
				changed = true

				// No error, and stop processing,
				return false, nil
			}

			//
			// If the previous opcode was "OpFalse" then
			// the jump is always going to be taken.
			//
			// So remove the OpFalse, and make the jump
			// unconditional
			//
			if prevOp == code.OpFalse {

				//
				// If we get here we have:
				//
				//   OpFalse
				//   OpJumpIfFalse Target
				//
				//     .. instructions ..
				//
				// Target:
				//     .. instructions ..
				//
				// Since the jump is unconditional
				// the instructions in the middle
				// can be nuked, as well as the
				// `OpFalse` and `OpJumpIfFalse`
				//

				i := offset - 1
				for i < opArg.(int) {
					vm.bytecode[i] = byte(code.OpNop)
					i++
				}

				// We made a change
				changed = true

				// No error, and stop processing,
				return false, nil
			}

		}

		//
		// Save the previous opcode.
		//
		prevOp = opCode

		//
		// No error, keep walking.
		//
		return true, nil
	})

	//
	// This function will be invoked until no changes
	// are made to the bytecode.
	//
	return changed
}

// removeNOPs removes any inline NOP instructions.
//
// It also rewrites the destinations for jumps as appropriate, to
// cope with the changed offsets.
func (vm *VM) removeNOPs() {

	//
	// Temporary instructions.
	//
	var tmp code.Instructions

	//
	// Map from old offset to new offset.
	//
	rewrite := make(map[int]int)

	//
	// Walk the bytecode.
	//
	vm.WalkBytecode(func(offset int, opCode code.Opcode, opArg interface{}) (bool, error) {

		//
		// Now we do the magic.
		//
		switch opCode {

		case code.OpNop:
			//
			// Do nothing here, with the instruction itself.
			//
			// However we have to update our map with the mapping
			// of old-instruction to new, because there might have
			// been a jump which pointed to this NOP instruction.
			//
			rewrite[offset] = len(tmp)

		default:

			//
			// Append the instruction(s)
			// to the temporary list.
			//
			// Record our new offset
			//
			//  IP has the current offset.
			//
			//  The new/changed offset will be the current
			// position - i.e. the length of the existing
			// instruction set.  Before we add it.
			//
			rewrite[offset] = len(tmp)

			//
			// Copy the instruction.
			//
			tmp = append(tmp, byte(opCode))

			//
			// Copy any argument.
			//
			if opArg != nil {
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, uint16(opArg.(int)))

				tmp = append(tmp, b...)
			}
		}

		// No error, keep going
		return true, nil
	})

	//
	// We've walked over our code, writing a new jump-table
	// and removing any OpNop instructions we came across.
	//
	// If we _didn't_ remove any OpNop instructions then
	// we've no need to proceed further and update our code.
	//
	if len(vm.bytecode) == len(tmp) {
		return
	}

	//
	// If we've done this correctly we've now got a temporary
	// program with no NOPs.   We now need to patch up
	// the jump targets
	//
	ip := 0
	ln := len(tmp)
	for ip < ln {

		// Get the instruction.
		op := code.Opcode(tmp[ip])

		// And its length
		opLen := code.Length(op)

		// Get the optional argument
		opArg := 0
		if opLen > 1 {
			opArg = int(binary.BigEndian.Uint16(tmp[ip+1 : ip+3]))
		}

		//
		// Now we do the magic.
		//
		switch op {

		// If this was a jump we'll have to change
		// the target.
		//
		// We use the rewrite map we already made,
		// which contains "old -> new".
		//
		case code.OpJump, code.OpJumpIfFalse:

			// The old destination is in "opArg".
			//
			// So the new one `rewrite[old]`
			//
			newDst, ok := rewrite[opArg]
			if !ok {

				//
				// Did we fail to find an updated/ location?  That's a bug.
				//
				// Since we can't do anything we'll just avoid rewriting further.
				//
				return
			}

			// Make into a two-byte pair.
			b := make([]byte, 2)
			binary.BigEndian.PutUint16(b, uint16(newDst))

			// Update in-place
			tmp[ip+1] = b[0]
			tmp[ip+2] = b[1]

		}

		//
		// Next instruction.
		//
		ip += opLen
	}

	//
	// Replace the instructions.
	//
	vm.bytecode = tmp
}

// removeDeadCode does the bare minimum of dead-code removal:
//
// If a script has no Jumps in it we stop processing at the first Return.
func (vm *VM) removeDeadCode() {

	//
	// Temporary instructions.
	//
	var tmp code.Instructions

	//
	// Did we make an optimization?
	//
	changed := false

	//
	// Walk the bytecode.
	//
	vm.WalkBytecode(func(offset int, opCode code.Opcode, opArg interface{}) (bool, error) {

		//
		// Now we do the magic.
		//
		switch opCode {

		case code.OpJumpIfFalse, code.OpJump:
			// Stop walking
			return false, nil

		case code.OpReturn:

			// Record the return, and also stop walking
			tmp = append(tmp, byte(code.OpReturn))
			changed = true
			return false, nil
		default:

			tmp = append(tmp, byte(opCode))
			if opArg != nil {

				// Make a buffer for the arg
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, uint16(opArg.(int)))

				// append
				tmp = append(tmp, b...)
			}
		}

		// keep walking
		return true, nil
	})

	//
	// Replace the instructions, if we made a sane change
	//
	if changed {
		vm.bytecode = tmp
	}
}
