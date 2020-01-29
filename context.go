package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"
)

// Context is an interface that is passed through to
// each Handler action in a cli application. It
// can be used to retrieve context-specific args and
// parsed command-line options.
type Context interface {
	// New Methods (WIP)
	WithContext(context context.Context) Context
	WithParent(parent Context) Context
	WithApp(app *App) Context
	WithCommand(command *Command) Context
	WithFlagset(set *flag.FlagSet) Context
	// Functions to be able to manage Context type
	App() *App
	setCommand(command *Command)
	Command() *Command
	Context() context.Context
	ParentContext() Context
	SetShellComplete(shellComplete bool)
	ShellComplete() bool
	SetFlagSet(set *flag.FlagSet)
	FlagSet() *flag.FlagSet
	NumFlags() int
	Set(name, value string) error
	IsSet(name string) bool
	LocalFlagNames() []string
	FlagNames() []string
	Lineage() []Context
	Value(name string) interface{}
	Args() Args
	// Deprecated: Use context.Args().Len() instead
	NArg() int
	// Functions for each flag type supported by the cli
	Timestamp(name string) *time.Time
	Generic(name string) interface{}
	Bool(name string) bool
	Float64(name string) float64
	String(name string) string
	Int64Slice(name string) []int64
	Uint64(name string) uint64
	StringSlice(name string) []string
	Int64(name string) int64
	Float64Slice(name string) []float64
	Duration(name string) time.Duration
	IntSlice(name string) []int
	Int(name string) int
	Path(name string) string
	Uint(name string) uint
}

type cliContext struct {
	context       context.Context
	app           *App
	command       *Command
	shellComplete bool
	flagSet       *flag.FlagSet
	parentContext Context
}

func NewContext() Context {
	return &cliContext{
		context: context.Background(),
		command: &Command{},
	}
}

func (c *cliContext) WithContext(context context.Context) Context {
	c.context = context

	return c
}

func (c *cliContext) WithParent(parent Context) Context {
	c.parentContext = parent

	if parent != nil {
		c.context = parent.Context()
		c.shellComplete = parent.ShellComplete()
		if parent.FlagSet() == nil {
			parent.SetFlagSet(&flag.FlagSet{})
		}
	}

	return c
}

func (c *cliContext) WithApp(app *App) Context {
	c.app = app
	return c
}

func (c *cliContext) WithCommand(command *Command) Context {
	c.command = command
	return c
}

func (c *cliContext) WithFlagset(set *flag.FlagSet) Context {
	c.flagSet = set
	return c
}

// Returns the App associated with the current context
func (c *cliContext) App() *App {
	return c.app
}

// Associates a command with the current context
func (c *cliContext) setCommand(command *Command) {
	c.command = command
}

// Returns the Command associated with the current context
func (c *cliContext) Command() *Command {
	return c.command
}

// Returns the context.Context wrapped inside the current context
func (c *cliContext) Context() context.Context {
	return c.context
}

// Returns the parent of the current context
func (c *cliContext) ParentContext() Context {
	return c.parentContext
}

// Sets the shellComplete boolean for the current context
func (c *cliContext) SetShellComplete(shellComplete bool) {
	c.shellComplete = shellComplete
}

// Returns the value of shellComplete boolean for the current context
func (c *cliContext) ShellComplete() bool {
	return c.shellComplete
}

// Associates a flagset with the current context
func (c *cliContext) SetFlagSet(set *flag.FlagSet) {
	c.flagSet = set
}

// Returns the flagset associated with the current context
func (c *cliContext) FlagSet() *flag.FlagSet {
	return c.flagSet
}

// NumFlags returns the number of flags set
func (c *cliContext) NumFlags() int {
	return c.flagSet.NFlag()
}

// Set sets a context flag to a value.
func (c *cliContext) Set(name, value string) error {
	return c.flagSet.Set(name, value)
}

// IsSet determines if the flag was actually set
func (c *cliContext) IsSet(name string) bool {
	if fs := lookupFlagSet(name, c); fs != nil {
		if fs := lookupFlagSet(name, c); fs != nil {
			isSet := false
			fs.Visit(func(f *flag.Flag) {
				if f.Name == name {
					isSet = true
				}
			})
			if isSet {
				return true
			}
		}

		f := lookupFlag(name, c)
		if f == nil {
			return false
		}

		return f.IsSet()
	}

	return false
}

// LocalFlagNames returns a slice of flag names used in this context.
func (c *cliContext) LocalFlagNames() []string {
	var names []string
	c.flagSet.Visit(makeFlagNameVisitor(&names))
	return names
}

// FlagNames returns a slice of flag names used by the this context and all of
// its parent contexts.
func (c *cliContext) FlagNames() []string {
	var names []string
	for _, ctx := range c.Lineage() {
		ctx.FlagSet().Visit(makeFlagNameVisitor(&names))
	}
	return names
}

// Lineage returns *this* context and all of its ancestor contexts in order from
// child to parent
func (c *cliContext) Lineage() []Context {
	var lineage []Context

	for cur := Context(c); cur != nil; cur = cur.ParentContext() {
		lineage = append(lineage, cur)
	}

	return lineage
}

// Value returns the value of the flag corresponding to `name`
func (c *cliContext) Value(name string) interface{} {
	return c.flagSet.Lookup(name).Value.(flag.Getter).Get()
}

// Args returns the command line arguments associated with the context.
func (c *cliContext) Args() Args {
	ret := args(c.flagSet.Args())
	return &ret
}

// NArg returns the number of the command line arguments.
// Deprecated: Use context.Args().Len() instead
func (c *cliContext) NArg() int {
	return c.Args().Len()
}

func lookupFlag(name string, ctx *cliContext) Flag {
	for _, c := range ctx.Lineage() {
		if c.Command() == nil {
			continue
		}

		for _, f := range c.Command().Flags {
			for _, n := range f.Names() {
				if n == name {
					return f
				}
			}
		}
	}

	if ctx.App() != nil {
		for _, f := range ctx.App().Flags {
			for _, n := range f.Names() {
				if n == name {
					return f
				}
			}
		}
	}

	return nil
}

func lookupFlagSet(name string, ctx Context) *flag.FlagSet {
	for _, c := range ctx.Lineage() {
		if f := c.FlagSet().Lookup(name); f != nil {
			return c.FlagSet()
		}
	}

	return nil
}

func copyFlag(name string, ff *flag.Flag, set *flag.FlagSet) {
	switch ff.Value.(type) {
	case Serializer:
		_ = set.Set(name, ff.Value.(Serializer).Serialize())
	default:
		_ = set.Set(name, ff.Value.String())
	}
}

func normalizeFlags(flags []Flag, set *flag.FlagSet) error {
	visited := make(map[string]bool)
	set.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	for _, f := range flags {
		parts := f.Names()
		if len(parts) == 1 {
			continue
		}
		var ff *flag.Flag
		for _, name := range parts {
			name = strings.Trim(name, " ")
			if visited[name] {
				if ff != nil {
					return errors.New("Cannot use two forms of the same flag: " + name + " " + ff.Name)
				}
				ff = set.Lookup(name)
			}
		}
		if ff == nil {
			continue
		}
		for _, name := range parts {
			name = strings.Trim(name, " ")
			if !visited[name] {
				copyFlag(name, ff, set)
			}
		}
	}
	return nil
}

func makeFlagNameVisitor(names *[]string) func(*flag.Flag) {
	return func(f *flag.Flag) {
		nameParts := strings.Split(f.Name, ",")
		name := strings.TrimSpace(nameParts[0])

		for _, part := range nameParts {
			part = strings.TrimSpace(part)
			if len(part) > len(name) {
				name = part
			}
		}

		if name != "" {
			*names = append(*names, name)
		}
	}
}

type requiredFlagsErr interface {
	error
	getMissingFlags() []string
}

type errRequiredFlags struct {
	missingFlags []string
}

func (e *errRequiredFlags) Error() string {
	numberOfMissingFlags := len(e.missingFlags)
	if numberOfMissingFlags == 1 {
		return fmt.Sprintf("Required flag %q not set", e.missingFlags[0])
	}
	joinedMissingFlags := strings.Join(e.missingFlags, ", ")
	return fmt.Sprintf("Required flags %q not set", joinedMissingFlags)
}

func (e *errRequiredFlags) getMissingFlags() []string {
	return e.missingFlags
}

func checkRequiredFlags(flags []Flag, context Context) requiredFlagsErr {
	var missingFlags []string
	for _, f := range flags {
		if rf, ok := f.(RequiredFlag); ok && rf.IsRequired() {
			var flagPresent bool
			var flagName string

			for _, key := range f.Names() {
				if len(key) > 1 {
					flagName = key
				}

				if context.IsSet(strings.TrimSpace(key)) {
					flagPresent = true
				}
			}

			if !flagPresent && flagName != "" {
				missingFlags = append(missingFlags, flagName)
			}
		}
	}

	if len(missingFlags) != 0 {
		return &errRequiredFlags{missingFlags: missingFlags}
	}

	return nil
}
