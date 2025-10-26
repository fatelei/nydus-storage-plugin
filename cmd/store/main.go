package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"log/slog"
	"path/filepath"

	"github.com/containerd/nydus-snapshotter/cmd/containerd-nydus-grpc/pkg/command"
	"github.com/containerd/nydus-snapshotter/config"
	"github.com/containerd/nydus-snapshotter/pkg/errdefs"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	"github.com/containers/nydus-storage-plugin/pkg/fs"
	"github.com/containers/nydus-storage-plugin/pkg/manager"
	"github.com/containers/nydus-storage-plugin/pkg/services/keychain/dockerconfig"
	podmanauth "github.com/containers/nydus-storage-plugin/pkg/services/keychain/podmanauth"
	"github.com/containers/nydus-storage-plugin/pkg/services/resolver"
 )

func waitForSIGINT() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
}

func parseOctalMode(s string) (uint32, error) {
	var v uint32
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '7' { return 0, fmt.Errorf("invalid octal: %s", s) }
		v = (v << 3) | uint32(c-'0')
	}
	return v, nil
}

func setupSlog(level string, toStdout bool, logDir string, root string) error {
	var lvl slog.Level
	switch level {
	case "debug": lvl = slog.LevelDebug
	case "warn", "warning": lvl = slog.LevelWarn
	case "error": lvl = slog.LevelError
	default: lvl = slog.LevelInfo
	}
	var w *os.File
	if toStdout || logDir == "" {
		w = os.Stdout
	} else {
		path := filepath.Join(logDir, "nydus-store.log")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil { return err }
		w = f
	}
	h := slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
	return nil
}

func main() {
	flags := command.NewFlags()
	app := &cli.App{
		Name:    "crio nydus store",
		Usage:   "crio nydus store plugin",
		Version: "0.0.0",
		Flags:   append(flags.F,
			&cli.StringFlag{Name: "fs-file-mode", Usage: "octal file mode for files (e.g. 0400)"},
			&cli.StringFlag{Name: "fs-dir-mode", Usage: "octal dir mode (e.g. 0500)"},
			&cli.StringFlag{Name: "fs-link-mode", Usage: "octal symlink mode (e.g. 0400)"},
		),
		Action: func(c *cli.Context) error {
			if err := setupSlog(flags.Args.LogLevel, flags.Args.LogToStdout, flags.Args.LogDir, flags.Args.RootDir); err != nil {
				return errors.Wrap(err, "failed to prepare logger")
			}

			var cfg config.Config
			if err := command.Validate(flags.Args, &cfg); err != nil {
				return errors.Wrap(err, "invalid argument")
			}

			mountPoint := fmt.Sprintf("%s/store", flags.Args.RootDir)
			if err := os.MkdirAll(mountPoint, 0755); err != nil {
				return errors.Wrapf(err, "create root directory %s", mountPoint)
			}

			// replace it with nydus-snapshotter resolver.
			hosts := resolver.RegistryHostsFromConfig(
				[]resolver.Credential{
					dockerconfig.NewDockerconfigKeychain(c.Context),
					// Podman-compatible auth.json
					podmanauth.NewPodmanAuthKeychain(c.Context),
				}..., 
			)
			layManager, err := manager.NewLayerManager(c.Context, flags.Args.RootDir, hosts, &cfg)
			if err != nil {
				panic(err)
			}

			// Parse optional FS modes
			fileMode := fs.DefaultFileMode()
			dirMode := fs.DefaultDirMode()
			linkMode := fs.DefaultLinkMode()
			if v := c.String("fs-file-mode"); v != "" {
				if m, err := parseOctalMode(v); err == nil { fileMode = m }
			}
			if v := c.String("fs-dir-mode"); v != "" {
				if m, err := parseOctalMode(v); err == nil { dirMode = m }
			}
			if v := c.String("fs-link-mode"); v != "" {
				if m, err := parseOctalMode(v); err == nil { linkMode = m }
			}

			// Recover orphan bind mounts from previous crashes
			_ = layManager.RecoverOrphanMounts(c.Context)

			if err := fs.Mount(c.Context, mountPoint, flags.Args.RootDir, true, layManager, fs.WithModes(fileMode, dirMode, linkMode)); err != nil {
				slog.ErrorContext(c.Context, "failed to mount fs", "mountPoint", mountPoint, "err", err)
				return err
			}
			defer func() {
				layManager.ReleaseAll(c.Context)
				err := syscall.Unmount(mountPoint, 0)
				if err != nil {
					slog.ErrorContext(c.Context, "unmount failed", "err", err)
				}
				slog.InfoContext(c.Context, "Exiting")
			}()
			waitForSIGINT()
			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		if errdefs.IsConnectionClosed(err) {
			slog.Info("snapshotter exited")
			return
		}
		slog.Error("failed to start crio nydus store", "err", err)
	}
}
