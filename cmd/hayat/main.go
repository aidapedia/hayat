// Command hayat runs Hayat.
//
//	hayat serve [--role=auth,admin,api,worker]   run one or more roles (default: all)
//	hayat migrate [--status]                 apply or list migrations
//	hayat setup-link                         print a fresh first-admin link
//	hayat rotate-keys                        rotate the signing keys now
//	hayat seed                               dev data and sign-in accounts (HAYAT_DEV only)
//	hayat import --file b.json [--app-url u] load another system's people and access (tabletsgo)
//	hayat version
package main

import (
	"context"
	"encoding/json"
	"errors"
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
	fmt.Fprintln(os.Stderr, "usage: hayat <serve|migrate|setup-link|rotate-keys|seed|import|version> [flags]")
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

	case "import":
		fs := flag.NewFlagSet("import", flag.ExitOnError)
		file := fs.String("file", "", "the bundle another system exported (required)")
		appURL := fs.String("app-url", "", "the application's public URL; registers its sign-in client with <url>/callback")
		out := fs.String("out", "", "also write the result (id map, credentials) to this JSON file")
		_ = fs.Parse(args)
		if *file == "" {
			return errors.New("import needs --file")
		}
		raw, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		bundle, err := app.ParseBundle(raw)
		if err != nil {
			return err
		}
		a, err := app.New(cfg, s, log)
		if err != nil {
			return err
		}
		res, importErr := a.Import(ctx, bundle, app.ImportOptions{AppURL: *appURL})
		printImport(cfg, bundle, res)
		if *out != "" {
			body, _ := json.MarshalIndent(res, "", "  ")
			if err := os.WriteFile(*out, body, 0o600); err != nil {
				return err
			}
			fmt.Println("Result written to " + *out)
		}
		return importErr

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

// printImport tells the operator what happened and what to give the service.
func printImport(cfg config.Config, b app.Bundle, r app.ImportResult) {
	fmt.Printf("Imported from %s: %d people (%d already here), %d tenants (%d skipped), %d groups, %d resources, %d grants.\n",
		b.Source, r.UsersCreated, r.UsersMatched, r.TenantsCreated, r.TenantsSkipped, r.Groups, r.Resources, r.Grants)
	if r.ContractVersion > 0 {
		fmt.Printf("Service %q runs contract version %d.\n", b.Service.ID, r.ContractVersion)
	}
	for _, w := range r.Warnings {
		fmt.Println("  warning: " + w)
	}
	if len(r.IDMap) > 0 {
		fmt.Printf("%d people matched existing accounts under other ids. Pass the result file (--out) to %s so it rewrites its references.\n",
			len(r.IDMap), b.Source)
	}
	if r.ServiceRegistered || r.WebClientID != "" {
		fmt.Println("\nConfigure " + b.Source + " with:")
		fmt.Println("  HAYAT_ISSUER=" + cfg.Issuer)
		if r.ServiceRegistered {
			fmt.Println("  HAYAT_CLIENT_ID=" + r.ClientID)
			fmt.Println("  HAYAT_CLIENT_SECRET=" + r.ClientSecret + "   (shown once)")
		}
		if r.WebClientID != "" {
			fmt.Println("  HAYAT_WEB_CLIENT_ID=" + r.WebClientID)
		}
		fmt.Println("  HAYAT_API_URL=<the api role's address, e.g. http://hayat-api:7443>")
	}
}
