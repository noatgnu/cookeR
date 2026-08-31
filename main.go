package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/noatgnu/cookeR/internal/appdir"
	"github.com/noatgnu/cookeR/rversion"
)

// version is set via -ldflags "-X main.version=..." at release build time; it stays "dev" for local builds.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println("cooker " + version)
		return
	case "r":
		err = cmdR(os.Args[2:])
	case "env":
		err = cmdEnv(os.Args[2:])
	case "lib":
		err = cmdLib(os.Args[2:])
	case "doctor":
		err = cmdDoctor()
	default:
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: cooker <command> [subcommand] [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  r install <version>       Install an R version")
	fmt.Println("  r list [--available]      List installed (or available) R versions")
	fmt.Println("  r uninstall <version>     Remove an installed R version")
	fmt.Println("  r path <version>          Print the Rscript path for an installed version")
	fmt.Println("  env create <path>         Create an isolated R library environment (not yet implemented)")
	fmt.Println("  lib install <pkg>...      Install R packages into an environment (not yet implemented)")
	fmt.Println("  lib list                  List packages in an environment (not yet implemented)")
	fmt.Println("  doctor                    Report cookeR's environment status")
	fmt.Println("  version                   Print the cooker version")
}

func resolveInstallDir() (string, error) {
	if dir := os.Getenv("COOKER_INSTALL_DIR"); dir != "" {
		return dir, nil
	}
	dir, err := appdir.Default()
	if err != nil {
		return "", fmt.Errorf("failed to resolve default install directory: %w", err)
	}
	return dir, nil
}

func newManager() (*rversion.Manager, error) {
	dir, err := resolveInstallDir()
	if err != nil {
		return nil, err
	}
	return rversion.New(dir), nil
}

func cmdR(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cooker r install <version> | cooker r list [--available] [--json] | cooker r uninstall <version> | cooker r path <version>")
	}

	switch args[0] {
	case "install":
		return cmdRInstall(args[1:])
	case "list":
		return cmdRList(args[1:])
	case "uninstall":
		return cmdRUninstall(args[1:])
	case "path":
		return cmdRPath(args[1:])
	default:
		return fmt.Errorf("unknown r subcommand: %s", args[0])
	}
}

func cmdRInstall(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: cooker r install <version>")
	}
	version := args[0]

	m, err := newManager()
	if err != nil {
		return err
	}

	releases, err := m.ListAvailableRVersions()
	if err != nil {
		return fmt.Errorf("failed to list available R versions: %w", err)
	}

	var target *rversion.Release
	for i := range releases {
		if releases[i].Version == version {
			target = &releases[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("R version %q is not available for this platform yet", version)
	}

	return m.InstallRVersion(*target, func(msg string) {
		fmt.Println(msg)
	})
}

func cmdRList(args []string) error {
	fs := flag.NewFlagSet("r list", flag.ContinueOnError)
	available := fs.Bool("available", false, "list versions available to install instead of installed versions")
	asJSON := fs.Bool("json", false, "output as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	m, err := newManager()
	if err != nil {
		return err
	}

	if *available {
		releases, err := m.ListAvailableRVersions()
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(releases)
		}
		if len(releases) == 0 {
			fmt.Println("No R versions available for this platform yet.")
			return nil
		}
		for _, r := range releases {
			fmt.Println(r.Version)
		}
		return nil
	}

	versions, err := m.ListInstalledRVersions()
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(versions)
	}
	if len(versions) == 0 {
		fmt.Println("No R versions installed.")
		return nil
	}
	for _, v := range versions {
		fmt.Println(v)
	}
	return nil
}

func cmdRUninstall(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: cooker r uninstall <version>")
	}
	m, err := newManager()
	if err != nil {
		return err
	}
	return m.UninstallRVersion(args[0])
}

func cmdRPath(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: cooker r path <version>")
	}
	m, err := newManager()
	if err != nil {
		return err
	}
	path, err := m.GetRPath(args[0])
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func cmdEnv(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cooker env create <path> --r <version>")
	}
	switch args[0] {
	case "create":
		return fmt.Errorf("cooker env create is not yet implemented")
	default:
		return fmt.Errorf("unknown env subcommand: %s", args[0])
	}
}

func cmdLib(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cooker lib install <pkg>... | cooker lib list")
	}
	switch args[0] {
	case "install":
		return fmt.Errorf("cooker lib install is not yet implemented")
	case "list":
		return fmt.Errorf("cooker lib list is not yet implemented")
	default:
		return fmt.Errorf("unknown lib subcommand: %s", args[0])
	}
}

func cmdDoctor() error {
	fmt.Println("cookeR Doctor")
	fmt.Println("=============")

	dir, err := resolveInstallDir()
	if err != nil {
		fmt.Printf("[FAIL] Install directory: %v\n", err)
		return err
	}
	fmt.Printf("[OK]   Install directory: %s\n", dir)

	m := rversion.New(dir)
	installed, err := m.ListInstalledRVersions()
	if err != nil {
		fmt.Printf("[FAIL] Installed R versions: %v\n", err)
	} else {
		fmt.Printf("[OK]   Installed R versions: %d\n", len(installed))
	}

	if _, err := m.ListAvailableRVersions(); err != nil {
		fmt.Printf("[FAIL] r-portable releases reachable: %v\n", err)
	} else {
		fmt.Println("[OK]   r-portable releases reachable")
	}

	return nil
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
