package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
)

type SubCommand struct {
	// Description of the sub-command
	Description string

	// Pointer to the arguments we wish to parse.
	Args any

	// Callback used to configure the flags
	// of this sub-command.
	ConfigureFlags func(args any, flags *flag.FlagSet)

	// The entrypoint to this sub-command. Arguments
	// parsed from the CLI are type erased with `any`.
	Entrypoint func(args any) error
}

type runnerEntry struct {
	Description string
	Run         func()
}

type Runner struct {
	subCommands map[string]*runnerEntry
}

func NewRunner(subCommands map[string]*SubCommand) *Runner {
	r := &Runner{
		subCommands: make(map[string]*runnerEntry),
	}

	for name, subcmd := range subCommands {
		flagSet := flag.NewFlagSet(fmt.Sprintf("%s %s", os.Args[0], name), flag.ExitOnError)

		args := subcmd.Args
		entryPoint := subcmd.Entrypoint
		description := subcmd.Description

		if subcmd.ConfigureFlags != nil {
			subcmd.ConfigureFlags(args, flagSet)
		}

		flagSet.BoolFunc("h", fmt.Sprintf("print the docs of %s", name), func(flag string) error {
			flagSet.Usage()
			os.Exit(0)
			return nil
		})

		r.subCommands[name] = &runnerEntry{
			Description: description,
			Run: func() {
				flagSet.Parse(os.Args[2:])

				err := entryPoint(args)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					os.Exit(1)
				}

				os.Exit(0)
			},
		}
	}

	return r
}

func (r *Runner) Run() {
	topLevel := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	usage := topLevel.Usage

	type cmd struct {
		name string
		exec string
		desc string
	}

	topLevel.Usage = func() {
		// Print default usage
		usage()

		if len(r.subCommands) == 0 {
			return
		}

		maxWidth := -1
		allSubCmds := make([]cmd, 0, len(r.subCommands))

		for name, subcmd := range r.subCommands {
			c := cmd{
				name: name,
				exec: fmt.Sprintf("%s %s", os.Args[0], name),
				desc: subcmd.Description,
			}

			if len(c.exec) > maxWidth {
				maxWidth = len(c.exec)
			}

			allSubCmds = append(allSubCmds, c)
		}

		slices.SortFunc(allSubCmds, func(c1, c2 cmd) int {
			return strings.Compare(c1.name, c2.name)
		})

		// Print all subcmds
		fmt.Fprintln(os.Stderr)

		w := &tabwriter.Writer{}
		w.Init(os.Stderr, max(2+maxWidth, 8), 4, 4, ' ', tabwriter.TabIndent)

		for _, subCmd := range allSubCmds {
			fmt.Fprintf(w, "  %s\t%s\n", subCmd.exec, subCmd.desc)
		}

		w.Flush()
	}

	topLevel.Parse(os.Args)
	topLevelArgs := topLevel.Args()

	if len(topLevelArgs) < 2 {
		topLevel.Usage()
		os.Exit(1)
	}

	subcmd := r.subCommands[topLevelArgs[1]]
	if subcmd == nil {
		topLevel.Usage()
		os.Exit(1)
	}

	subcmd.Run()
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
