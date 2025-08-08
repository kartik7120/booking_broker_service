package utils

import (
	"net/http"
	"os"
)

func RedirectToHttpMiddleware(next http.Handler) http.Handler {
	testEnv := os.Getenv("ENV") // You could also use "TEST_ENV" if that's your key
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if testEnv == "test" && r.TLS != nil {
			// Reconstruct the HTTP URL
			target := "http://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		}
		next.ServeHTTP(w, r)
	})
}
