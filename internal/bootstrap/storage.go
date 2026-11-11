package bootstrap

import (
	"context"
	"errors"
	filemigration "fkteams/internal/adapters/storage/migration"
	sqlitestore "fkteams/internal/adapters/storage/sqlite"
	httpserver "fkteams/internal/adapters/transport/http"
	"fkteams/internal/adapters/transport/http/upgrade"
	"fkteams/internal/app/appdata"
	"fkteams/internal/app/config"
	appmigration "fkteams/internal/app/migration"
	"fkteams/web"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
)

func (d *ExecutionDependencies) prepareStorage(ctx context.Context, mode httpserver.Mode, options *httpserver.ServeOptions) (context.Context, error) {
	if d.database != nil {
		return d.Context(ctx), nil
	}
	root := appdata.Dir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0700); err != nil {
		return ctx, err
	}
	unlock, err := sqlitestore.LockDirectory(filepath.Join(root, "data", "application.lock"))
	if err != nil {
		return ctx, err
	}
	database, err := sqlitestore.Open(ctx, filepath.Join(root, "data", "fkteams.db"))
	if err != nil {
		unlock()
		return ctx, err
	}
	success := false
	defer func() {
		if !success {
			database.Close()
			unlock()
		}
	}()
	service, err := appmigration.NewService(ctx, &filemigration.Source{Root: root}, database)
	if err != nil {
		return ctx, err
	}
	if service.Status().State != "completed" {
		migrationContext, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer cancel()
		legacy, err := hasLegacyData(root)
		if err != nil {
			return ctx, err
		}
		if mode == httpserver.ModeWeb && legacy {
			host := config.Get().Server.Host
			if host == "" {
				host = "127.0.0.1"
			}
			port := config.Get().Server.Port
			if port == 0 {
				port = 23456
			}
			if options != nil {
				if options.Host != "" {
					host = options.Host
				}
				if options.Port > 0 {
					port = options.Port
				}
			}
			fmt.Fprintf(os.Stderr, "检测到旧版数据，请打开 http://%s 完成一次性升级。\n", net.JoinHostPort(host, strconv.Itoa(port)))
			err = upgrade.Serve(migrationContext, net.JoinHostPort(host, strconv.Itoa(port)), web.GetFS(), service)
			if err != nil {
				cancel()
				if service.Status().State == "running" {
					_ = service.Wait(context.Background())
				}
				return ctx, err
			}
		} else {
			if legacy {
				fmt.Fprintln(os.Stderr, "正在备份、迁移并校验旧版业务数据...")
			}
			service.Start(migrationContext)
			if err := service.Wait(context.Background()); err != nil {
				return ctx, err
			}
			if legacy {
				fmt.Fprintf(os.Stderr, "数据升级完成，备份位置：%s\n", service.Status().BackupPath)
			}
		}
	}
	status, err := database.MigrationStatus(ctx)
	if err != nil {
		return ctx, err
	}
	documents, err := sqlitestore.NewDocuments(database, root)
	if err != nil {
		return ctx, err
	}
	fresh, err := newExecutionDependencies(documents, sqlitestore.NewMemoryStore(database))
	if err != nil {
		return ctx, err
	}
	d.Dependencies = fresh.Dependencies
	d.ExecutionGate = fresh.ExecutionGate
	d.ModelProviderRegistry = fresh.ModelProviderRegistry
	d.MemoryStore = fresh.MemoryStore
	d.MigrationStatus = status
	d.database = database
	d.unlockStorage = unlock
	success = true
	return d.Context(ctx), nil
}
func (d *ExecutionDependencies) closeStorage() error {
	d.storageCloseOnce.Do(func() {
		if d.database != nil {
			d.storageCloseError = d.database.Close()
		}
		if d.unlockStorage != nil {
			d.storageCloseError = errors.Join(d.storageCloseError, d.unlockStorage())
		}
	})
	return d.storageCloseError
}
func hasLegacyData(root string) (bool, error) {
	found := false
	for _, directory := range []string{"sessions", "scheduler", "share", "javascript/state", "workspace/memory"} {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(directory)), func(_ string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				found = true
				return fs.SkipAll
			}
			return nil
		})
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}
