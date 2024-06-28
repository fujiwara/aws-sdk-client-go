package sdkclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/jmespath/go-jmespath"
)

var LogLevel = new(slog.LevelVar)

func init() {
	opts := &slog.HandlerOptions{Level: LogLevel}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, opts)))
	if os.Getenv("DEBUG") != "" {
		LogLevel.Set(slog.LevelDebug)
	}
}

var Version = "HEAD"

var clientMethods = map[string]map[string]ClientMethod{} // to be defined in main_gen.go

type ClientMethod func(context.Context, *clientMethodParam) (any, error)

var ErrDryRun = fmt.Errorf("dry-run mode")

type CLI struct {
	Service string   `arg:"" help:"service name" default:""`
	Method  string   `arg:"" help:"method name" default:""`
	Input   string   `arg:"" help:"input JSON/Jsonnet struct or filename" default:"{}"`
	Args    []string `arg:"" optional:"" help:"additional arguments"`

	InputStream  string            `short:"i" help:"bind input filename or '-' to io.Reader field in the input struct"`
	OutputStream string            `short:"o" help:"bind output filename or '-' to io.ReadCloser field in the output struct"`
	APIOutput    bool              `help:"output API response into stdout" default:"true" negatable:"true"`
	RawOutput    bool              `short:"r" help:"output raw strings, not JSON texts"`
	Compact      bool              `short:"c" help:"compact JSON output"`
	Query        string            `short:"q" help:"JMESPath query to apply to output"`
	ExtStr       map[string]string `help:"external variables for Jsonnet"`
	ExtCode      map[string]string `help:"external code for Jsonnet"`
	Strict       bool              `name:"strict" help:"strict input JSON unmarshaling" default:"true" negatable:"true"`
	FollowNext   string            `short:"f" help:"OutputField=InputField format. follow the next token." default:""`
	CamelCase    bool              `name:"camel" help:"convert keys to camelCase"`

	DryRun  bool `short:"n" help:"dry-run mode"`
	Version bool `short:"v" help:"show version"`

	w  io.Writer
	rc *RuntimeConfig
}

func Run(ctx context.Context) error {
	var c CLI
	if rc, err := loadRuntimeConfig(); err != nil {
		return fmt.Errorf("failed to load runtime config: %w", err)
	} else {
		c.rc = rc
	}
	slog.Debug("runtime config", "config", c.rc)

	c.w = os.Stdout
	args, err := c.resolveAliases(os.Args[1:])
	if err != nil {
		return err
	}
	slog.Debug("resolved args", "args", args)

	k, err := kong.New(&c, kong.Vars{"version": Version})
	if err != nil {
		return err
	}
	if _, err := k.Parse(args); err != nil {
		return err
	}
	slog.Debug("extra args", "args", c.Args)

	return c.Dispatch(ctx)
}

func (c *CLI) Dispatch(ctx context.Context) error {
	if c.Version {
		fmt.Fprintf(c.w, "awslim %s\n", Version)
		return nil
	}
	if c.Service == "" {
		return c.ListServices(ctx)
	} else if c.Method == "" {
		return c.ListMethods(ctx)
	} else {
		return c.CallMethod(ctx)
	}
}

func (c *CLI) SetWriter(w io.Writer) {
	c.w = w
}

func (c *CLI) CallMethod(ctx context.Context) error {
	method := kebabToPascal(c.Method)
	key := buildKey(c.Service, method)
	ms, ok := clientMethods[c.Service]
	if !ok {
		return fmt.Errorf("unknown service %s", c.Service)
	}
	fn, ok := ms[method]
	if !ok {
		return fmt.Errorf("unknown function %s", key)
	}
	if c.Input == "help" {
		c.showHelp(key)
		return nil
	}

	p, err := c.clientMethodParam(ctx)
	if err != nil {
		return err
	}
	defer p.Cleanup()

	cont := true
	for cont {
		out, err := fn(ctx, p)
		if err != nil {
			if err == ErrDryRun {
				fmt.Fprintf(c.w, "dry-run: %s will be called with:\n%s", key, string(p.InputBytes))
				return nil
			} else if strings.Contains(err.Error(), "json: unknown field") {
				c.showHelp(key)
				return err
			}
			return fmt.Errorf("failed to call %s: %w", key, err)
		}
		if err := c.output(ctx, out); err != nil {
			return err
		}
		cont, err = p.FollowNext(out)
		if err != nil {
			return fmt.Errorf("failed to follow next token: %w", err)
		}
	}
	return nil
}

func (c *CLI) showHelp(key string) {
	u := fmt.Sprintf("https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/%s", key)
	fmt.Fprintf(c.w, "See %s\n", u)
	if c.rc != nil && c.rc.Open != "" {
		slog.Info("run open command", "url", u)
		if err := exec.Command(c.rc.Open, u).Run(); err != nil {
			slog.Error("failed to open command", "error", err)
		}
	}
}

func (c *CLI) output(_ context.Context, out any) error {
	if !c.APIOutput {
		return nil
	}
	var err error
	if c.Query != "" {
		out, err = jmespath.Search(c.Query, out)
		if err != nil {
			return fmt.Errorf("failed to apply JMESPath query: %w", err)
		}
	}

	if c.RawOutput {
		switch t := out.(type) {
		case string:
			fmt.Fprintln(c.w, t)
			return nil
		case *string:
			fmt.Fprintln(c.w, aws.ToString(t))
			return nil
		default:
			// do nothing. output as JSON
		}
	}

	b, err := marshalJSON(out, c.CamelCase)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	if !c.Compact {
		var buf bytes.Buffer
		json.Indent(&buf, b, "", "  ")
		buf.WriteString("\n")
		buf.WriteTo(c.w)
	} else {
		io.WriteString(c.w, string(b))
	}
	return nil
}

func (c *CLI) loadInput(_ context.Context) ([]byte, error) {
	var input []byte
	vm := jsonnet.MakeVM()
	for k, v := range c.ExtStr {
		vm.ExtVar(k, v)
	}
	for k, v := range c.ExtCode {
		vm.ExtCode(k, v)
	}
	def := c.setNativeFuncs(vm)
	if strings.HasPrefix(c.Input, "{") {
		// string is JSON or Jsonnet
		in := def + c.Input
		slog.Debug("evaluate Jsonnet", "input", in)
		s, err := vm.EvaluateAnonymousSnippet("input", in)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate Jsonnet: %w", err)
		}
		input = []byte(s)
	} else {
		// string is filename
		s, err := vm.EvaluateFile(c.Input)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate Jsonnet: %w", err)
		}
		input = []byte(s)
	}
	return input, nil
}

func (c *CLI) clientMethodParam(ctx context.Context) (*clientMethodParam, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	input, err := c.loadInput(ctx)
	if err != nil {
		return nil, err
	}

	p := &clientMethodParam{
		awsCfg:       awsCfg,
		InputBytes:   input,
		InputReader:  nil,
		OutputWriter: nil,
		DryRun:       c.DryRun,
		Strict:       c.Strict,
	}
	p.SetNextToken(c.FollowNext)

	switch c.InputStream {
	case "":
		// do nothing
	case "-": // stdin
		buf := &bytes.Buffer{}
		if _, err := io.Copy(buf, os.Stdin); err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("failed to read from stdin: %w", err)
			}
		}
		p.InputReader = buf
		p.InputReaderLength = aws.Int64(int64(buf.Len()))
	default:
		f, err := os.Open(c.InputStream)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file: %w", err)
		}
		p.InputReader = f
		p.cleanup = append(p.cleanup, f.Close)
		st, err := f.Stat()
		if err != nil {
			return nil, fmt.Errorf("failed to stat input file: %w", err)
		}
		p.InputReaderLength = aws.Int64(st.Size())
	}

	switch c.OutputStream {
	case "":
		// do nothing
	case "-": // stdout
		p.OutputWriter = os.Stdout
	default:
		f, err := os.Create(c.OutputStream)
		if err != nil {
			return nil, fmt.Errorf("failed to create output file: %w", err)
		}
		p.OutputWriter = f
		p.cleanup = append(p.cleanup, f.Close)
	}

	return p, nil
}

func (c *CLI) ListMethods(_ context.Context) error {
	methods := make([]string, 0)
	if m, ok := clientMethods[c.Service]; !ok {
		return fmt.Errorf("unknown service %s", c.Service)
	} else {
		for method := range m {
			methods = append(methods, method)
		}
	}
	sort.Strings(methods)
	for _, name := range methods {
		fmt.Fprintln(c.w, name)
	}
	return nil
}

func (c *CLI) ListServices(_ context.Context) error {
	var names []string
	for name := range clientMethods {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintln(c.w, name)
	}
	return nil
}

func (c *CLI) setNativeFuncs(vm *jsonnet.VM) string {
	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "args",
		Params: []ast.Identifier{"n"},
		Func: func(args []interface{}) (interface{}, error) {
			n := args[0].(float64)
			return c.Args[int(n)], nil
		},
	})
	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "env",
		Params: []ast.Identifier{"name", "default"},
		Func: func(args []interface{}) (interface{}, error) {
			name := args[0].(string)
			def := args[1].(string)
			if v, ok := os.LookupEnv(name); ok {
				return v, nil
			}
			return def, nil
		},
	})
	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "must_env",
		Params: []ast.Identifier{"name"},
		Func: func(args []interface{}) (interface{}, error) {
			name := args[0].(string)
			if v, ok := os.LookupEnv(name); ok {
				return v, nil
			}
			return nil, fmt.Errorf("environment variable %s is not set", name)
		},
	})
	return `
	local _ = std.native('args');
	local env = std.native('env');
	local must_env = std.native('must_env');
	`
}

func buildKey(service, method string) string {
	return fmt.Sprintf("%s#Client.%s", service, method)
}

func kebabToPascal(kebab string) string {
	parts := strings.Split(kebab, "-")
	results := make([]string, 0, len(parts))
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		results = append(results, strings.ToUpper(p[:1])+p[1:])
	}
	return strings.Join(results, "")
}

//go:generate go run cmd/awslim-gen/main.go cmd/awslim-gen/gen.go
