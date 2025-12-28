package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ghpm/internal/config"
	"ghpm/internal/ghpm"
	"ghpm/internal/state"
	"ghpm/internal/ui"
)

func main() {
	var (
		root        string
		packagesDir string
		stateDir    string
		cacheDir    string
		jsonOut     bool
		silent      bool
		verbose     bool
		configPath  string
	)

	rootCmd := &cobra.Command{
		Use:   "ghpm",
		Short: "GitHub/GitLab package manager",
	}
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true

	rootCmd.PersistentFlags().StringVar(&root, "root", "/", "install root")
	rootCmd.PersistentFlags().StringVar(&packagesDir, "packages-dir", "", "packages directory")
	rootCmd.PersistentFlags().StringVar(&stateDir, "state-dir", "", "state directory")
	rootCmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "", "cache directory")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "json output")
	rootCmd.PersistentFlags().BoolVar(&silent, "silent", false, "suppress progress output")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "detailed progress output")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "/etc/ghpm/config.yaml", "config path")

	buildManager := func() (*ghpm.Manager, config.Config, error) {
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			return nil, cfg, err
		}
		if packagesDir != "" {
			cfg.PackagesDir = packagesDir
		}
		if stateDir != "" {
			cfg.StateDir = stateDir
		}
		if cacheDir != "" {
			cfg.CacheDir = cacheDir
		}
		manager := ghpm.NewManager(cfg, root)
		if silent {
			manager.Logger = ui.NewLogger(ui.LevelSilent, os.Stderr)
		} else if verbose {
			manager.Logger = ui.NewLogger(ui.LevelVerbose, os.Stderr)
		}
		return manager, cfg, nil
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List known packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := buildManager()
			if err != nil {
				return err
			}
			manifests, err := manager.ListManifests()
			if err != nil {
				return err
			}
			installed, err := state.LoadInstalled(state.InstalledPath(manager.StateDir()))
			if err != nil {
				return err
			}
			if jsonOut {
				type entry struct {
					Name        string `json:"name"`
					Description string `json:"description,omitempty"`
					Installed   string `json:"installed,omitempty"`
				}
				var list []entry
				for _, mf := range manifests {
					e := entry{Name: mf.Name, Description: mf.Description}
					if inst, ok := installed.Installed[mf.Name]; ok {
						e.Installed = inst.Version
					}
					list = append(list, e)
				}
				writeJSON(list)
				return nil
			}
			for _, mf := range manifests {
				version := ""
				if inst, ok := installed.Installed[mf.Name]; ok {
					version = inst.Version
				}
				if version == "" {
					fmt.Printf("%s\n", mf.Name)
				} else {
					fmt.Printf("%s\t%s\n", mf.Name, version)
				}
			}
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status <name>",
		Short: "Show install status for a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := buildManager()
			if err != nil {
				return err
			}
			receipt, status, err := manager.Status(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				writeJSON(map[string]any{"receipt": receipt, "status": status})
				return nil
			}
			fmt.Printf("name: %s\nversion: %s\n", receipt.Name, receipt.Source.Tag)
			for _, f := range receipt.Files {
				ok := status[f.Path]
				state := "ok"
				if !ok {
					state = "mismatch"
				}
				fmt.Printf("%s\t%s\n", state, f.Path)
			}
			return nil
		},
	}

	var installVersion string
	var installAll bool
	var installForce bool
	installCmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a package",
		Args: func(cmd *cobra.Command, args []string) error {
			if installAll {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := buildManager()
			if err != nil {
				return err
			}
			if installAll {
				mfs, err := manager.ListManifests()
				if err != nil {
					return err
				}
				for _, mf := range mfs {
					if _, err := manager.Install(mf.Name, ghpm.InstallOptions{Version: installVersion, Force: installForce}); err != nil {
						return err
					}
				}
				return nil
			}
			receipt, err := manager.Install(args[0], ghpm.InstallOptions{Version: installVersion, Force: installForce})
			if err != nil {
				return err
			}
			if jsonOut {
				writeJSON(receipt)
				return nil
			}
			fmt.Printf("installed %s %s\n", receipt.Name, receipt.Source.Tag)
			return nil
		},
	}
	installCmd.Flags().StringVar(&installVersion, "version", "", "version/tag")
	installCmd.Flags().BoolVar(&installAll, "all", false, "install all")
	installCmd.Flags().BoolVar(&installForce, "force", false, "overwrite conflicts")

	var removePurge bool
	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := buildManager()
			if err != nil {
				return err
			}
			receipt, _, _ := manager.Status(args[0])
			if err := manager.Remove(args[0], ghpm.RemoveOptions{Purge: removePurge}); err != nil {
				return err
			}
			if !jsonOut {
				if receipt.Name != "" && receipt.Source.Tag != "" {
					fmt.Printf("removed %s %s\n", receipt.Name, receipt.Source.Tag)
				} else {
					fmt.Printf("removed %s\n", args[0])
				}
			}
			return nil
		},
	}
	removeCmd.Flags().BoolVar(&removePurge, "purge", false, "remove preserved files")

	var upgradeAll bool
	var upgradeDryRun bool
	upgradeCmd := &cobra.Command{
		Use:   "upgrade <name>",
		Short: "Upgrade a package",
		Args: func(cmd *cobra.Command, args []string) error {
			if upgradeAll {
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, _, err := buildManager()
			if err != nil {
				return err
			}
			if upgradeAll {
				mfs, err := manager.ListManifests()
				if err != nil {
					return err
				}
				for _, mf := range mfs {
					changed, _, err := manager.Upgrade(mf.Name, ghpm.InstallOptions{DryRun: upgradeDryRun})
					if err != nil {
						return err
					}
					if !jsonOut && upgradeDryRun {
						fmt.Printf("%s\t%s\n", mf.Name, yesNo(changed))
					}
				}
				return nil
			}
			changed, receipt, err := manager.Upgrade(args[0], ghpm.InstallOptions{DryRun: upgradeDryRun})
			if err != nil {
				return err
			}
			if jsonOut {
				writeJSON(map[string]any{"changed": changed, "receipt": receipt})
				return nil
			}
			if changed {
				fmt.Printf("upgraded %s to %s\n", receipt.Name, receipt.Source.Tag)
			} else {
				fmt.Printf("%s already up to date\n", receipt.Name)
			}
			return nil
		},
	}
	upgradeCmd.Flags().BoolVar(&upgradeAll, "all", false, "upgrade all")
	upgradeCmd.Flags().BoolVar(&upgradeDryRun, "dry-run", false, "check for upgrades")

	rootCmd.AddCommand(listCmd, statusCmd, installCmd, removeCmd, upgradeCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
