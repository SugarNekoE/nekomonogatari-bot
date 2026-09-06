package mcstatus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseAddress(t *testing.T) {
	for _, test := range []struct {
		input, host string
		port        uint16
		srv         bool
	}{
		{"play.example.com", "play.example.com", 25565, true},
		{"play.example.com.", "play.example.com.", 25565, true},
		{"play.example.com:25566", "play.example.com", 25566, false},
		{"play.example.com:25565", "play.example.com", 25565, false},
		{"127.0.0.1", "127.0.0.1", 25565, false},
		{"::1", "::1", 25565, false},
		{"[::1]", "::1", 25565, false},
		{"[2001:db8::1]:25566", "2001:db8::1", 25566, false},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := parseAddress(test.input)
			if err != nil || got.host != test.host || got.port != test.port || got.srv != test.srv {
				t.Fatalf("parseAddress = %#v, %v", got, err)
			}
		})
	}
	for _, input := range []string{"", "https://example.com", "host:0", "host:65536", "host:abc", "host:", ":25565", "a b", "a\nb", "user@host", "host/path", "a..b", "-host", "[host]", "host#fragment", "host:000001", "host:" + strings.Repeat("0", 3000) + "25565"} {
		if _, err := parseAddress(input); err == nil {
			t.Errorf("accepted invalid address %q", input)
		}
	}
}

func TestQuerySRVAndHandshake(t *testing.T) {
	for _, test := range []struct {
		name, address, endpoint string
		records                 []*net.SRV
		dnsErr                  error
		lookup                  bool
	}{
		{"srv", "play.test", "node.test:25570", []*net.SRV{{Target: "node.test.", Port: 25570}}, nil, true},
		{"missing", "play.test", "play.test:25565", nil, &net.DNSError{IsNotFound: true}, true},
		{"empty", "play.test", "play.test:25565", nil, nil, true},
		{"explicit", "play.test:25565", "play.test:25565", nil, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			lookups := 0
			serverDone := make(chan error, 1)
			client := statusClient{
				lookupSRV: func(ctx context.Context, service, proto, host string) (string, []*net.SRV, error) {
					lookups++
					if service != "minecraft" || proto != "tcp" || host != "play.test" {
						t.Errorf("unexpected SRV lookup %s %s %s", service, proto, host)
					}
					return "", test.records, test.dnsErr
				},
				dial: func(ctx context.Context, network, endpoint string) (net.Conn, error) {
					if network != "tcp" || endpoint != test.endpoint {
						t.Errorf("dial %s %s", network, endpoint)
					}
					local, remote := net.Pipe()
					go func() {
						defer remote.Close()
						// Exact wire bytes for original host play.test, port 25565,
						// protocol -1, status state, followed by an empty request.
						want := append([]byte{19, 0, 255, 255, 255, 255, 15, 9}, []byte("play.test")...)
						want = append(want, 0x63, 0xdd, 1, 1, 0)
						got := make([]byte, len(want))
						if _, err := io.ReadFull(remote, got); err != nil {
							serverDone <- err
							return
						}
						if !bytes.Equal(got, want) {
							serverDone <- errors.New("incorrect handshake/request")
							return
						}
						payload := []byte(`{"description":{"text":"§aHello","extra":[{"text":"\nworld"}]},"players":{"online":3,"max":20}}`)
						_, err := remote.Write(responsePacket(payload))
						serverDone <- err
					}()
					return local, nil
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			got, err := client.query(ctx, test.address)
			if err != nil {
				t.Fatal(err)
			}
			if got.Players == nil || *got.Players.Online != 3 || *got.Players.Max != 20 {
				t.Fatalf("players = %#v", got.Players)
			}
			var component any
			if err := json.Unmarshal(got.Description, &component); err != nil {
				t.Fatal(err)
			}
			if motd := plainText(componentText(component, 0), 100); motd != "Hello\nworld" {
				t.Fatalf("MOTD = %q", motd)
			}
			if (lookups == 1) != test.lookup {
				t.Fatalf("lookups = %d", lookups)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func responsePacket(payload []byte) []byte {
	packet := append([]byte{0}, appendVarInt(nil, uint32(len(payload)))...)
	packet = append(packet, payload...)
	return append(appendVarInt(nil, uint32(len(packet))), packet...)
}

func TestQueryRejectsBadResponses(t *testing.T) {
	for _, test := range []struct {
		name     string
		response []byte
	}{
		{"overflow", []byte{255, 255, 255, 255, 127}},
		{"oversize", appendVarInt(nil, maxPacketSize+1)},
		{"empty frame", []byte{0}},
		{"truncated", []byte{10, 0}},
		{"wrong ID", []byte{2, 1, 0}},
		{"wrong string length", []byte{3, 0, 5, 'a'}},
		{"bad JSON", responsePacket([]byte("oops"))},
		{"empty status", responsePacket([]byte(`{}`))},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := pipeClient(func(conn net.Conn) { _, _ = conn.Write(test.response) })
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, err := client.query(ctx, "127.0.0.1"); err == nil {
				t.Fatal("accepted malformed response")
			}
		})
	}
}

// Consume one handshake frame and the status request before returning a response.
func pipeClient(respond func(net.Conn)) statusClient {
	return statusClient{dial: func(context.Context, string, string) (net.Conn, error) {
		local, remote := net.Pipe()
		go func() {
			defer remote.Close()
			size, err := readVarInt(remote)
			if err != nil {
				return
			}
			if _, err := io.CopyN(io.Discard, remote, int64(size)+2); err != nil {
				return
			}
			respond(remote)
		}()
		return local, nil
	}}
}

func TestQueryTimeoutAndCancellation(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		client := pipeClient(func(conn net.Conn) {
			if cancelEarly {
				cancel()
			}
			_, _ = conn.Read(make([]byte, 1))
		})
		started := time.Now()
		_, err := client.query(ctx, "127.0.0.1")
		cancel()
		if err == nil || time.Since(started) > time.Second {
			t.Fatalf("query did not stop promptly: %v", err)
		}
	}
}

func TestQuerySRVFailuresDoNotDial(t *testing.T) {
	for _, test := range []struct {
		records []*net.SRV
		err     error
	}{
		{err: &net.DNSError{IsTimeout: true}},
		{records: []*net.SRV{{Target: ".", Port: 25565}}},
		{records: []*net.SRV{{Target: "node.test", Port: 0}}},
	} {
		client := statusClient{
			lookupSRV: func(context.Context, string, string, string) (string, []*net.SRV, error) {
				return "", test.records, test.err
			},
			dial: func(context.Context, string, string) (net.Conn, error) {
				t.Error("dial after SRV failure")
				return nil, errors.New("unexpected dial")
			},
		}
		if _, err := client.query(context.Background(), "play.test"); err == nil {
			t.Fatal("expected SRV error")
		}
	}
}

func TestVarIntRejectsTruncation(t *testing.T) {
	if _, err := readVarInt(strings.NewReader("\x80")); err == nil {
		t.Fatal("accepted truncated VarInt")
	}
}
