package ship

import (
	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/foundation"

	"github.com/president-tuychiyev/goravel-ship/commands"
)

type ServiceProvider struct{}

func (s *ServiceProvider) Register(app foundation.Application) {
	app.Commands([]console.Command{
		&commands.ShipCommand{},
		&commands.InstallCommand{},
	})
}

func (s *ServiceProvider) Boot(app foundation.Application) {}
