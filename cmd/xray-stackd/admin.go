// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/amirrezakm/zeroone/internal/auth"
	"github.com/amirrezakm/zeroone/internal/stack"
)

// runAdminSubcommand handles `xray-stackd admin <verb>` invocations and
// returns true if it consumed the args. The verbs are kept narrow on
// purpose: they exist so the installer (and operators) can manage panel
// admins without poking at JSON or hitting the bootstrap-mode API.
func runAdminSubcommand(args []string) (handled bool, exitCode int) {
	if len(args) < 1 || args[0] != "admin" {
		return false, 0
	}
	if len(args) < 2 {
		adminUsage()
		return true, 2
	}
	verb := args[1]
	switch verb {
	case "add":
		return true, adminAdd(args[2:])
	case "reset-password":
		return true, adminResetPassword(args[2:])
	case "list":
		return true, adminList(args[2:])
	case "-h", "--help", "help":
		adminUsage()
		return true, 0
	default:
		fmt.Fprintf(os.Stderr, "unknown admin verb: %s\n", verb)
		adminUsage()
		return true, 2
	}
}

func adminUsage() {
	fmt.Fprint(os.Stderr, `usage: xray-stackd admin <verb> [flags]

verbs:
  add             -config PATH -username U -password P
  reset-password  -config PATH -username U -password P
  list            -config PATH
`)
}

func adminAdd(args []string) int {
	fs := flag.NewFlagSet("admin add", flag.ExitOnError)
	configPath := fs.String("config", "", "stack config path")
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "admin password")
	_ = fs.Parse(args)
	if *configPath == "" || *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "admin add: -config, -username, -password are required")
		return 2
	}
	cfg, err := stack.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	user := strings.TrimSpace(*username)
	if user == "" {
		fmt.Fprintln(os.Stderr, "username cannot be empty")
		return 2
	}
	for _, a := range cfg.Panel.Admins {
		if strings.EqualFold(a.Username, user) {
			fmt.Fprintf(os.Stderr, "admin %q already exists; use reset-password instead\n", user)
			return 1
		}
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		return 1
	}
	cfg.Panel.Admins = append(cfg.Panel.Admins, stack.Admin{
		Username:     user,
		PasswordHash: hash,
		CreatedAt:    time.Now().Unix(),
	})
	if err := stack.Save(*configPath, *cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	fmt.Printf("admin %q added\n", user)
	return 0
}

func adminResetPassword(args []string) int {
	fs := flag.NewFlagSet("admin reset-password", flag.ExitOnError)
	configPath := fs.String("config", "", "stack config path")
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "new password")
	_ = fs.Parse(args)
	if *configPath == "" || *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "admin reset-password: -config, -username, -password are required")
		return 2
	}
	cfg, err := stack.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	idx := -1
	for i, a := range cfg.Panel.Admins {
		if strings.EqualFold(a.Username, *username) {
			idx = i
			break
		}
	}
	if idx < 0 {
		fmt.Fprintf(os.Stderr, "admin %q not found\n", *username)
		return 1
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		return 1
	}
	cfg.Panel.Admins[idx].PasswordHash = hash
	if err := stack.Save(*configPath, *cfg); err != nil {
		fmt.Fprintf(os.Stderr, "save config: %v\n", err)
		return 1
	}
	fmt.Printf("admin %q password reset\n", *username)
	return 0
}

func adminList(args []string) int {
	fs := flag.NewFlagSet("admin list", flag.ExitOnError)
	configPath := fs.String("config", "", "stack config path")
	_ = fs.Parse(args)
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "admin list: -config is required")
		return 2
	}
	cfg, err := stack.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	if len(cfg.Panel.Admins) == 0 {
		fmt.Println("(no admins configured)")
		return 0
	}
	for _, a := range cfg.Panel.Admins {
		last := "never"
		if a.LastLogin > 0 {
			last = time.Unix(a.LastLogin, 0).UTC().Format(time.RFC3339)
		}
		fmt.Printf("%s\tcreated=%s\tlast_login=%s\n",
			a.Username,
			time.Unix(a.CreatedAt, 0).UTC().Format(time.RFC3339),
			last)
	}
	return 0
}
