package my_jwt

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTManager interface {
	Issue(ctx context.Context, userid string) (string, error)
	Validate(ctx context.Context, token string) (bool, error)
}

const defaultSecretLengthBytes int = 32
const defaultTokenLifeSpanSecs int = 7 * 24 * 60 * 60

type SimpleJWTManager struct {
	secret []byte
}

func NewSimpleJWTManager(secret []byte) *SimpleJWTManager {
	secMng := &SimpleJWTManager{secret: secret}
	if secret == nil {
		secMng.secret = make([]byte, defaultSecretLengthBytes)
		if _, err := rand.Read(secMng.secret); err != nil {
			log.Fatal(errors.New("failed to initialize simple jwt manager"))
		}
	}
	return secMng
}

func (m *SimpleJWTManager) Issue(ctx context.Context, userid string) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userid,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(defaultTokenLifeSpanSecs) * time.Second)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *SimpleJWTManager) Validate(ctx context.Context, tokenString string) (bool, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return m.secret, nil
	})

	if err != nil {
		return false, err
	}

	return token.Valid, nil
}
