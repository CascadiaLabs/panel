package node

import "context"

type BearerToken struct {
	token string
}

func NewBearerToken(token string) *BearerToken {
	return &BearerToken{token: token}
}

func (b *BearerToken) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b *BearerToken) RequireTransportSecurity() bool {
	return false
}
