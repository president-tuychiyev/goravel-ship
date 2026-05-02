package commands

import (
	"embed"
	"fmt"
	"os"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"
)

//go:embed stubs/*
var stubs embed.FS

type InstallCommand struct{}

func (r *InstallCommand) Signature() string {
	return "ship:install"
}

func (r *InstallCommand) Description() string {
	return "adds the necessary files for goravel-ship to the project"
}

func (r *InstallCommand) Extend() command.Extend {
	return command.Extend{}
}

func (r *InstallCommand) Handle(ctx console.Context) error {
	files := map[string]string{
		"stubs/docker-compose-prod.yml": "docker-compose-prod.yml",
		"stubs/env.prod.example":        ".env.prod.example",
		"stubs/deploy.sh":               "deploy.sh",
	}

	for stub, dest := range files {
		if _, err := os.Stat(dest); err == nil {
			ctx.Warning(fmt.Sprintf("'%s' already exists, skipping", dest))
			continue
		}

		data, err := stubs.ReadFile(stub)
		if err != nil {
			ctx.Error(fmt.Sprintf("'%s' could not be read: %v", stub, err))
			return err
		}

		if err := os.WriteFile(dest, data, 0644); err != nil {
			ctx.Error(fmt.Sprintf("'%s' could not be written: %v", dest, err))
			return err
		}

		ctx.Info(fmt.Sprintf("✓ %s created", dest))
	}

	ctx.NewLine()
	ctx.Success("goravel-ship installed! Edit .env.prod.example and run artisan ship.")
	return nil
}
