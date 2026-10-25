package model

import (
	"context"
	"net/http"

	"github.com/aalvaropc/upkeep/internal/config"
	"github.com/aalvaropc/upkeep/internal/stub"
)

type TokenFactory struct {
	BaseURL string
	Key     config.Secret
	HTTP    *http.Client
}

func (t *TokenFactory) Complete(_ context.Context, _ Request) (Response, error) {
	return Response{}, stub.Err("token factory completion")
}
