package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/tabwriter"
)

type Context interface {
	Stdout() *log.Logger
	Stderr() *log.Logger
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
func (e *Environment) GetLoggers() (*log.Logger, *log.Logger) {
	return log.New(e.stdout, "", 0), log.New(e.stderr, "", 0)
}
func (e *Environment) GetDefaultContext() Context {
	stdout, stderr := e.GetLoggers()
	return &defaultContext{stdout, stderr}
}

type defaultContext struct{ stdout, stderr *log.Logger }

func (dc *defaultContext) Stdout() *log.Logger { return dc.stdout }
func (dc *defaultContext) Stderr() *log.Logger { return dc.stderr }

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

	return &Program{
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
	}, nil
}

func (p *Program) ParseArgs(args []string) error {
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

	p.env.Args = args

	return p.parseArgs(args)
}

func (p *Program) Run(fn func(*Environment, Command, []string) error) error {
	_, stderr := p.env.GetLoggers()

	for _, cmd := range p.commands {
		if cmd.Name() == p.calledCmd {
			fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
			fs.SetOutput(p.env.stderr)
			cmd.Register(fs)

			fs.Usage = func() {
				stderr.Print(p.createCommandUsage(fs, cmd))
			}

			if p.printCmdHelp {
				fs.Usage()
				return nil
			}

			if err := fs.Parse(p.env.Args[2:]); err != nil {
				return fmt.Errorf("")
			}

			if err := fn(p.env, cmd, fs.Args()); err != nil {
				return fmt.Errorf("%s: %v", p.name, err)
			}

			return nil
		}
	}

	if p.calledCmd == defaultCommand && p.root != nil {
		fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
		fs.SetOutput(p.env.stderr)
		p.root.Register(fs)

		fs.Usage = func() {
			stderr.Print(p.createCommandUsage(fs, p.root))
		}

		if p.printCmdHelp {
			fs.Usage()
			return nil
		}

		if err := fs.Parse(p.env.Args[1:]); err != nil {
			return fmt.Errorf("")
		}

		return fn(p.env, p.root, fs.Args())
	} else if p.calledCmd == defaultCommand && p.root == nil {
		return fmt.Errorf(p.usage())
	}

	return nil
}

func (p *Program) createCommandUsage(fs *flag.FlagSet, cmd Command) string {
	var (
		usage bytes.Buffer
		flags bool
		fb    bytes.Buffer
		fw    = tabwriter.NewWriter(&fb, 0, 4, 2, ' ', 0)
	)

	fs.VisitAll(func(f *flag.Flag) {
		flags = true
		dv := f.DefValue
		if dv == "" {
			dv = "<none>"
		}
		fmt.Fprintf(fw, "\t-%s\t%s (default: %s)\n", f.Name, f.Usage, dv)
	})
	fw.Flush()

	if p.root.Name() == cmd.Name() {
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

func (p *Program) parseArgs(args []string) error {
	isHelp := func(arg string) bool {
		return strings.Contains(strings.ToLower(arg), "help") || strings.ToLower(arg) == "-h"
	}

	isCommand := func(arg string) bool {
		for _, cmd := range p.commands {
			if cmd.Name() == arg {
				return true
			}
		}
		return false
	}

	switch len(args) {
	case 0, 1:
		p.calledCmd = defaultCommand
	case 2:
		if isHelp(args[1]) {
			return fmt.Errorf(p.usage())
		} else if isCommand(args[1]) {
			p.calledCmd = args[1]
		} else if p.root != nil {
			p.calledCmd = defaultCommand
		} else {
			return fmt.Errorf("%s: %s: no such command", p.name, args[1])
		}
	default:
		if isHelp(args[1]) {
			p.calledCmd = args[2]
			p.printCmdHelp = true
		} else if isCommand(args[1]) {
			p.calledCmd = args[1]
		} else if p.root != nil {
			p.calledCmd = defaultCommand
		} else {
			return fmt.Errorf("%s: %s: no such command", p.name, args[1])
		}
	}

	return nil
}
