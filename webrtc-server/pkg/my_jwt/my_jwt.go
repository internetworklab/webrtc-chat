package my_jwt

import "context"

type JWTManager interface {
	Issue(ctx context.Context, userid string) (string, error)
	Validate(ctx context.Context, token string) (bool, error)
}
