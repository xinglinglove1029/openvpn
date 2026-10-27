package openvpnweb

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestFirewallNftOutputRejectsOversizedSnapshot(t *testing.T) {
	var output firewallNftOutput
	if _, err := output.Write(bytes.Repeat([]byte("x"), firewallNftMaxOutput)); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Write([]byte("x")); err == nil {
		t.Fatal("oversized nft output was allowed to grow without a bound")
	}
	if output.Len() != firewallNftMaxOutput {
		t.Fatalf("output grew beyond its bound: %d", output.Len())
	}
}

func TestFirewallProcessHelper(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "--nft-helper" || i+2 >= len(os.Args) {
			continue
		}
		lock := filepath.Join(os.Args[i+1], "active")
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(23)
		}
		f.Close()
		delay, _ := strconv.Atoi(os.Args[i+2])
		time.Sleep(time.Duration(delay) * time.Millisecond)
		os.Remove(lock)
		os.Stdout.WriteString("finished")
		os.Exit(0)
	}
}

func helperNftCommand(t *testing.T, dir string, delay int, timeout time.Duration) *firewallNftCommand {
	t.Helper()
	return &firewallNftCommand{executable: os.Args[0], args: []string{"-test.run=^TestFirewallProcessHelper$", "--", "--nft-helper", dir, strconv.Itoa(delay)}, timeout: timeout}
}

func TestFirewallNftCommandsSerializeChildProcesses(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := helperNftCommand(t, dir, 60, 5*time.Second).CombinedOutput()
			if err != nil || string(out) != "finished" {
				t.Errorf("overlapping or failed nft process: %q, %v", out, err)
			}
		}()
	}
	wg.Wait()
}

func TestFirewallNftCommandKillsStalledProcessAndReleasesSlot(t *testing.T) {
	start := time.Now()
	_, err := helperNftCommand(t, t.TempDir(), 10000, 200*time.Millisecond).CombinedOutput()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline exceeded, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("stalled child was not stopped promptly")
	}
	out, err := helperNftCommand(t, t.TempDir(), 0, time.Second).CombinedOutput()
	if err != nil || string(out) != "finished" {
		t.Fatalf("process slot was not released: %q, %v", out, err)
	}
}

func TestFirewallNftQueueHasDeadline(t *testing.T) {
	firewallNftSlot <- struct{}{}
	defer func() { <-firewallNftSlot }()
	_, err := helperNftCommand(t, t.TempDir(), 0, 30*time.Millisecond).CombinedOutput()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting for busy slot did not time out: %v", err)
	}
}
