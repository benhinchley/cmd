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
		cmd.Err.Fatal(err)
	}
	if err := p.Run(os.Args, func(env *cmd.Environment, c cmd.Command, args []string) error {
		if err := c.Run(env.GetDefaultContext(), args); err != nil {
			return fmt.Errorf("%s: %v", c.Name(), err)
		}
		return nil
	}); err != nil {
		cmd.Err.Fatal(err)
	}
}

type greetCommand struct {
	pirate bool
}

var _ cmd.Command = (*greetCommand)(nil)

const greetHelp = "a friendly greeting in the terminal."

func (c *greetCommand) Name() string { return "greet" }
func (c *greetCommand) Args() string { return "[name]" }
func (c *greetCommand) Desc() string { return "says hello" }
func (c *greetCommand) Help() string { return strings.TrimSpace(greetHelp) }
func (c *greetCommand) Register(fs *flag.FlagSet) {
	fs.BoolVar(&c.pirate, "pirate", false, "Say hello like a pirate")
	fs.BoolVar(&c.pirate, "p", false, "Say hello like a pirate")
}

func (c *greetCommand) Run(ctx cmd.Context, args []string) error {
	greeting := "Hello, %s!"

	if c.pirate {
		greeting = "Ahoy, %s!"
	}

	switch len(args) {
	case 0:
		cmd.Out.Printf(greeting, "there")
	default:
		cmd.Out.Printf(greeting, args[0])
	}

	return nil
}
