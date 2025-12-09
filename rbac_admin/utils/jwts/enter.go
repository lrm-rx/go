package jwts

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"rbac.admin/global"
	"time"
)

type ClaimsUserInfo struct {
	UserID   uint   `json:"userID"`
	Username string `json:"username"`
	RoleList []uint `json:"roleList"`
}

type Claims struct {
	ClaimsUserInfo
	jwt.StandardClaims
}

// GetToken 生成token
func GetToken(info ClaimsUserInfo) (string, error) {
	j := global.Config.Jwt
	cla := Claims{
		ClaimsUserInfo: info,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Duration(j.Expires) * time.Hour).Unix(), // 过期时间
			Issuer:    j.Issuer,                                                    // 签发人
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, cla)
	return token.SignedString([]byte(j.Secret)) // 进行签名生成对应的token
}

// ParseToken 解析token
func ParseToken(tokenString string) (*Claims, error) {
	j := global.Config.Jwt
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(j.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if ok && token.Valid {
		if claims.Issuer != j.Issuer {
			return nil, errors.New("invalid issuer")
		}
		return claims, nil
	}
	return nil, errors.New("invalid token")
}
