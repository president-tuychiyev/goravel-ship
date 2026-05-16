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
	return "Build a Docker image locally and ship it to a remote server over SSH"
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
				Usage:   "Docker image name (auto-detected from go.mod if omitted)",
			},
			&command.StringFlag{
				Name:    "container",
				Aliases: []string{"c"},
				Usage:   "Docker container name (defaults to image name)",
			},
			&command.StringFlag{
				Name:    "binary",
				Aliases: []string{"b"},
				Usage:   "Binary path inside the container",
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
				Usage:   "Run database migrations after deploy",
			},
			&command.BoolFlag{
				Name:  "fresh",
				Usage: "Drop all tables and re-run migrations (use with --migrate)",
			},
			&command.BoolFlag{
				Name:    "seed",
				Aliases: []string{"s"},
				Usage:   "Run database seeders after deploy",
			},
			&command.BoolFlag{
				Name:  "root",
				Usage: "Run privileged remote commands with sudo (passwordless sudo required)",
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
	fresh := ctx.OptionBool("fresh")
	seed := ctx.OptionBool("seed")
	root := ctx.OptionBool("root")

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
	ctx.Info(fmt.Sprintf("Fresh     : %v", fresh))
	ctx.Info(fmt.Sprintf("Seed      : %v", seed))
	ctx.Info(fmt.Sprintf("Root      : %v", root))
	ctx.Divider()

	err := deploy(ctx, target, user, portStr, path, tag, imageTagged, imageName, containerName, binaryPath, tarGz, migrate, fresh, seed, root)

	os.Remove(tarGz)

	if err != nil {
		ctx.Error(err.Error())
		os.Exit(1)
	}

	ctx.NewLine()
	ctx.Success("Shipped successfully!")
	return nil
}

func deploy(ctx console.Context, target, user, portStr, path, tag, imageTagged, imageName, containerName, binaryPath, tarGz string, migrate, fresh, seed, root bool) error {
	controlPath := fmt.Sprintf("/tmp/ship_ctl_%s", tarGz[:8])
	defer exec.Command("ssh",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-O", "exit", target,
	).Run()

	sshExec := func(cmd string, asRoot bool) error {
		args := []string{
			"-p", portStr,
			"-o", "ControlMaster=auto",
			"-o", fmt.Sprintf("ControlPath=%s", controlPath),
			"-o", "ControlPersist=300",
		}
		if root && asRoot {
			args = append(args, "-tt")
			cmd = "sudo " + cmd
		}
		args = append(args, target, cmd)
		return run(ctx, "ssh", args...)
	}
	ssh := func(cmd string) error { return sshExec(cmd, false) }
	sshRoot := func(cmd string) error { return sshExec(cmd, true) }

	scp := func(files ...string) error {
		args := []string{
			"-P", portStr,
			"-o", "ControlMaster=auto",
			"-o", fmt.Sprintf("ControlPath=%s", controlPath),
			"-o", "ControlPersist=300",
		}
		return run(ctx, "scp", append(args, files...)...)
	}

	// Always rebuild .env from .env.prod.example
	if err := copyFile(".env.prod.example", ".env"); err != nil {
		return fmt.Errorf(".env.prod.example not found: %w", err)
	}
	ctx.Info(".env.prod.example → .env")

	// Build Docker image
	if err := run(ctx, "docker", "build", "-t", imageTagged, "."); err != nil {
		return err
	}
	if tag != "latest" {
		if err := run(ctx, "docker", "tag", imageTagged, imageName+":latest"); err != nil {
			return err
		}
	}

	// Save image to tar.gz
	ctx.Info(fmt.Sprintf(">>> Saving image → %s", tarGz))
	if err := saveImage(imageTagged, tarGz); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Create remote directory. With --root, chown to the SSH user so scp can upload.
	if root {
		if err := sshRoot(fmt.Sprintf("mkdir -p %s && chown -R %s: %s", path, user, path)); err != nil {
			return err
		}
	} else {
		if err := ssh(fmt.Sprintf("mkdir -p %s", path)); err != nil {
			return err
		}
	}

	// Upload: tar.gz, .env, deploy.sh
	scpFiles := []string{tarGz, ".env"}
	if _, err := os.Stat("deploy.sh"); err == nil {
		scpFiles = append(scpFiles, "deploy.sh")
	} else {
		ctx.Warning("'deploy.sh' not found, skipping")
	}
	scpFiles = append(scpFiles, fmt.Sprintf("%s:%s/", target, path))
	if err := scp(scpFiles...); err != nil {
		return err
	}

	// Upload docker-compose-prod.yml as docker-compose.yml on the server
	if _, err := os.Stat("docker-compose-prod.yml"); err == nil {
		dest := fmt.Sprintf("%s:%s/docker-compose.yml", target, path)
		if err := scp("docker-compose-prod.yml", dest); err != nil {
			return err
		}
	} else {
		ctx.Warning("'docker-compose-prod.yml' not found, skipping")
	}

	// Run deploy.sh on the server — pass IMAGE_NAME so compose uses the correct local image
	if err := sshRoot(fmt.Sprintf("cd %s && IMAGE_NAME=%s bash deploy.sh %s", path, imageName, tarGz)); err != nil {
		return err
	}

	// Run migrations
	if migrate {
		migrateCmd := "migrate"
		if fresh {
			migrateCmd = "migrate:fresh"
		}
		ctx.Info(fmt.Sprintf(">>> Running artisan %s...", migrateCmd))
		if err := sshRoot(fmt.Sprintf("docker exec %s %s artisan %s", containerName, binaryPath, migrateCmd)); err != nil {
			return err
		}
	}

	// Run seeders
	if seed {
		ctx.Info(">>> Running seeders...")
		if err := sshRoot(fmt.Sprintf("docker exec %s %s artisan db:seed --force", containerName, binaryPath)); err != nil {
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
