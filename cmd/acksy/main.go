// Command acksy is the Acksy notification server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/oowliq/acksy/internal/app"
	"github.com/oowliq/acksy/internal/buildinfo"
	"github.com/oowliq/acksy/internal/config"
	"github.com/oowliq/acksy/internal/store"
)

const usage = `Usage: acksy <command> [flags]

Commands:
  serve            Run the server (applies pending migrations first)
  migrate up       Apply pending database migrations
  migrate status   Show migration status
  user create      Create a user (--username, --admin; password from stdin, or --no-password)
  user list        List users
  user passwd      Reset a user's password (--username)
  user link-oidc   Link an OIDC identity to a user (--username, --subject, [--issuer])
  user unlink-oidc Remove a linked OIDC identity (--username, --subject, [--issuer])
  user identities  List a user's linked OIDC identities (--username)
  token create     Create an API token (--username, --name, --scopes, --topic)
  oidc allow-password  Turn password sign-in back on (when the SSO provider is unreachable)
  version          Print version information

Flags:
  --config <path>  YAML config file (ACKSY_* environment variables take precedence)
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()

	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return flag.ErrHelp
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "version", "--version", "-v":
		i := buildinfo.Get()
		fmt.Fprintf(out, "acksy %s (commit %s, built %s)\n", i.Version, i.Commit, i.Date)
		return nil
	case "help", "--help", "-h":
		fmt.Fprint(out, usage)
		return nil
	case "serve":
		cfg, err := loadConfig("serve", rest)
		if err != nil {
			return err
		}
		return serve(ctx, cfg)
	case "user":
		return userCmd(ctx, rest, out)
	case "token":
		return tokenCmd(ctx, rest, out)
	case "oidc":
		return oidcCmd(ctx, rest, out)
	case "migrate":
		if len(rest) == 0 {
			return errors.New("migrate: expected 'up' or 'status'")
		}
		cfg, err := loadConfig("migrate "+rest[0], rest[1:])
		if err != nil {
			return err
		}
		return migrate(ctx, cfg, rest[0], out)
	default:
		fmt.Fprint(out, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func loadConfig(name string, args []string) (config.Config, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	path := fs.String("config", os.Getenv("ACKSY_CONFIG"), "YAML config file")
	if err := fs.Parse(args); err != nil {
		return config.Config{}, err
	}
	if fs.NArg() > 0 {
		return config.Config{}, fmt.Errorf("%s: unexpected arguments %v", name, fs.Args())
	}
	return config.Load(*path)
}

func serve(ctx context.Context, cfg config.Config) error {
	log := cfg.Log.NewLogger()
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()
	return a.Run(ctx)
}

func migrate(ctx context.Context, cfg config.Config, sub string, out io.Writer) error {
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	switch sub {
	case "up":
		v, err := db.Migrate(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s schema at version %d\n", db.Dialect, v)
		return nil
	case "status":
		st, err := db.MigrationStatus(ctx)
		if err != nil {
			return err
		}
		for _, s := range st {
			state := "pending"
			if s.Applied {
				state = "applied"
			}
			fmt.Fprintf(out, "%-8s %s\n", state, s.Name)
		}
		return nil
	default:
		return fmt.Errorf("migrate: unknown subcommand %q", sub)
	}
}
