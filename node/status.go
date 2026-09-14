package node

import (
	"context"
	"time"
)

type Status struct {
	Running        bool   `json:"running"`
	SingboxVersion string `json:"singbox_version"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
	Inbounds       int32  `json:"inbounds"`
	Outbounds      int32  `json:"outbounds"`
	LastUpdateUnix int64  `json:"last_update_unix"`
}

func FetchStatus(ctx context.Context, addr, token, certPEM string) (Status, error) {
	client, err := NewClient(addr, token, certPEM)
	if err != nil {
		return Status{}, err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return client.GetStatus(ctx)
}

func FetchConfig(ctx context.Context, addr, token, certPEM string) (string, error) {
	client, err := NewClient(addr, token, certPEM)
	if err != nil {
		return "", err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return client.GetConfig(ctx)
}

func PushConfig(ctx context.Context, addr, token, certPEM, configJSON string) error {
	client, err := NewClient(addr, token, certPEM)
	if err != nil {
		return err
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return client.PushConfig(ctx, configJSON)
}
