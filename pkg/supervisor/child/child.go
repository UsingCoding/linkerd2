package child

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	"github.com/pkg/errors"
)

// Child - defines child process
type Child interface {
	Start(ctx context.Context) error

	Terminate() error
}

func New(args []string) Child {
	//nolint:gosec
	cmd := exec.Command(args[0], args[1:]...)

	// configure proxying streams and env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	return &child{
		cmd: cmd,
	}
}

type child struct {
	cmd *exec.Cmd
}

func (c child) Start(ctx context.Context) error {
	err := c.cmd.Start()
	if err != nil {
		return errors.Wrap(err, "failed to start child")
	}

	go func() {
		<-ctx.Done()

		// ignore if failed to terminate child
		_ = c.Terminate()
	}()

	return c.cmd.Wait()
}

func (c child) Terminate() error {
	err := c.cmd.Process.Signal(syscall.SIGTERM)
	return errors.Wrap(err, "failed to terminate child process")
}
