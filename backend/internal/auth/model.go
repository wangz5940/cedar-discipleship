package auth

import "agp/backend/internal/user"

type Session struct {
	Token string
	User  user.UserVO
}

type Claims struct {
	UserID         uint64
	CurrentGroupID uint64
	ExpiresAt      int64
}
