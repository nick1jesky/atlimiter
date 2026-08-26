package atlimiter

import "errors"

// ErrExceedsCapacity is returned by WaitN when the requested token count can
// never be satisfied because it exceeds the limiter's total capacity.
var ErrExceedsCapacity = errors.New("atlimiter: requested tokens exceed capacity")
