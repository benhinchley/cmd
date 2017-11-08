package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/benhinchley/cmd"
)

func main() {
	p, err := cmd.NewProgram("greet", "", &greetCommand{}, nil)
	if err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	if err := p.ParseArgs(os.Args); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
	if err := p.Run(func(env *cmd.Environment, c cmd.Command, args []string) error {
		if err := c.Run(env.GetDefaultContext(), args); err != nil {
			return fmt.Errorf("%s: %v", c.Name(), err)
		}
		return nil
	}); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}

type greetCommand struct {
	priate bool
}

const greetHelp = `
a friendly greeting in the terminal.
`

func (cmd *greetCommand) Name() string { return "greet" }
func (cmd *greetCommand) Args() string { return "[name]" }
func (cmd *greetCommand) Desc() string { return "says hello" }
func (cmd *greetCommand) Help() string { return strings.TrimSpace(greetHelp) }
func (cmd *greetCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&cmd.priate, "pirate", false, "Say hello like a pirate")
}

func (cmd *greetCommand) Run(ctx cmd.Context, args []string) error {
	greeting := "Hello, %s!"

	if cmd.priate {
		greeting = "Ahoy, %s!"
	}

	switch len(args) {
	case 0:
		ctx.Stdout().Printf(greeting, "there")
	default:
		ctx.Stdout().Printf(greeting, args[0])
	}

	return nil
}
