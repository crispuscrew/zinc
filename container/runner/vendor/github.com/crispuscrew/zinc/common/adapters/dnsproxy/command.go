package dnsproxy

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Command runs the explicitly provisioned proxy until interrupted or terminated.
func Command(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return Run(ctx, args)
}
