package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/term"

	"github.com/oowliq/acksy/internal/broker/memory"
	"github.com/oowliq/acksy/internal/config"
	"github.com/oowliq/acksy/internal/domain"
	"github.com/oowliq/acksy/internal/service"
	"github.com/oowliq/acksy/internal/store"
)

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// withService opens the database (migrating it) and runs fn with a service.
func withService(ctx context.Context, cfg config.Config, fn func(*service.Service) error) error {
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Migrate(ctx); err != nil {
		return err
	}
	b := memory.New()
	defer b.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return fn(service.New(store.New(db), b, log, service.Options{}))
}

// readPassword reads a password from stdin: prompted (twice) on a terminal,
// otherwise the first line.
func readPassword(in *os.File, out io.Writer) (string, error) {
	fd := int(in.Fd()) //nolint:gosec // file descriptors fit in int
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	fmt.Fprint(out, "Password: ")
	p1, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", err
	}
	fmt.Fprint(out, "Repeat password: ")
	p2, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", err
	}
	if string(p1) != string(p2) {
		return "", errors.New("passwords do not match")
	}
	return string(p1), nil
}

func userCmd(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("user: expected create, list, passwd, link-oidc, unlink-oidc or identities")
	}
	sub, rest := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ContinueOnError)
	cfgPath := fs.String("config", os.Getenv("ACKSY_CONFIG"), "YAML config file")
	username := fs.String("username", "", "username")
	admin := fs.Bool("admin", false, "grant the admin role (create)")
	displayName := fs.String("display-name", "", "display name (create)")
	noPassword := fs.Bool("no-password", false, "create without a password, for SSO-only users (create)")
	issuer := fs.String("issuer", "", "OIDC issuer (link-oidc, unlink-oidc; default: configured issuer)")
	subject := fs.String("subject", "", "OIDC subject, the sub claim (link-oidc, unlink-oidc)")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	return withService(ctx, cfg, func(svc *service.Service) error {
		sys := service.SystemPrincipal()
		switch sub {
		case "create":
			if *username == "" {
				return errors.New("user create: --username is required")
			}
			var password string
			if !*noPassword {
				if password, err = readPassword(os.Stdin, out); err != nil {
					return err
				}
			}
			role := domain.RoleUser
			if *admin {
				role = domain.RoleAdmin
			}
			u, err := svc.CreateUser(ctx, sys, service.CreateUserInput{
				Username: *username, Password: password, DisplayName: *displayName, Role: role,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "created %s %s (%s)\n", u.Role, u.Username, u.ID)
			return nil

		case "list":
			users, err := svc.ListUsers(ctx, sys)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "USERNAME\tROLE\tSTATUS\tID")
			for _, u := range users {
				status := "active"
				if u.Disabled() {
					status = "disabled"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", u.Username, u.Role, status, u.ID)
			}
			return tw.Flush()

		case "passwd":
			if *username == "" {
				return errors.New("user passwd: --username is required")
			}
			u, err := svc.GetUserByUsername(ctx, sys, *username)
			if err != nil {
				return err
			}
			password, err := readPassword(os.Stdin, out)
			if err != nil {
				return err
			}
			if _, err := svc.UpdateUser(ctx, sys, u.ID, service.UpdateUserInput{Password: &password}); err != nil {
				return err
			}
			fmt.Fprintf(out, "password updated for %s; existing sessions were ended\n", u.Username)
			return nil

		case "link-oidc", "unlink-oidc", "identities":
			if *username == "" {
				return fmt.Errorf("user %s: --username is required", sub)
			}
			u, err := svc.GetUserByUsername(ctx, sys, *username)
			if err != nil {
				return err
			}
			return identityCmd(ctx, svc, sub, u, *issuer, *subject, out)

		default:
			return fmt.Errorf("user: unknown subcommand %q", sub)
		}
	})
}

func identityCmd(ctx context.Context, svc *service.Service, sub string, u *domain.User, issuer, subject string, out io.Writer) error {
	sys := service.SystemPrincipal()
	switch sub {
	case "link-oidc":
		if subject == "" {
			return errors.New("user link-oidc: --subject is required")
		}
		ident, err := svc.LinkIdentity(ctx, sys, u.ID, issuer, subject)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "linked %s to %s sub=%s\n", u.Username, ident.Issuer, ident.Subject)
		return nil
	case "unlink-oidc":
		if subject == "" {
			return errors.New("user unlink-oidc: --subject is required")
		}
		if err := svc.UnlinkIdentity(ctx, sys, u.ID, issuer, subject); err != nil {
			return err
		}
		fmt.Fprintf(out, "unlinked sub=%s from %s\n", subject, u.Username)
		return nil
	default: // identities
		ids, err := svc.ListIdentities(ctx, sys, u.ID)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ISSUER\tSUBJECT\tLAST LOGIN")
		for _, i := range ids {
			last := "never"
			if i.LastLoginAt != nil {
				last = i.LastLoginAt.Format(time.RFC3339)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\n", i.Issuer, i.Subject, last)
		}
		return tw.Flush()
	}
}

func tokenCmd(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "create" {
		return errors.New("token: expected create")
	}
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	cfgPath := fs.String("config", os.Getenv("ACKSY_CONFIG"), "YAML config file")
	username := fs.String("username", "", "token owner")
	name := fs.String("name", "", "token name")
	scopes := fs.String("scopes", string(domain.ScopePublish), "comma-separated scopes")
	expires := fs.Duration("expires-in", 0, "token lifetime, e.g. 720h (0 = never)")
	var topics stringList
	fs.Var(&topics, "topic", "restrict to topic (repeatable)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *username == "" || *name == "" {
		return errors.New("token create: --username and --name are required")
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	return withService(ctx, cfg, func(svc *service.Service) error {
		u, err := svc.GetUserByUsername(ctx, service.SystemPrincipal(), *username)
		if err != nil {
			return err
		}
		in := service.CreateTokenInput{Name: *name, Scopes: strings.Split(*scopes, ","), Topics: topics}
		if *expires > 0 {
			at := time.Now().Add(*expires)
			in.ExpiresAt = &at
		}
		ct, err := svc.CreateToken(ctx, service.UserPrincipal(u), in)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, ct.Secret)
		return nil
	})
}

// oidcCmd recovers sign-in when single sign-on is the only way in and the
// provider is gone. A running server picks the change up within seconds.
func oidcCmd(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "allow-password" {
		return errors.New("oidc: expected allow-password")
	}
	fs := flag.NewFlagSet("oidc allow-password", flag.ContinueOnError)
	cfgPath := fs.String("config", os.Getenv("ACKSY_CONFIG"), "YAML config file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	return withService(ctx, cfg, func(svc *service.Service) error {
		if err := svc.AllowPasswordLogin(ctx, service.SystemPrincipal()); err != nil {
			return err
		}
		fmt.Fprintln(out, "password sign-in is allowed again")
		return nil
	})
}
