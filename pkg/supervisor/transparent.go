package supervisor

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/linkerd/linkerd2/pkg/supervisor/child"
)

// TransparentSupervisor used to be supervisor without any logic and just launch child process
// Used in cases when no supervisor required
func TransparentSupervisor(ctx context.Context, p Params, logger logrus.FieldLogger) error {
	data, err := json.Marshal(p)
	if err != nil {
		return errors.Wrap(err, "failed to marshal supervisor params")
	}

	logger.
		WithFields(logrus.Fields{
			"params": json.RawMessage(data),
		}).
		Infof("Start proxy transparent supervisor")

	ctx = listenStopSignals(ctx)

	c := child.New(p.ChildProcessArgs)

	// child will be stopped on os signal
	return c.Start(ctx)
}

func listenStopSignals(ctx context.Context) context.Context {
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
		case <-ch:
		}
	}()

	return ctx
}
