package integ

import (
	"time"
	"context"
	"bytes"
	"github.com/pkg/errors"
)

type Beat struct {
	Dir  string // Working directory.
	Path string
	ConfigFile string
	HTTPProfAddress string
	Args []string

	combinedOutput *bytes.Buffer
}

func (b *Beat) CombinedOutput() *bytes.Buffer {
	return b.combinedOutput
}

func (b *Beat) Run(timeout time.Duration) (*Cmd, error) {
	b.combinedOutput = &bytes.Buffer{}

	if b.ConfigFile != "" {
		b.Args = append(b.Args, "-c", b.ConfigFile)
	}
	if b.HTTPProfAddress != "" {
		b.Args = append(b.Args, "-httpprof", b.HTTPProfAddress)
	}
	b.Args = append(b.Args, "-e")

	var cmd *Cmd
	if timeout > 0 {
		timeoutCtx, _ := context.WithTimeout(context.Background(), timeout)
		cmd = CommandContext(timeoutCtx, b.Path, b.Args...)
	} else {
		cmd = Command(b.Path, b.Args...)
	}

	cmd.Dir = b.Dir
	cmd.Stdout = b.combinedOutput
	cmd.Stderr = b.combinedOutput
	return cmd, cmd.Start()
}

func (b *Beat) RunWithCondition(timeout time.Duration, condition func(b *Beat) error) (*Cmd, error) {
	cmd, err := b.Run(timeout)
	if err != nil {
		return cmd, err
	}

	conditionCtx, conditionCancel := context.WithCancel(context.Background())
	defer conditionCancel()

	wait := make(chan error, 1)
	go func() {
		wait <- cmd.Wait()
		close(wait)
		conditionCancel()
	}()

	if err := b.waitForCondition(conditionCtx, condition); err != nil {
		if err != context.Canceled {
			return cmd, errors.Wrap(err, "condition was not met")
		}
	}

	// Trigger graceful shutdown of the Beat.
	cmd.SendCtrlCSignal()

	return cmd, <-wait
}

func (b *Beat) waitForCondition(ctx context.Context, condition func(b *Beat) error) error {
	tick := time.NewTimer(500 * time.Millisecond)
	defer tick.Stop()

	for {
		tick.Reset(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}

		if condition(b) == nil {
			return nil
		}
	}
}
