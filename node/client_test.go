package node

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	pb "github.com/CascadiaLabs/panel/proto/node"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func testCertificate(t *testing.T, notBefore, notAfter time.Time) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: notBefore, NotAfter: notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		// No SAN: exact leaf pinning intentionally does not require a hostname match.
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

type testNodeServer struct {
	pb.UnimplementedNodeServiceServer
}

func (testNodeServer) GetStatus(context.Context, *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	return &pb.GetStatusResponse{Running: true}, nil
}

func TestClientTransport(t *testing.T) {
	now := time.Now()
	valid, pin := testCertificate(t, now.Add(-time.Hour), now.Add(time.Hour))
	_, otherPin := testCertificate(t, now.Add(-time.Hour), now.Add(time.Hour))
	expired, expiredPin := testCertificate(t, now.Add(-2*time.Hour), now.Add(-time.Hour))
	future, futurePin := testCertificate(t, now.Add(time.Hour), now.Add(2*time.Hour))

	for _, tt := range []struct {
		name    string
		cert    tls.Certificate
		pin     string
		token   string
		version uint16
		want    codes.Code
	}{
		{"TLS12 correct pin and token", valid, pin, "test-token", tls.VersionTLS12, codes.OK},
		{"TLS13 correct pin and token", valid, pin, "test-token", tls.VersionTLS13, codes.OK},
		{"different pin", valid, otherPin, "test-token", tls.VersionTLS12, codes.Unavailable},
		{"expired pin", expired, expiredPin, "test-token", tls.VersionTLS12, codes.Unavailable},
		{"future pin", future, futurePin, "test-token", tls.VersionTLS12, codes.Unavailable},
		{"wrong token", valid, pin, "wrong", tls.VersionTLS12, codes.Unauthenticated},
		{"TLS11 rejected", valid, pin, "test-token", tls.VersionTLS11, codes.Unavailable},
		{"plaintext compatibility", tls.Certificate{}, "", "test-token", 0, codes.OK},
		{"no plaintext fallback", tls.Certificate{}, pin, "test-token", 0, codes.Unavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			opts := []grpc.ServerOption{grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
				md, _ := metadata.FromIncomingContext(ctx)
				values := md.Get("authorization")
				if len(values) != 1 || values[0] != "Bearer test-token" {
					return nil, status.Error(codes.Unauthenticated, "invalid token")
				}
				return handler(ctx, req)
			})}
			if tt.version != 0 {
				opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{
					Certificates: []tls.Certificate{tt.cert},
					MinVersion:   tt.version, MaxVersion: tt.version,
				})))
			}
			srv := grpc.NewServer(opts...)
			pb.RegisterNodeServiceServer(srv, testNodeServer{})
			go func() { _ = srv.Serve(listener) }()
			t.Cleanup(srv.Stop)

			client, err := NewClient(listener.Addr().String(), tt.token, tt.pin)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			got, err := client.GetStatus(ctx)
			if status.Code(err) != tt.want {
				t.Fatalf("GetStatus code = %v, want %v", status.Code(err), tt.want)
			}
			if tt.want == codes.OK && !got.Running {
				t.Fatal("successful RPC did not return server status")
			}
		})
	}
}

func TestClientRejectsInvalidPEM(t *testing.T) {
	_, valid := testCertificate(t, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	block, _ := pem.Decode([]byte(valid))
	block.Type = "PRIVATE KEY"
	for name, input := range map[string]string{
		"garbage":        "not a certificate",
		"whitespace":     " \n\t",
		"invalid DER":    string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")})),
		"wrong PEM type": string(pem.EncodeToMemory(block)),
	} {
		t.Run(name, func(t *testing.T) {
			client, err := NewClient("127.0.0.1:1", "test-token", input)
			if client != nil {
				_ = client.Close()
			}
			if err == nil || client != nil {
				t.Fatal("invalid PEM must fail before connecting")
			}
		})
	}
}
