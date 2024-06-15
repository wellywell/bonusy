package auth

import (
	"net/http"
)

const userCookie = "_user"

func VerifyUser(r *http.Request, secret []byte) (string, error) {
	cookie, err := r.Cookie(userCookie)
	if err == nil {
		user, err := GetUser(cookie.Value, secret)
		if err != nil {
			return user, err
		}
		return user, nil
	}
	return "", err
}

func SetAuthCookie(username string, w http.ResponseWriter, secret []byte, TTLSeconds int) error {

	token, err := BuildJWTString(username, secret)
	if err != nil {
		return err
	}
	cookie := &http.Cookie{Name: userCookie, Value: token, MaxAge: TTLSeconds}
	http.SetCookie(w, cookie)
	return nil
}
