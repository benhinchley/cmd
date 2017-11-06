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
		fmt.Fprintf(&u, "Usage: %s <command>\n", p.name)
		fmt.Fprintln(&u, "")
		fmt.Fprintln(&u, "Commands:")
		fmt.Fprintln(&u, "")
		w := tabwriter.NewWriter(&u, 0, 0, 2, ' ', 0)
		for _, cmd := range p.commands {
			fmt.Fprintf(w, "\t%s\t%s\n", cmd.Name(), cmd.Desc())
		}
		w.Flush()
		fmt.Fprintln(&u, "")
		fmt.Fprintf(&u, "Use \"%s help [command]\" for more information about a command.\n", p.name)

		return u.String()
	}

	p.config.args = args

	help, command, exit := p.parseArgs(args)
	if exit {
		return fmt.Errorf(p.usage())
	}
	p.calledCmd = command
	p.printCmdHelp = help
	return nil
}

func (p *Program) Run(fn func(Environment, Command, []string) error) error {
	_, stderr := p.config.GetLoggers()

	for _, cmd := range p.commands {
		if cmd.Name() == p.calledCmd {
			fs := flag.NewFlagSet(p.calledCmd, flag.ContinueOnError)
			fs.SetOutput(p.config.stderr)
			cmd.Register(fs)

			setCommandUsage(stderr, fs, p.name, cmd)
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

		setCommandUsage(stderr, fs, p.name, p.root)
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

func setCommandUsage(l *log.Logger, fs *flag.FlagSet, programName string, cmd Command) {
	var (
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

	fs.Usage = func() {
		l.Printf("Usage: %s %s %s\n", programName, cmd.Name(), cmd.Args())
		l.Println("")
		l.Println(strings.TrimSpace(cmd.Help()))
		l.Println("")
		if flags {
			l.Println("Flags:")
			l.Println("")
			l.Println(fb.String())
		}
	}
}

func (p *Program) parseArgs(args []string) (usage bool, cmd string, exit bool) {
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
		cmd = "default"
	case 2:
		if isHelp(args[1]) {
			exit = true
		} else if isCommand(args[1]) {
			cmd = args[1]
		} else {
			if p.root != nil {
				cmd = "default"
			} else {
				cmd = args[1]
			}
		}
	default:
		if isHelp(args[1]) {
			cmd = args[2]
			usage = true
		} else {
			cmd = args[1]
		}
	}

	return usage, cmd, exit
}
