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
	return "goravel-ship uchun kerakli fayllarni loyihaga qo'shadi"
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
			ctx.Warning(fmt.Sprintf("'%s' allaqachon mavjud, o'tkazib yuborildi", dest))
			continue
		}

		data, err := stubs.ReadFile(stub)
		if err != nil {
			ctx.Error(fmt.Sprintf("'%s' o'qib bo'lmadi: %v", stub, err))
			return err
		}

		if err := os.WriteFile(dest, data, 0644); err != nil {
			ctx.Error(fmt.Sprintf("'%s' yozib bo'lmadi: %v", dest, err))
			return err
		}

		ctx.Info(fmt.Sprintf("✓ %s yaratildi", dest))
	}

	ctx.NewLine()
	ctx.Success("goravel-ship o'rnatildi! .env.prod.example ni tahrirlang va artisan ship ni ishga tushiring.")
	return nil
}
