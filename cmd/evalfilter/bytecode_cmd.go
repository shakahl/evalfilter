package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/google/subcommands"
	"github.com/skx/evalfilter/v2"
)

type bytecodeCmd struct {
}

//
// Glue
//
func (*bytecodeCmd) Name() string     { return "bytecode" }
func (*bytecodeCmd) Synopsis() string { return "Show the bytecode for a script." }
func (*bytecodeCmd) Usage() string {
	return `bytecode script1 script2 .. [scriptN]:
`
}

//
// Flag setup
//
func (p *bytecodeCmd) SetFlags(f *flag.FlagSet) {
}

//
// Show the bytecode of the given script.
//
func (p *bytecodeCmd) Run(file string) {

	//
	// Read the file contents.
	//
	dat, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Printf("Error reading file %s - %s\n", file, err.Error())
		return
	}

	//
	// Create the evaluator.
	//
	eval := evalfilter.New(string(dat))

	//
	// Prepare
	//
	err = eval.Prepare()
	if err != nil {
		fmt.Printf("Error compiling:%s\n", err.Error())
		return
	}

	//
	// Dump the script.
	//
	err = eval.Dump()
	if err != nil {
		fmt.Printf("Failed to dump script: %s\n", err.Error())
		return
	}

}

//
// Entry-point.
//
func (p *bytecodeCmd) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {

	//
	// For each file we've been passed; run it.
	//
	for _, file := range f.Args() {
		p.Run(file)
	}

	return subcommands.ExitSuccess

}
