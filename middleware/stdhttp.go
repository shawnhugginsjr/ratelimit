package stdlib

import (
	"net/http"
	"strconv"

	"github.com/shawnhugginsjr/ratelimit"
)

type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
type LimitReachedHandler func(w http.ResponseWriter, r *http.Request)
type IDHandler func(r *http.Request) (string, error)

// Middleware is the middleware for basic http.Handler.
type Middleware struct {
	Limiter        *ratelimit.Limiter
	GetID          IDHandler
	OnIDError      ErrorHandler
	OnLimitReached LimitReachedHandler
}

// NewMiddleware return a new instance of a basic HTTP middleware.
func NewMiddleware(limiter *ratelimit.Limiter, getID IDHandler, onIDError ErrorHandler) *Middleware {
	middleware := &Middleware{
		Limiter:   limiter,
		GetID:     getID,
		OnIDError: onIDError,
	}

	return middleware
}

// Handler handles a HTTP request.
func (middleware *Middleware) Handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := middleware.GetID(r)
		if err != nil {
			middleware.OnIDError(w, r, err)
			return
		}
		lr, err := middleware.Limiter.RecordRequest(r.Context(), key)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Add("X-RateLimit-Limit", strconv.FormatInt(lr.Limit, 10))
		w.Header().Add("X-RateLimit-Remaining", strconv.FormatInt(lr.Remaining, 10))
		w.Header().Add("X-RateLimit-Reset", strconv.FormatInt(lr.SecondsRemaining(), 10))

		if lr.LimitReached {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		h.ServeHTTP(w, r)
	})
}
