package ratelimit

// The LimitRecord stores data concerning a request limit.
type LimitRecord struct {
	Limit        int64
	Remainder    int64
	LimitReached bool
}

// The Limiter rate limits an IPAdress
type Limiter struct {
	Store Store // Store managing rate limits
	Rate  Rate  //  Rate to use for this Limmiter
}
