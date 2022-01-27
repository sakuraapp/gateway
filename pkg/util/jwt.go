package util

import (
	"crypto/rsa"
	"fmt"
	"github.com/golang-jwt/jwt"
)

type JWT struct {
	PublicKey *rsa.PublicKey
}

func (j *JWT) Parse(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return j.PublicKey, nil
	})

	claims, ok := token.Claims.(jwt.MapClaims)

	if !ok || !token.Valid {
		return nil, err
	} else {
		return claims, nil
	}
}