package node

import (
	"net/http"

	"github.com/theQRL/qrysm/beacon-chain/rpc/zond/helpers"
)

func middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		helpers.NormalizeQueryValues(query)
		r.URL.RawQuery = query.Encode()

		next.ServeHTTP(w, r)
	})
}
