package commands

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/goravel/framework/contracts/console"
	"github.com/goravel/framework/contracts/console/command"
)

type ShipCommand struct{}

func (r *ShipCommand) Signature() string {
	return "ship"
}

func (r *ShipCommand) Description() string {
	return "Docker builds the image and uploads it to the server."
}

func (r *ShipCommand) Extend() command.Extend {
	return command.Extend{
		Flags: []command.Flag{
			&command.StringFlag{
				Name:     "user",
				Aliases:  []string{"u"},
				Usage:    "SSH username",
				Required: true,
			},
			&command.StringFlag{
				Name:     "ip",
				Usage:    "Server IP address",
				Required: true,
			},
			&command.StringFlag{
				Name:    "path",
				Aliases: []string{"p"},
				Usage:   "Remote project directory",
				Value:   "/opt/app",
			},
			&command.StringFlag{
				Name:    "tag",
				Aliases: []string{"t"},
				Usage:   "Docker image tag",
				Value:   "latest",
			},
			&command.StringFlag{
				Name:    "image",
				Aliases: []string{"i"},
				Usage:   "Docker image name",
			},
			&command.StringFlag{
				Name:    "container",
				Aliases: []string{"c"},
				Usage:   "Docker container name",
			},
			&command.StringFlag{
				Name:    "binary",
				Aliases: []string{"b"},
				Usage:   "Binary path inside container",
				Value:   "/usr/local/bin/app",
			},
			&command.IntFlag{
				Name:    "port",
				Aliases: []string{"P"},
				Usage:   "SSH port",
				Value:   22,
			},
			&command.BoolFlag{
				Name:    "migrate",
				Aliases: []string{"m"},
				Usage:   "Run migrations after deploy",
			},
			&command.BoolFlag{
				Name:    "seed",
				Aliases: []string{"s"},
				Usage:   "Run seeders after deploy",
			},
		},
	}
}

func (r *ShipCommand) Handle(ctx console.Context) error {
	user := ctx.Option("user")
	ip := ctx.Option("ip")
	path := ctx.Option("path")
	tag := ctx.Option("tag")
	imageName := ctx.Option("image")
	containerName := ctx.Option("container")
	binaryPath := ctx.Option("binary")
	port := ctx.OptionInt("port")
	migrate := ctx.OptionBool("migrate")
	seed := ctx.OptionBool("seed")

	if imageName == "" {
		imageName = detectImageName()
	}
	if containerName == "" {
		containerName = imageName
	}

	target := fmt.Sprintf("%s@%s", user, ip)
	imageTagged := fmt.Sprintf("%s:%s", imageName, tag)
	portStr := fmt.Sprintf("%d", port)
	hash := buildHash(imageTagged)
	tarGz := fmt.Sprintf("%s.tar.gz", hash)

	ctx.NewLine()
	ctx.Info(fmt.Sprintf("Target    : %s", target))
	ctx.Info(fmt.Sprintf("Port      : %d", port))
	ctx.Info(fmt.Sprintf("Path      : %s", path))
	ctx.Info(fmt.Sprintf("Image     : %s", imageTagged))
	ctx.Info(fmt.Sprintf("Container : %s", containerName))
	ctx.Info(fmt.Sprintf("Migrate   : %v", migrate))
	ctx.Info(fmt.Sprintf("Seed      : %v", seed))
	ctx.Divider()

	err := deploy(ctx, target, portStr, path, tag, imageTagged, imageName, containerName, binaryPath, tarGz, migrate, seed)

	os.Remove(tarGz)

	if err != nil {
		ctx.Error(err.Error())
		os.Exit(1)
	}

	ctx.NewLine()
	ctx.Success("Ship muvaffaqiyatli yakunlandi!")
	return nil
}

func deploy(ctx console.Context, target, portStr, path, tag, imageTagged, imageName, containerName, binaryPath, tarGz string, migrate, seed bool) error {
	controlPath := fmt.Sprintf("/tmp/ship_ctl_%s", tarGz[:8])
	defer exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-O", "exit", target,
	).Run()

	ssh := func(cmd string) error {
		return run(ctx, "ssh",
			"-p", portStr,
			"-o", "ControlMaster=auto",
			"-o", fmt.Sprintf("ControlPath=%s", controlPath),
			"-o", "ControlPersist=300",
			target, cmd,
		)
	}

	scp := func(files ...string) error {
		args := []string{
			"-P", portStr,
			"-o", "ControlMaster=auto",
			"-o", fmt.Sprintf("ControlPath=%s", controlPath),
			"-o", "ControlPersist=300",
		}
		return run(ctx, "scp", append(args, files...)...)
	}

	// .env.prod.example → .env
	if err := copyFile(".env.prod.example", ".env"); err != nil {
		return fmt.Errorf(".env.prod.example topilmadi: %w", err)
	}
	ctx.Info(".env.prod.example → .env")

	// Docker build
	if err := run(ctx, "docker", "build", "-t", imageTagged, "."); err != nil {
		return err
	}
	if tag != "latest" {
		if err := run(ctx, "docker", "tag", imageTagged, imageName+":latest"); err != nil {
			return err
		}
	}

	// Image saqlash
	ctx.Info(fmt.Sprintf(">>> Save → %s", tarGz))
	if err := saveImage(imageTagged, tarGz); err != nil {
		return fmt.Errorf("save xatosi: %w", err)
	}

	// Remote papka
	if err := ssh(fmt.Sprintf("mkdir -p %s", path)); err != nil {
		return err
	}

	// SCP: tar.gz, .env, deploy.sh
	scpFiles := []string{tarGz, ".env"}
	if _, err := os.Stat("deploy.sh"); err == nil {
		scpFiles = append(scpFiles, "deploy.sh")
	} else {
		ctx.Warning("'deploy.sh' topilmadi, o'tkazib yuborildi")
	}
	scpFiles = append(scpFiles, fmt.Sprintf("%s:%s/", target, path))
	if err := scp(scpFiles...); err != nil {
		return err
	}

	// docker-compose-prod.yml → docker-compose.yml
	if _, err := os.Stat("docker-compose-prod.yml"); err == nil {
		dest := fmt.Sprintf("%s:%s/docker-compose.yml", target, path)
		if err := scp("docker-compose-prod.yml", dest); err != nil {
			return err
		}
	} else {
		ctx.Warning("'docker-compose-prod.yml' topilmadi, o'tkazib yuborildi")
	}

	// deploy.sh ishga tushirish
	if err := ssh(fmt.Sprintf("cd %s && bash deploy.sh %s", path, tarGz)); err != nil {
		return err
	}

	// Migrate
	if migrate {
		ctx.Info(">>> Migrating...")
		if err := ssh(fmt.Sprintf("docker exec %s %s artisan migrate", containerName, binaryPath)); err != nil {
			return err
		}
	}

	// Seed
	if seed {
		ctx.Info(">>> Seeding...")
		if err := ssh(fmt.Sprintf("docker exec %s %s artisan db:seed", containerName, binaryPath)); err != nil {
			return err
		}
	}

	return nil
}

func detectImageName() string {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "app"
	}
	for _, line := range splitLines(string(data)) {
		if len(line) > 7 && line[:7] == "module " {
			mod := line[7:]
			for i := len(mod) - 1; i >= 0; i-- {
				if mod[i] == '/' {
					return mod[i+1:]
				}
			}
			return mod
		}
	}
	return "app"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func buildHash(seed string) string {
	raw := fmt.Sprintf("%s_%d", seed, time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)[:12]
}

func run(ctx console.Context, name string, args ...string) error {
	ctx.Info(fmt.Sprintf(">>> %s %v", name, args))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func saveImage(image, tarGz string) error {
	save := exec.Command("docker", "save", image)
	gzip := exec.Command("gzip")

	out, err := os.Create(tarGz)
	if err != nil {
		return err
	}
	defer out.Close()

	gzip.Stdin, _ = save.StdoutPipe()
	gzip.Stdout = out
	gzip.Stderr = os.Stderr
	save.Stderr = os.Stderr

	if err := gzip.Start(); err != nil {
		return err
	}
	if err := save.Run(); err != nil {
		return err
	}
	return gzip.Wait()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
