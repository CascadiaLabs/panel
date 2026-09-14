package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	pb "github.com/CascadiaLabs/panel/proto/node"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn   *grpc.ClientConn
	client pb.NodeServiceClient
}

// NewClient подключается к ноде. Если certPEM непустой — соединение
// поднимается поверх TLS, и нода обязана предъявить ровно этот сертификат
// (пиннинг: самоподписанный сертификат ноды без внешнего CA).
// Пустой certPEM — обычное незащищённое соединение (для локальных тестов).
func NewClient(addr, token, certPEM string) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transport, err := transportCredentials(certPEM)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(transport),
		grpc.WithPerRPCCredentials(NewBearerToken(token)),
	)
	if err != nil {
		return nil, err
	}

	c := &Client{
		conn:   conn,
		client: pb.NewNodeServiceClient(conn),
	}
	return c, nil
}

// transportCredentials строит TLS-учётные данные с пиннингом сертификата
// или insecure-учётные данные, если сертификат не задан.
func transportCredentials(certPEM string) (credentials.TransportCredentials, error) {
	if certPEM == "" {
		return insecure.NewCredentials(), nil
	}

	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("cert_pem: не удалось разобрать PEM (ожидается сертификат ноды целиком, включая -----BEGIN CERTIFICATE-----)")
	}
	pinned, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cert_pem: %w", err)
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Пиннинг: имя хоста в сертификате не сверяется — вместо этого
		// сверяется сам сертификат с тем, что сохранён в панели.
		InsecureSkipVerify: true, //nolint:gosec // проверка выполняется в VerifyPeerCertificate
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("нода не предъявила сертификат (нода работает без TLS?)")
			}
			peer, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("сертификат ноды: %w", err)
			}
			if !peer.Equal(pinned) {
				return errors.New("сертификат ноды не совпадает с сохранённым в панели (cert_pem)")
			}
			// InsecureSkipVerify also skips validity checks; enforce them on every handshake.
			now := time.Now()
			if now.Before(peer.NotBefore) || now.After(peer.NotAfter) {
				return errors.New("сертификат ноды ещё не действителен или истёк")
			}
			return nil
		},
	}
	return credentials.NewTLS(tlsCfg), nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) UpdateConfig(ctx context.Context, configJSON string) (*pb.UpdateConfigResponse, error) {
	return c.client.UpdateConfig(ctx, &pb.UpdateConfigRequest{ConfigJson: configJSON})
}

func (c *Client) GetConfig(ctx context.Context) (string, error) {
	resp, err := c.client.GetConfig(ctx, &pb.GetConfigRequest{})
	if err != nil {
		return "", err
	}
	return resp.GetConfigJson(), nil
}

func (c *Client) GetStatus(ctx context.Context) (Status, error) {
	resp, err := c.client.GetStatus(ctx, &pb.GetStatusRequest{})
	if err != nil {
		return Status{}, err
	}
	return Status{
		Running:        resp.GetRunning(),
		SingboxVersion: resp.GetSingboxVersion(),
		UptimeSeconds:  resp.GetUptimeSeconds(),
		Inbounds:       resp.GetInbounds(),
		Outbounds:      resp.GetOutbounds(),
		LastUpdateUnix: resp.GetLastUpdateUnix(),
	}, nil
}

func (c *Client) PushConfig(ctx context.Context, configJSON string) error {
	resp, err := c.client.UpdateConfig(ctx, &pb.UpdateConfigRequest{ConfigJson: configJSON})
	if err != nil {
		return err
	}
	if !resp.GetOk() {
		return fmt.Errorf("node returned ok=false")
	}
	return nil
}
