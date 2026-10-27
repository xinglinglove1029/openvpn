package openvpnweb

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// A stalled netlink request must never leave an unbounded number of nft
// children behind as WebSocket polling and lifecycle hooks keep arriving.
var firewallNftSlot = make(chan struct{}, 1)

const firewallNftTimeout = 5 * time.Second
const firewallNftMaxOutput = 4 << 20

type firewallNftCommand struct {
	executable string
	args       []string
	timeout    time.Duration
	Stdin      io.Reader
}

func newFirewallNftCommand(executable string, args ...string) *firewallNftCommand {
	return &firewallNftCommand{executable: executable, args: args, timeout: firewallNftTimeout}
}

type firewallNftOutput struct{ bytes.Buffer }

func (b *firewallNftOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > firewallNftMaxOutput {
		return 0, fmt.Errorf("nft output exceeds %d bytes", firewallNftMaxOutput)
	}
	return b.Buffer.Write(p)
}

func (c *firewallNftCommand) execute(combined bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	select {
	case firewallNftSlot <- struct{}{}:
		defer func() { <-firewallNftSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	cmd := exec.CommandContext(ctx, c.executable, c.args...)
	cmd.WaitDelay = time.Second
	cmd.Stdin = c.Stdin
	var output firewallNftOutput
	cmd.Stdout = &output
	if combined {
		cmd.Stderr = &output
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		return output.Bytes(), ctx.Err()
	}
	return output.Bytes(), err
}

func (c *firewallNftCommand) Output() ([]byte, error)         { return c.execute(false) }
func (c *firewallNftCommand) CombinedOutput() ([]byte, error) { return c.execute(true) }
func (c *firewallNftCommand) Run() error                      { _, err := c.execute(false); return err }
