package auth

import (
	"context"

	"agp/backend/internal/user"
)

type TokenService interface {
	Sign(ctx context.Context, claims Claims) (string, error)
	Verify(ctx context.Context, token string) (Claims, error)
}

type Service struct {
	users  user.Repository
	tokens TokenService
}

func NewService(users user.Repository, tokens TokenService) *Service {
	return &Service{users: users, tokens: tokens}
}
