package supervisor

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/linkerd/linkerd2/pkg/supervisor/child"
	"github.com/linkerd/linkerd2/pkg/supervisor/kubewatch"
)

type Params struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`

	GracefulTimeout  time.Duration `json:"graceful_timeout"`
	ChildProcessArgs []string      `json:"child_process_args"`

	Logger logrus.FieldLogger `json:"-"`
}

// Supervisor track status of containers in Pod (except linkerd-proxy) and wait for their termination
// before terminate child process - linkerd-proxy
func Supervisor(ctx context.Context, p Params) error {
	data, err := json.Marshal(p)
	if err != nil {
		return errors.Wrap(err, "failed to marshal supervisor params")
	}

	p.Logger.
		WithFields(logrus.Fields{
			"params":  json.RawMessage(data),
			"params1": p,
		}).
		Infof("Start proxy supervisor")

	ctx = gracefulTermination(ctx, p.GracefulTimeout)

	eg, ctx := errgroup.WithContext(ctx)

	c := child.New(p.ChildProcessArgs)

	// ctx for child process
	childCtx, childCancel := context.WithCancel(ctx)
	defer childCancel()

	eg.Go(func() error {
		return kubewatch.Watcher{}.
			Start(ctx, kubewatch.StartParams{
				Namespace: p.Namespace,
				Pod:       p.Pod,
				Cancel:    childCancel,
			})
	})

	eg.Go(func() error {
		return c.Start(childCtx)
	})

	select {
	case <-ctx.Done():
		p.Logger.
			Info("Graceful timeout exhausted, proxy will be terminated")

		err = ctx.Err()
		if terminateErr := c.Terminate(); terminateErr != nil {
			err = errors.Wrap(err, terminateErr.Error())
		}

		return err
	case <-childCtx.Done():
		p.Logger.
			Info("Containers terminated, proxy exiting")

		return eg.Wait()
	}
}

func gracefulTermination(ctx context.Context, d time.Duration) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		defer func() {
			signal.Stop(ch)
			cancel()
		}()
		select {
		case <-ctx.Done():
			// immediate exit
		case <-ch:
			// exit after graceful timeout
			<-time.After(d)
		}
	}()

	return ctx
}
