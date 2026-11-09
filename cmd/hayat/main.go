// Command hayat runs Hayat.
//
//	hayat serve [--role=auth,admin,api,worker]   run one or more roles (default: all)
//	hayat migrate [--status]                 apply or list migrations
//	hayat setup-link                         print a fresh first-admin link
//	hayat rotate-keys                        rotate the signing keys now
//	hayat seed                               dev data and sign-in accounts (HAYAT_DEV only)
//	hayat version
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/aidapedia/hayat/internal/adminserver"
	"github.com/aidapedia/hayat/internal/app"
	"github.com/aidapedia/hayat/internal/audit"
	"github.com/aidapedia/hayat/internal/authserver"
	"github.com/aidapedia/hayat/internal/config"
	"github.com/aidapedia/hayat/internal/migrate"
	"github.com/aidapedia/hayat/internal/store"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1], os.Args[2:], log); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: hayat <serve|migrate|setup-link|rotate-keys|seed|version> [flags]")
}

func run(ctx context.Context, cmd string, args []string, log *slog.Logger) error {
	if cmd == "version" {
		fmt.Println(adminserver.Version)
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s, err := store.Open(ctx, cfg.DatabaseURL, cfg.Region)
	if err != nil {
		return err
	}
	defer s.Close()

	switch cmd {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		roles := fs.String("role", strings.Join(app.AllRoles, ","), "comma-separated roles: auth, admin, api, worker")
		_ = fs.Parse(args)
		a, err := app.New(cfg, s, log)
		if err != nil {
			return err
		}
		return a.Serve(ctx, splitRoles(*roles))

	case "migrate":
		fs := flag.NewFlagSet("migrate", flag.ExitOnError)
		status := fs.Bool("status", false, "list migrations instead of applying them")
		_ = fs.Parse(args)
		if *status {
			lines, err := migrate.Status(ctx, s, cfg.Engine)
			if err != nil {
				return err
			}
			fmt.Println(strings.Join(lines, "\n"))
			return nil
		}
		return migrate.Run(ctx, s, cfg.Engine, log)

	case "setup-link":
		a, err := app.New(cfg, s, log)
		if err != nil {
			return err
		}
		link, err := authserver.SetupLink(ctx, s, a.IDs, cfg.Issuer, true)
		if err != nil {
			return err
		}
		if link == "" {
			fmt.Println("This instance already has an admin; setup is closed.")
			return nil
		}
		fmt.Println(link)
		return nil

	case "seed":
		a, err := app.New(cfg, s, log)
		if err != nil {
			return err
		}
		created, err := a.Seed(ctx)
		if err != nil {
			return err
		}
		if created {
			fmt.Println("Seeded dev data. Sign in at " + cfg.Issuer + "/login or the console with:")
		} else {
			fmt.Println("Dev data already seeded. Sign in with:")
		}
		for _, acc := range app.SeedAccounts {
			fmt.Printf("  %-18s %s   %s\n", acc.Email, app.SeedPassword, acc.About)
		}
		return nil

	case "rotate-keys":
		a, err := app.New(cfg, s, log)
		if err != nil {
			return err
		}
		if err := a.Keys.Rotate(ctx, audit.System); err != nil {
			return err
		}
		fmt.Println("Signing keys rotated. The previous key stays published for 48 hours.")
		return nil
	}
	usage()
	return fmt.Errorf("unknown command %q", cmd)
}

func splitRoles(raw string) []string {
	var out []string
	for _, r := range strings.Split(raw, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}
