package mcwhitelist

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const (
	rconResponseValue int32 = 0
	rconExecCommand   int32 = 2
	rconAuthResponse  int32 = 2
	rconAuth          int32 = 3
	rconMaxPacketSize       = 4096
)

type remoteOutcome uint8

const (
	remoteDesired remoteOutcome = iota
	remoteRejected
	remoteUnknown
)

type whitelister interface {
	add(context.Context, string) (remoteOutcome, error)
	remove(context.Context, string) (remoteOutcome, error)
}

type minecraftWhitelister struct {
	executor commandExecutor
}

type commandExecutor interface {
	execute(context.Context, string) (string, error)
}

func (w *minecraftWhitelister) add(ctx context.Context, playerName string) (remoteOutcome, error) {
	response, err := w.executor.execute(ctx, "whitelist add "+playerName)
	if err != nil {
		if commandMayHaveRun(err) {
			return remoteUnknown, err
		}
		return remoteRejected, err
	}
	return classifyWhitelistResponse(response, true)
}

func (w *minecraftWhitelister) remove(ctx context.Context, playerName string) (remoteOutcome, error) {
	response, err := w.executor.execute(ctx, "whitelist remove "+playerName)
	if err != nil {
		if commandMayHaveRun(err) {
			return remoteUnknown, err
		}
		return remoteRejected, err
	}
	return classifyWhitelistResponse(response, false)
}

func classifyWhitelistResponse(response string, adding bool) (remoteOutcome, error) {
	normalized := strings.ToLower(stripMinecraftFormatting(strings.TrimSpace(response)))
	if adding {
		if (strings.Contains(normalized, "added ") && strings.Contains(normalized, "whitelist")) ||
			strings.Contains(normalized, "already whitelisted") {
			return remoteDesired, nil
		}
	} else if (strings.Contains(normalized, "removed ") && strings.Contains(normalized, "whitelist")) ||
		strings.Contains(normalized, "isn't whitelisted") ||
		strings.Contains(normalized, "is not whitelisted") ||
		strings.Contains(normalized, "not whitelisted") {
		return remoteDesired, nil
	}

	if strings.Contains(normalized, "unknown command") ||
		strings.Contains(normalized, "unknown or incomplete command") ||
		strings.Contains(normalized, "incorrect argument") {
		return remoteRejected, fmt.Errorf("minecraft rejected whitelist command: %q", response)
	}
	return remoteUnknown, fmt.Errorf("unrecognized minecraft RCON response: %q", response)
}

func stripMinecraftFormatting(value string) string {
	runes := []rune(value)
	result := make([]rune, 0, len(runes))
	for index := 0; index < len(runes); index++ {
		if runes[index] == '§' && index+1 < len(runes) {
			index++
			continue
		}
		result = append(result, runes[index])
	}
	return string(result)
}

type rconExecutor struct {
	address  string
	password string
	timeout  time.Duration
	dial     func(context.Context, string, string) (net.Conn, error)
}

func (r *rconExecutor) execute(ctx context.Context, command string) (string, error) {
	dial := r.dial
	if dial == nil {
		dialer := net.Dialer{Timeout: r.timeout}
		dial = dialer.DialContext
	}
	conn, err := dial(ctx, "tcp", r.address)
	if err != nil {
		return "", &rconOperationError{operation: "dial", err: err}
	}
	defer conn.Close()

	deadline := time.Now().Add(r.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", &rconOperationError{operation: "set deadline", err: err}
	}
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
	})
	defer stopCancellation()

	if err := authenticateRCON(conn, r.password); err != nil {
		return "", &rconOperationError{operation: "authenticate", err: err}
	}

	const commandID int32 = 2
	written, err := writeRCONPacket(conn, rconPacket{ID: commandID, Type: rconExecCommand, Body: command})
	if err != nil {
		return "", &rconOperationError{operation: "write command", uncertain: written > 0, err: err}
	}
	response, err := readRCONPacket(conn)
	if err != nil {
		return "", &rconOperationError{operation: "read command response", uncertain: true, err: err}
	}
	if response.ID != commandID || response.Type != rconResponseValue {
		return "", &rconOperationError{
			operation: "validate command response",
			uncertain: true,
			err:       fmt.Errorf("unexpected packet id=%d type=%d", response.ID, response.Type),
		}
	}
	return response.Body, nil
}

func authenticateRCON(conn net.Conn, password string) error {
	const authID int32 = 1
	if _, err := writeRCONPacket(conn, rconPacket{ID: authID, Type: rconAuth, Body: password}); err != nil {
		return fmt.Errorf("write authentication packet: %w", err)
	}
	for attempts := 0; attempts < 3; attempts++ {
		response, err := readRCONPacket(conn)
		if err != nil {
			return fmt.Errorf("read authentication response: %w", err)
		}
		if response.Type != rconAuthResponse {
			continue
		}
		if response.ID == -1 {
			return errors.New("RCON authentication rejected")
		}
		if response.ID != authID {
			return fmt.Errorf("unexpected authentication packet id %d", response.ID)
		}
		return nil
	}
	return errors.New("RCON authentication response was not received")
}

type rconOperationError struct {
	operation string
	uncertain bool
	err       error
}

func (e *rconOperationError) Error() string {
	return fmt.Sprintf("RCON %s: %v", e.operation, e.err)
}

func (e *rconOperationError) Unwrap() error {
	return e.err
}

func commandMayHaveRun(err error) bool {
	var operationError *rconOperationError
	return errors.As(err, &operationError) && operationError.uncertain
}

type rconPacket struct {
	ID   int32
	Type int32
	Body string
}

func writeRCONPacket(writer io.Writer, packet rconPacket) (int, error) {
	if strings.IndexByte(packet.Body, 0) >= 0 {
		return 0, errors.New("RCON packet body contains a NUL byte")
	}
	payloadSize := 10 + len(packet.Body)
	if payloadSize > rconMaxPacketSize {
		return 0, fmt.Errorf("RCON packet size %d exceeds %d", payloadSize, rconMaxPacketSize)
	}
	buffer := bytes.NewBuffer(make([]byte, 0, payloadSize+4))
	_ = binary.Write(buffer, binary.LittleEndian, int32(payloadSize))
	_ = binary.Write(buffer, binary.LittleEndian, packet.ID)
	_ = binary.Write(buffer, binary.LittleEndian, packet.Type)
	_, _ = buffer.WriteString(packet.Body)
	_ = buffer.WriteByte(0)
	_ = buffer.WriteByte(0)
	data := buffer.Bytes()
	total := 0
	for total < len(data) {
		written, err := writer.Write(data[total:])
		total += written
		if err != nil {
			return total, err
		}
		if written == 0 {
			return total, io.ErrUnexpectedEOF
		}
	}
	return total, nil
}

func readRCONPacket(reader io.Reader) (rconPacket, error) {
	var size int32
	if err := binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return rconPacket{}, err
	}
	if size < 10 || size > rconMaxPacketSize {
		return rconPacket{}, fmt.Errorf("invalid RCON packet size %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return rconPacket{}, err
	}
	if payload[len(payload)-2] != 0 || payload[len(payload)-1] != 0 {
		return rconPacket{}, errors.New("RCON packet is missing terminators")
	}
	return rconPacket{
		ID:   int32(binary.LittleEndian.Uint32(payload[0:4])),
		Type: int32(binary.LittleEndian.Uint32(payload[4:8])),
		Body: string(payload[8 : len(payload)-2]),
	}, nil
}
