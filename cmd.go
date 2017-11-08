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

type Environment interface {
	WorkingDir() string
	Args() []string
	Env() []string
	GetStdio() (io.Writer, io.Writer)
	GetLoggers() (*log.Logger, *log.Logger)
	GetDefaultContext() Context
}

type config struct {
	wd             string
	args           []string
	env            []string
	stdout, stderr io.Writer
}

func (c *config) WorkingDir() string               { return c.wd }
func (c *config) Args() []string                   { return c.args }
func (c *config) Env() []string                    { return c.env }
func (c *config) GetStdio() (io.Writer, io.Writer) { return c.stdout, c.stderr }
func (c *config) GetLoggers() (stdout *log.Logger, stderr *log.Logger) {
	return log.New(c.stdout, "", 0), log.New(c.stderr, "", 0)
}
func (c *config) GetDefaultContext() Context {
	stdout, stderr := c.GetLoggers()
	return &defaultContext{stdout, stderr}
}

type defaultContext struct{ stdout, stderr *log.Logger }

func (dc *defaultContext) Stdout() *log.Logger { return dc.stdout }
func (dc *defaultContext) Stderr() *log.Logger { return dc.stderr }

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
		config: &config{
			wd:     wd,
			env:    os.Environ(),
			stdout: os.Stdout,
			stderr: os.Stderr,
		},
	}, nil
}

type Program struct {
	name         string
	desc         string
	root         Command
	commands     []Command
	config       *config
	usage        func() string
	calledCmd    string
	printCmdHelp bool
}

func (p *Program) ParseArgs(args []string) error {
	p.usage = func() string {
		var u bytes.Buffer

		if len(p.desc) > 0 {
			fmt.Fprintln(&u, strings.TrimSpace(p.desc))
		}

		if len(p.commands) > 0 {
			fmt.Fprintf(&u, "Usage: %s <command>\n", p.name)
			fmt.Fprintln(&u, "")
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

	p.config.args = args

	return p.parseArgs(args)
}

func (p *Program) Run(fn func(Environment, Command, []string) error) error {
	_, stderr := p.config.GetLoggers()

	for _, cmd := range p.commands {
		if cmd.Name() == p.calledCmd {
			fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
			fs.SetOutput(p.config.stderr)
			cmd.Register(fs)

			fs.Usage = func() {
				stderr.Print(p.createCommandUsage(fs, cmd))
			}

			if p.printCmdHelp {
				fs.Usage()
				return nil
			}

			if err := fs.Parse(p.config.args[2:]); err != nil {
				return fmt.Errorf("%s: %s: unable to parse arguments: %v", p.name, p.calledCmd, err)
			}

			if err := fn(p.config, cmd, fs.Args()); err != nil {
				return fmt.Errorf("%s: %v", p.name, err)
			}

			return nil
		}
	}

	if p.calledCmd == "default" && p.root != nil {
		fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
		fs.SetOutput(p.config.stderr)
		p.root.Register(fs)

		fs.Usage = func() {
			stderr.Print(p.createCommandUsage(fs, p.root))
		}

		if p.printCmdHelp {
			fs.Usage()
			return nil
		}

		if err := fs.Parse(p.config.args[1:]); err != nil {
			return fmt.Errorf("%s: unable to parse arguments: %v", p.name, err)
		}

		if err := fn(p.config, p.root, fs.Args()); err != nil {
			return fmt.Errorf("%s: %v", p.name, err)
		}

		return nil
	} else if p.calledCmd == "default" && p.root == nil {
		return fmt.Errorf(p.usage())
	}

	return fmt.Errorf("%s: %s: no such command", p.name, p.calledCmd)
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
		p.calledCmd = "default"
	case 2:
		if isHelp(args[1]) {
			return fmt.Errorf(p.usage())
		} else if isCommand(args[1]) {
			p.calledCmd = args[1]
		} else if p.root != nil {
			p.calledCmd = "default"
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
			p.calledCmd = "default"
		} else {
			return fmt.Errorf("%s: %s: no such command", p.name, args[1])
		}
	}

	return nil
}
