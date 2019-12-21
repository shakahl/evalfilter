// This file contains some simple code which optimizes our
// previously-generated bytecode operations.
//
// There are a couple of basic things we do:
//
// The first thing we do is collapse maths which uses (integer) constants
// to directly contain the results - rather than using the stack as
// expected.
//
// Once we've done that we can convert some jumping operations which might
// use those results into unconditional jumps, or NOPs as appropriate.
//
// Brief discussion in this blog post:
//
// https://blog.steve.fi/adventures_optimizing_a_bytecode_based_scripting_language.html

package evalfilter

import (
	"encoding/binary"

	"github.com/skx/evalfilter/v2/code"
)

// optimize optimizes our bytecode by working over the program
// simplifying where it can.
//
// Mostly this is designed to collapse "maths", simple comparisons,
// and remove dead-code in cases where that can be proven safe.
func (e *Eval) optimize() int {

	// Count changes we've made
	changes := 0

	// Attempt to collapse maths
	for e.optimizeMaths() {
		changes++
	}

	// Attempt to collapse jumps
	for e.optimizeJumps() {
		changes++
	}

	// Remove NOPs
	e.removeNOPs()

	// Finally kill dead code
	e.removeDeadCode()

	// And return the changes.
	return changes
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
func (e *Eval) optimizeMaths() bool {

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
	// We're going to walk over the bytecode from start to
	// finish.
	//
	ip := 0
	ln := len(e.instructions)

	//
	// Keep track of adjacent values here.
	//
	var args []Constants

	//
	// Walk the bytecode.
	//
	for ip < ln {

		//
		// Get the next opcode
		//
		op := code.Opcode(e.instructions[ip])

		//
		// Find out how long it is.
		//
		opLen := code.Length(op)

		//
		// If the opcode is more than a single byte long
		// we read the argument here.
		//
		opArg := 0
		if opLen > 1 {

			//
			// Note in the future we might have to cope
			// with opcodes with more than a single argument,
			// and they might be different sizes.
			//
			opArg = int(binary.BigEndian.Uint16(e.instructions[ip+1 : ip+3]))
		}

		//
		// Now we do the magic.
		//
		switch op {

		case code.OpPush:

			//
			// If we see a constant being pushed we
			// add that to our list tracking such things.
			//
			args = append(args, Constants{offset: ip, value: opArg})

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
				e.instructions[a.offset] = byte(code.OpNop)
				e.instructions[a.offset+1] = byte(code.OpNop)
				e.instructions[a.offset+2] = byte(code.OpNop)

				// Replace the second argument with nop
				e.instructions[b.offset] = byte(code.OpNop)
				e.instructions[b.offset+1] = byte(code.OpNop)
				e.instructions[b.offset+2] = byte(code.OpNop)

				//
				// Now we can replace the comparison
				// instruction with either "True" or "False"
				// depending on whether the constant values
				// match.
				//
				if op == code.OpEqual {
					if a.value == b.value {
						e.instructions[ip] = byte(code.OpTrue)
					} else {
						e.instructions[ip] = byte(code.OpFalse)
					}
				}
				if op == code.OpNotEqual {
					if a.value != b.value {
						e.instructions[ip] = byte(code.OpTrue)
					} else {
						e.instructions[ip] = byte(code.OpFalse)
					}
				}

				// Made a change to the bytecode.
				return true
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

				if op == code.OpMul {
					result = a.value * b.value
				}
				if op == code.OpAdd {
					result = a.value + b.value
				}
				if op == code.OpSub {
					result = b.value - a.value
				}
				if op == code.OpDiv {
					result = b.value / a.value
				}

				if result%1 == 0 && result >= 0 && result <= 65534 {
					e.changeOperand(a.offset, result)

					// Replace the second argument-load with nop
					e.instructions[b.offset] = byte(code.OpNop)
					e.instructions[b.offset+1] = byte(code.OpNop)
					e.instructions[b.offset+2] = byte(code.OpNop)

					// and finally replace the math-operation
					// itself with a Nop.
					e.instructions[ip] = byte(code.OpNop)

					// We changed something, so we stop now.
					return true
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

		//
		// Continue to the next instruction.
		//
		ip += opLen
	}

	//
	// If we get here we walked all the way over our bytecode
	// and made zero changes.
	//
	return false
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
func (e *Eval) optimizeJumps() bool {

	//
	// We're going to walk over the bytecode from start to
	// finish.
	//
	ip := 0
	ln := len(e.instructions)

	//
	// Previous opcode.
	//
	prevOp := code.OpNop

	//
	// Walk the bytecode.
	//
	for ip < ln {

		//
		// Get the next opcode
		//
		op := code.Opcode(e.instructions[ip])

		//
		// Find out how long it is.
		//
		opLen := code.Length(op)

		//
		// Now we do the magic.
		//
		switch op {

		case code.OpJumpIfFalse:

			//
			// If the previous opcode was "OpTrue" then
			// the jump is pointless.
			//
			if prevOp == code.OpTrue {

				// wipe the previous instruction, (OpTrue)
				e.instructions[ip-1] = byte(code.OpNop)

				// wipe this jump
				e.instructions[ip] = byte(code.OpNop)
				e.instructions[ip+1] = byte(code.OpNop)
				e.instructions[ip+2] = byte(code.OpNop)

				return true
			}

			//
			// If the previous opcode was "OpFalse" then
			// the jump is always going to be taken.
			//
			// So remove the OpFalse, and make the jump
			// unconditional
			//
			if prevOp == code.OpFalse {

				// wipe the previous instruction, (OpFalse)
				e.instructions[ip-1] = byte(code.OpNop)

				// This jump is now unconditional
				e.instructions[ip] = byte(code.OpJump)

				return true
			}

		}

		//
		// Continue to the next instruction.
		//
		ip += opLen

		//
		// Save the previous opcode.
		//
		prevOp = op
	}

	//
	// If we get here we walked all the way over our bytecode
	// and made zero changes.
	//
	return false
}

// removeNOPs removes any inline NOP instructions.
//
// It also rewrites the destinations for jumps as appropriate, to
// cope with the changed offsets.
func (e *Eval) removeNOPs() {

	//
	// Start.
	//
	ip := 0
	ln := len(e.instructions)

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
	for ip < ln {

		// Get the instruction & length.
		op := code.Opcode(e.instructions[ip])
		opLen := code.Length(op)

		// Get the opcode's argument, if any.
		opArg := 0
		if opLen > 1 {
			opArg = int(binary.BigEndian.Uint16(e.instructions[ip+1 : ip+3]))
		}

		//
		// Now we do the magic.
		//
		switch op {

		case code.OpNop:
			//
			// Do nothing here, with the instruction itself.
			//
			// However we have to update our map with the mapping
			// of old-instruction to new, because there might have
			// been a jump which pointed to this NOP instruction.
			//
			rewrite[ip] = len(tmp)

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
			rewrite[ip] = len(tmp)

			//
			// Copy the instruction.
			//
			tmp = append(tmp, byte(op))

			//
			// Copy any argument.
			//
			if opLen > 1 {
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, uint16(opArg))

				tmp = append(tmp, b...)
			}
		}
		ip += opLen
	}

	//
	// If we've done this correctly we've now got a temporary
	// program with no NOPs.   We now need to patch up
	// the jump targets
	//
	ip = 0
	ln = len(tmp)
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
			newDst := rewrite[opArg]

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
	e.instructions = tmp
}

// removeDeadCode does the bare minimum of dead-code removal:
//
// If a script has no Jumps in it we stop processing at the first Return.
func (e *Eval) removeDeadCode() {

	//
	// Start.
	//
	ip := 0
	ln := len(e.instructions)

	//
	// Temporary instructions.
	//
	var tmp code.Instructions

	run := true

	//
	// Walk the bytecode.
	//
	for ip < ln && run {

		//
		// Get the next opcode
		//
		op := code.Opcode(e.instructions[ip])

		//
		// Find out how long it is.
		//
		opLen := code.Length(op)

		//
		// If the opcode is more than a single byte long
		// we read the argument here.
		//
		opArg := 0
		if opLen > 1 {

			//
			// Note in the future we might have to cope
			// with opcodes with more than a single argument,
			// and they might be different sizes.
			//
			opArg = int(binary.BigEndian.Uint16(e.instructions[ip+1 : ip+3]))
		}

		//
		// Now we do the magic.
		//
		switch op {

		case code.OpJumpIfFalse, code.OpJump:
			return

		case code.OpReturn:

			// Stop once we've seen the first return
			run = false

			tmp = append(tmp, byte(code.OpReturn))

		default:

			tmp = append(tmp, byte(op))
			if opLen > 1 {

				// Make a buffer for the arg
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, uint16(opArg))

				// append
				tmp = append(tmp, b...)
			}
		}
		ip += opLen
	}

	//
	// Replace the instructions.
	//
	e.instructions = tmp
}
