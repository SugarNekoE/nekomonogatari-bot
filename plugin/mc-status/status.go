package mcstatus

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"unicode"
)

const defaultPort = 25565
const maxPacketSize = 1 << 20

type serverAddress struct {
	host string
	port uint16
	srv  bool
}

func parseAddress(address string) (serverAddress, error) {
	invalid := errors.New("expected a hostname or IP address, optionally with a port")
	if address == "" || len(address) > 259 || strings.ContainsAny(address, "/\\?#@") || strings.ContainsFunc(address, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return serverAddress{}, invalid
	}
	host, port := address, uint64(defaultPort)
	explicitPort := false
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		host = address[1 : len(address)-1]
		if net.ParseIP(host) == nil {
			return serverAddress{}, invalid
		}
	} else if strings.Contains(address, ":") && net.ParseIP(address) == nil {
		var portText string
		var err error
		host, portText, err = net.SplitHostPort(address)
		if err != nil || len(portText) > 5 {
			return serverAddress{}, invalid
		}
		port, err = strconv.ParseUint(portText, 10, 16)
		if err != nil || port == 0 {
			return serverAddress{}, invalid
		}
		explicitPort = true
	}
	if net.ParseIP(host) == nil {
		domain := strings.TrimSuffix(host, ".")
		if len(domain) == 0 || len(host) > 253 {
			return serverAddress{}, invalid
		}
		for _, label := range strings.Split(domain, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return serverAddress{}, invalid
			}
			for _, r := range label {
				if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
					return serverAddress{}, invalid
				}
			}
		}
	}
	return serverAddress{host: host, port: uint16(port), srv: !explicitPort && net.ParseIP(host) == nil}, nil
}

type status struct {
	Description json.RawMessage `json:"description"`
	Players     *struct {
		Online *int `json:"online"`
		Max    *int `json:"max"`
	} `json:"players"`
}

type statusClient struct {
	lookupSRV func(context.Context, string, string, string) (string, []*net.SRV, error)
	dial      func(context.Context, string, string) (net.Conn, error)
}

func (c statusClient) query(ctx context.Context, address string) (status, error) {
	server, err := parseAddress(address)
	if err != nil {
		return status{}, err
	}
	endpoint := net.JoinHostPort(server.host, strconv.Itoa(int(server.port)))
	if server.srv {
		_, records, err := c.lookupSRV(ctx, "minecraft", "tcp", server.host)
		if err != nil {
			var dnsErr *net.DNSError
			if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
				return status{}, fmt.Errorf("SRV lookup: %w", err)
			}
		} else if len(records) > 0 {
			record := records[0] // Go's resolver orders records by priority and weight.
			if record.Target == "." || record.Port == 0 {
				return status{}, errors.New("Minecraft service is unavailable")
			}
			endpoint = net.JoinHostPort(strings.TrimSuffix(record.Target, "."), strconv.Itoa(int(record.Port)))
		}
	}
	conn, err := c.dial(ctx, "tcp", endpoint)
	if err != nil {
		return status{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return status{}, err
		}
	}
	// Java server-list ping: handshake (protocol -1, status state), then request.
	// Keep the original virtual host in the handshake even when dialing SRV.
	handshake := appendVarInt(nil, 0)
	handshake = appendVarInt(handshake, ^uint32(0))
	handshake = appendVarInt(handshake, uint32(len(server.host)))
	handshake = append(handshake, server.host...)
	handshake = binary.BigEndian.AppendUint16(handshake, server.port)
	handshake = appendVarInt(handshake, 1)
	request := append(appendVarInt(nil, uint32(len(handshake))), handshake...)
	request = append(request, 1, 0)
	if _, err := io.Copy(conn, bytes.NewReader(request)); err != nil {
		return status{}, err
	}
	length, err := readVarInt(conn)
	if err != nil {
		return status{}, err
	}
	if length < 2 || length > maxPacketSize {
		return status{}, errors.New("invalid status packet size")
	}
	packet := make([]byte, int(length))
	if _, err := io.ReadFull(conn, packet); err != nil {
		return status{}, err
	}
	reader := bytes.NewReader(packet)
	id, err := readVarInt(reader)
	if err != nil || id != 0 {
		return status{}, errors.New("invalid status packet ID")
	}
	jsonLength, err := readVarInt(reader)
	if err != nil || int(jsonLength) != reader.Len() {
		return status{}, errors.New("invalid status JSON size")
	}
	var result status
	if err := json.Unmarshal(packet[len(packet)-reader.Len():], &result); err != nil {
		return status{}, fmt.Errorf("decode status: %w", err)
	}
	if len(result.Description) == 0 && result.Players == nil {
		return status{}, errors.New("empty status response")
	}
	return result, nil
}

func appendVarInt(dst []byte, value uint32) []byte {
	for value >= 0x80 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

func readVarInt(reader io.Reader) (uint32, error) {
	var value uint32
	var next [1]byte
	for i := 0; i < 5; i++ {
		if _, err := io.ReadFull(reader, next[:]); err != nil {
			return 0, err
		}
		if i == 4 && next[0] > 0x0f {
			return 0, errors.New("VarInt overflow")
		}
		value |= uint32(next[0]&0x7f) << (7 * i)
		if next[0]&0x80 == 0 {
			return value, nil
		}
	}
	return 0, errors.New("invalid VarInt")
}
