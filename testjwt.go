package main
import (
	"fmt"
	"time"
	"encoding/base64"
	"crypto/ed25519"
	"github.com/golang-jwt/jwt/v5"
)
func main() {
	keyStr := "nJAJSkhfwgFJIu6JioVyCad3nSHri7H3Uwljze9U8VmPEonfmsqhILMFoy2TJOF4WrsLf7nNIK78gTJa52IygA=="
	k, _ := base64.StdEncoding.DecodeString(keyStr)
	priv := ed25519.PrivateKey(k)
	claims := jwt.MapClaims{
		"sub": "70b1b4ca-9d5e-40cf-b9be-15403d71a201",
		"role": "owner",
		"iss": "mujeeb24",
		"aud": "mujeeb24",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour * 24).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	s, _ := t.SignedString(priv)
	fmt.Println(s)
}
