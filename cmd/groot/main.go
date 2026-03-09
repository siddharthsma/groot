package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"groot/internal/integrations/installer"
)

var BuildVersion = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	if args[0] != "integration" {
		return usageError()
	}
	cfg := installer.ConfigFromEnv(BuildVersion)
	service := installer.New(cfg, &http.Client{Timeout: 30 * time.Second})
	ctx := context.Background()

	if len(args) < 2 {
		return usageError()
	}
	switch args[1] {
	case "install":
		if len(args) != 3 {
			return usageError()
		}
		target := strings.TrimSpace(args[2])
		var result installer.InstallResult
		var err error
		if strings.HasSuffix(target, ".grootpkg") || fileExists(target) {
			result, err = service.InstallFile(ctx, target)
		} else {
			result, err = service.InstallName(ctx, target)
		}
		if err != nil {
			return err
		}
		return printJSON(result)
	case "remove":
		if len(args) != 3 {
			return usageError()
		}
		if err := service.Remove(strings.TrimSpace(args[2])); err != nil {
			return err
		}
		return printJSON(map[string]any{"removed": strings.TrimSpace(args[2])})
	case "list":
		if len(args) != 2 {
			return usageError()
		}
		integrations, err := service.List()
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"integrations": integrations})
	case "info":
		if len(args) != 3 {
			return usageError()
		}
		info, err := service.Info(strings.TrimSpace(args[2]))
		if err != nil {
			return err
		}
		return printJSON(info)
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("usage: groot integration <install|remove|list|info> [target]")
}

func printJSON(value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal output: %w", err)
	}
	_, err = os.Stdout.Write(append(body, '\n'))
	return err
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
