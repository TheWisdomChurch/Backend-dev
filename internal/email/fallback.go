package email

import "errors"

// FallbackError marks a provider error as safe for an immediate secondary
// provider attempt because the primary provider definitively did not accept
// the message. Ambiguous transport failures must remain unmarked to avoid
// duplicate delivery.
type FallbackError struct{ cause error }

func (e *FallbackError) Error() string { return e.cause.Error() }
func (e *FallbackError) Unwrap() error { return e.cause }

// MarkFallbackEligible wraps an error known to represent a definite rejection.
func MarkFallbackEligible(err error) error {
	if err == nil {
		return nil
	}
	return &FallbackError{cause: err}
}

// CanFallback reports whether retrying the same message through another
// provider is safe based on a definitive rejection from the first provider.
func CanFallback(err error) bool {
	var eligible *FallbackError
	return errors.As(err, &eligible)
}
