package mcwhitelist

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRCONPacketRoundTrip(t *testing.T) {
	want := rconPacket{ID: 42, Type: rconExecCommand, Body: "whitelist add Steve"}
	var wire bytes.Buffer
	written, err := writeRCONPacket(&wire, want)
	if err != nil {
		t.Fatal(err)
	}
	if written != wire.Len() {
		t.Fatalf("write count = %d, buffer = %d", written, wire.Len())
	}
	got, err := readRCONPacket(&wire)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("packet = %#v, want %#v", got, want)
	}
}

func TestRCONPacketValidation(t *testing.T) {
	if _, err := writeRCONPacket(&bytes.Buffer{}, rconPacket{Body: "bad\x00body"}); err == nil {
		t.Error("NUL body unexpectedly accepted")
	}
	if _, err := writeRCONPacket(&bytes.Buffer{}, rconPacket{Body: strings.Repeat("x", rconMaxPacketSize)}); err == nil {
		t.Error("oversized body unexpectedly accepted")
	}
	for _, size := range []int32{9, rconMaxPacketSize + 1} {
		var wire bytes.Buffer
		if err := binary.Write(&wire, binary.LittleEndian, size); err != nil {
			t.Fatal(err)
		}
		if _, err := readRCONPacket(&wire); err == nil {
			t.Errorf("packet size %d unexpectedly accepted", size)
		}
	}

	var malformed bytes.Buffer
	if err := binary.Write(&malformed, binary.LittleEndian, int32(10)); err != nil {
		t.Fatal(err)
	}
	malformed.Write(make([]byte, 8))
	malformed.Write([]byte{'x', 'y'})
	if _, err := readRCONPacket(&malformed); err == nil {
		t.Error("missing packet terminators unexpectedly accepted")
	}
}

func TestClassifyWhitelistResponse(t *testing.T) {
	tests := []struct {
		name    string
		adding  bool
		value   string
		outcome remoteOutcome
	}{
		{"added", true, "Added Steve to the whitelist", remoteDesired},
		{"formatted added", true, "§aAdded Steve to the whitelist", remoteDesired},
		{"already added", true, "Nothing changed. That player is already whitelisted", remoteDesired},
		{"removed", false, "Removed Steve from the whitelist", remoteDesired},
		{"already absent contraction", false, "Nothing changed. That player isn't whitelisted", remoteDesired},
		{"already absent", false, "That player is not whitelisted", remoteDesired},
		{"rejected", true, "Unknown or incomplete command", remoteRejected},
		{"localized or custom", true, "已将 Steve 加入白名单", remoteUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome, _ := classifyWhitelistResponse(test.value, test.adding)
			if outcome != test.outcome {
				t.Fatalf("outcome = %d, want %d", outcome, test.outcome)
			}
		})
	}
}

type fakeExecutor struct {
	response string
	err      error
	commands []string
}

func (f *fakeExecutor) execute(_ context.Context, command string) (string, error) {
	f.commands = append(f.commands, command)
	return f.response, f.err
}

func TestMinecraftWhitelisterUsesExactCommandsAndUncertainty(t *testing.T) {
	executor := &fakeExecutor{response: "Added Steve to the whitelist"}
	whitelist := &minecraftWhitelister{executor: executor}
	if outcome, err := whitelist.add(context.Background(), "Steve"); err != nil || outcome != remoteDesired {
		t.Fatalf("add outcome/error = %d, %v", outcome, err)
	}
	executor.response = "Removed Steve from the whitelist"
	if outcome, err := whitelist.remove(context.Background(), "Steve"); err != nil || outcome != remoteDesired {
		t.Fatalf("remove outcome/error = %d, %v", outcome, err)
	}
	want := []string{"whitelist add Steve", "whitelist remove Steve"}
	if fmt.Sprint(executor.commands) != fmt.Sprint(want) {
		t.Fatalf("commands = %v, want %v", executor.commands, want)
	}
	executor.err = &rconOperationError{operation: "read", uncertain: true, err: errors.New("timeout")}
	if outcome, _ := whitelist.add(context.Background(), "Alex"); outcome != remoteUnknown {
		t.Fatalf("uncertain error outcome = %d, want remoteUnknown", outcome)
	}
}

func TestRCONExecutorAuthenticatesAndExecutes(t *testing.T) {
	dial, commands, serverErrors := startRCONTestServer(t, "secret", func(command string) (string, bool) {
		return "Added Steve to the whitelist", true
	})
	executor := &rconExecutor{address: "test:0", password: "secret", timeout: time.Second, dial: dial}
	response, err := executor.execute(context.Background(), "whitelist add Steve")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if response != "Added Steve to the whitelist" {
		t.Fatalf("response = %q", response)
	}
	if command := <-commands; command != "whitelist add Steve" {
		t.Fatalf("server command = %q", command)
	}
	if err := <-serverErrors; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestRCONExecutorAuthenticationFailureIsDefinite(t *testing.T) {
	dial, _, serverErrors := startRCONTestServer(t, "secret", nil)
	executor := &rconExecutor{address: "test:0", password: "wrong", timeout: time.Second, dial: dial}
	_, err := executor.execute(context.Background(), "whitelist add Steve")
	if err == nil {
		t.Fatal("authentication unexpectedly succeeded")
	}
	if commandMayHaveRun(err) {
		t.Fatalf("authentication error marked uncertain: %v", err)
	}
	if err := <-serverErrors; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestRCONExecutorLostResponseIsUncertain(t *testing.T) {
	dial, commands, serverErrors := startRCONTestServer(t, "secret", func(command string) (string, bool) {
		return "", false
	})
	executor := &rconExecutor{address: "test:0", password: "secret", timeout: time.Second, dial: dial}
	_, err := executor.execute(context.Background(), "whitelist remove Steve")
	if err == nil || !commandMayHaveRun(err) {
		t.Fatalf("lost-response error = %v, want uncertain", err)
	}
	if command := <-commands; command != "whitelist remove Steve" {
		t.Fatalf("server command = %q", command)
	}
	if err := <-serverErrors; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func startRCONTestServer(t *testing.T, password string, respond func(string) (string, bool)) (func(context.Context, string, string) (net.Conn, error), <-chan string, <-chan error) {
	t.Helper()
	client, server := net.Pipe()
	used := false
	dial := func(context.Context, string, string) (net.Conn, error) {
		if used {
			return nil, errors.New("test connection already used")
		}
		used = true
		return client, nil
	}
	commands := make(chan string, 1)
	serverErrors := make(chan error, 1)
	go func() {
		defer server.Close()
		auth, err := readRCONPacket(server)
		if err != nil {
			serverErrors <- err
			return
		}
		if auth.Type != rconAuth {
			serverErrors <- fmt.Errorf("auth type = %d", auth.Type)
			return
		}
		if auth.Body != password {
			_, err = writeRCONPacket(server, rconPacket{ID: -1, Type: rconAuthResponse})
			serverErrors <- err
			return
		}
		if _, err = writeRCONPacket(server, rconPacket{ID: auth.ID, Type: rconResponseValue}); err != nil {
			serverErrors <- err
			return
		}
		if _, err = writeRCONPacket(server, rconPacket{ID: auth.ID, Type: rconAuthResponse}); err != nil {
			serverErrors <- err
			return
		}
		command, err := readRCONPacket(server)
		if err != nil {
			serverErrors <- err
			return
		}
		commands <- command.Body
		response, shouldRespond := respond(command.Body)
		if shouldRespond {
			_, err = writeRCONPacket(server, rconPacket{ID: command.ID, Type: rconResponseValue, Body: response})
		}
		serverErrors <- err
	}()
	return dial, commands, serverErrors
}
