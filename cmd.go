package cmd

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/tabwriter"
)

var Out = log.New(os.Stdout, "", 0)
var Err = log.New(os.Stderr, "", 0)

type Context interface {
	WorkingDir() string
}

type Command interface {
	Name() string
	Args() string
	Desc() string
	Help() string
	Register(*flag.FlagSet)
	Run(Context, []string) error
}

type Environment struct {
	WorkingDir     string
	Args           []string
	Env            []string
	stdout, stderr io.Writer
}

func (e *Environment) GetStdio() (io.Writer, io.Writer) { return e.stdout, e.stderr }
func (e *Environment) GetDefaultContext() Context {
	return &defaultContext{
		wd: e.WorkingDir,
	}
}

type defaultContext struct {
	wd string // working directory
}

var _ Context = (*defaultContext)(nil)

func (dc *defaultContext) WorkingDir() string {
	return dc.wd
}

type Program struct {
	name         string
	desc         string
	root         Command
	commands     []Command
	env          *Environment
	usage        func() string
	calledCmd    string
	printCmdHelp bool
}

func NewProgram(name string, desc string, root Command, cmds []Command) (*Program, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("unable to get working directory: %v", err)
	}

	p := &Program{
		name:     name,
		desc:     desc,
		root:     root,
		commands: cmds,
		env: &Environment{
			WorkingDir: wd,
			Env:        os.Environ(),
			stdout:     os.Stdout,
			stderr:     os.Stderr,
		},
	}

	p.createProgramUsage()

	return p, nil
}

func (p *Program) createProgramUsage() {
	p.usage = func() string {
		var u bytes.Buffer

		if len(p.commands) > 0 {
			fmt.Fprintf(&u, "Usage: %s <command>\n", p.name)
			fmt.Fprintln(&u, "")
			if len(p.desc) > 0 {
				fmt.Fprintln(&u, strings.TrimSpace(p.desc))
				fmt.Fprintln(&u, "")
			}
			fmt.Fprintln(&u, "Commands:")
			fmt.Fprintln(&u, "")
			w := tabwriter.NewWriter(&u, 0, 0, 2, ' ', 0)
			if p.root != nil {
				fmt.Fprintf(w, "\t[default]\t%s\n", p.root.Name())
			}
			for _, cmd := range p.commands {
				fmt.Fprintf(w, "\t%s\t%s\n", cmd.Name(), cmd.Desc())
			}
			w.Flush()
			fmt.Fprintln(&u, "")
		} else {
			fs := flag.NewFlagSet(p.root.Name(), flag.ContinueOnError)
			p.root.Register(fs)
			fmt.Fprintln(&u, strings.TrimSpace(p.createCommandUsage(fs, p.root)))
		}

		if len(p.commands) > 0 {
			fmt.Fprintf(&u, "Use \"%s help [command]\" for more information about a command.\n", p.name)
		}

		return u.String()
	}
}

var ErrParseArgs = errors.New("could not parse arguments")

func (p *Program) Run(args []string, fn func(*Environment, Command, []string) error) error {
	p.env.Args = args
	if err := p.parseArgs(args); err != nil {
		return err
	}

	for _, cmd := range p.commands {
		if cmd.Name() == p.calledCmd {
			fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
			fs.SetOutput(p.env.stderr)
			cmd.Register(fs)

			fs.Usage = func() {
				Err.Print(p.createCommandUsage(fs, cmd))
			}

			if p.printCmdHelp {
				fs.Usage()
				return nil
			}

			if err := fs.Parse(p.env.Args[2:]); err != nil {
				return ErrParseArgs
			}

			return fn(p.env, cmd, fs.Args())
		}
	}

	if p.calledCmd == defaultCommand && p.root != nil {
		fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
		fs.SetOutput(p.env.stderr)
		p.root.Register(fs)

		fs.Usage = func() {
			Err.Print(p.createCommandUsage(fs, p.root))
		}

		if p.printCmdHelp {
			fs.Usage()
			return nil
		}

		if err := fs.Parse(p.env.Args[1:]); err != nil {
			return ErrParseArgs
		}

		return fn(p.env, p.root, fs.Args())
	} else if p.calledCmd == defaultCommand && p.root == nil {
		return &ErrNoDefaultCommand{
			usage: p.usage(),
		}
	}

	return nil
}

// ErrNoDefaultCommand is returned when the default command is called but no command is provided to
// handle it.
type ErrNoDefaultCommand struct {
	usage string
}

// Error implements the error interface
func (e *ErrNoDefaultCommand) Error() string {
	return e.usage
}

// prettyDefaultValue sets the default value to `<none>` if it is blank
func prettyDefaultValue(s string) (dv string) {
	dv = s
	if s == "" {
		dv = "<none>"
	}
	return dv
}

func (p *Program) createCommandUsage(fs *flag.FlagSet, cmd Command) string {
	var (
		usage bytes.Buffer
		flags bool
		fb    bytes.Buffer
		fw    = tabwriter.NewWriter(&fb, 0, 4, 2, ' ', 0)
	)

	hold := make(map[string]*flag.Flag)
	fs.VisitAll(func(f *flag.Flag) {
		flags = true
		if hf, ok := hold[f.Usage]; ok {
			fmt.Fprintf(fw, "\t-%s -%s\t%s (default: %s)\n", hf.Name, f.Name, f.Usage, prettyDefaultValue(f.DefValue))
			delete(hold, f.Usage)
		} else {
			hold[f.Usage] = f
			return
		}
	})
	for _, f := range hold {
		fmt.Fprintf(fw, "\t-%s\t%s (default: %s)\n", f.Name, f.Usage, prettyDefaultValue(f.DefValue))
	}
	fw.Flush()

	if p.root != nil && p.root.Name() == cmd.Name() {
		fmt.Fprintf(&usage, "Usage: %s %s\n", p.name, cmd.Args())
	} else {
		fmt.Fprintf(&usage, "Usage: %s %s %s\n", p.name, cmd.Name(), cmd.Args())
	}

	fmt.Fprintln(&usage, "")
	fmt.Fprintln(&usage, strings.TrimSpace(cmd.Help()))
	fmt.Fprintln(&usage, "")
	if flags {
		fmt.Fprintln(&usage, "Flags:")
		fmt.Fprintln(&usage, "")
		fmt.Fprintln(&usage, fb.String())
	}

	return usage.String()
}

const defaultCommand = "default"

// isHelp checks whether the provided args is for help
func isHelp(arg string) bool {
	return strings.Contains(strings.ToLower(arg), "help") || strings.ToLower(arg) == "-h"
}

// isCommand checks if the provided arg is a command
func isCommand(arg string, cmds []Command) bool {
	for _, cmd := range cmds {
		if cmd.Name() == arg {
			return true
		}
	}
	return false
}

func (p *Program) parseArgs(args []string) error {
	switch len(args) {
	case 0, 1:
		p.calledCmd = defaultCommand
	case 2:
		if isHelp(args[1]) {
			return fmt.Errorf(p.usage())
		} else if isCommand(args[1], p.commands) {
			p.calledCmd = args[1]
		} else if p.root != nil {
			p.calledCmd = defaultCommand
		} else {
			return &ErrNoSuchCommand{
				programName: p.name,
				commandName: args[1],
			}
		}
	default:
		if isHelp(args[1]) {
			p.calledCmd = args[2]
			p.printCmdHelp = true
		} else if isCommand(args[1], p.commands) {
			p.calledCmd = args[1]
		} else if p.root != nil {
			p.calledCmd = defaultCommand
		} else {
			return &ErrNoSuchCommand{
				programName: p.name,
				commandName: args[1],
			}
		}
	}

	return nil
}

// ErrNoSuchCommand is returned when the requested command is not found
type ErrNoSuchCommand struct {
	programName string
	commandName string
}

// Error implements the error interface
func (e *ErrNoSuchCommand) Error() string {
	return fmt.Sprintf("%s: %s: no such command", e.programName, e.commandName)
}
