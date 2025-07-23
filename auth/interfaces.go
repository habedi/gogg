package auth

import "github.com/habedi/gogg/db"

// TokenStorer defines the contract for any component that can store and retrieve a token.
type TokenStorer interface {
	GetTokenRecord() (*db.Token, error)
	UpsertTokenRecord(token *db.Token) error
}

// TokenRefresher defines the contract for any component that can perform a token refresh action.
type TokenRefresher interface {
	PerformTokenRefresh(refreshToken string) (accessToken string, newRefreshToken string, expiresIn int64, err error)
}
