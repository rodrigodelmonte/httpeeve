package httpeeve

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
)

type (
	// Client represents an HTTP client. The "net/http".Client implements it for easy substitution in your project.
	Client interface {
		Do(*http.Request) (*http.Response, error)
	}

	clientFunc func(*http.Request) (*http.Response, error)

	// Conditioner determines whether a response is erroneous and whether to retry it.
	Conditioner func(resp *http.Response) (shouldRetry bool, err error)

	contextKeyAttempts struct{}
)

func (c clientFunc) Do(req *http.Request) (*http.Response, error) {
	return c(req)
}

// NewBackoffClient returns a Client implementation. It takes an implementation of backoff.Backoff,
// which determines the rate and limits of retrying. It takes a Conditioner which determines when to
// stop or continue retrying.
func NewBackoffClient(httpClient http.Client, backoffer backoff.BackOff, conditioner Conditioner) Client {
	return clientFunc(func(req *http.Request) (*http.Response, error) {
		var resp *http.Response
		var attempts int
		err := backoff.Retry(func() error {
			attempts++
			var reqErr error
			resp, reqErr = httpClient.Do(req)
			if reqErr != nil {
				return categorizeRequestError(reqErr)
			}

			var shouldRetry bool
			shouldRetry, reqErr = conditioner(resp)
			if reqErr == nil {
				return nil
			}

			if shouldRetry {
				return reqErr
			}

			return backoff.Permanent(reqErr)
		}, backoffer)

		addAttemptsToRequest(resp, attempts)
		return resp, err
	})
}

func categorizeRequestError(reqErr error) error {
	if strings.Contains(reqErr.Error(), "EOF") {
		return reqErr
	}

	switch specificErr := reqErr.(type) {
	case net.Error:
		switch {
		case specificErr.Timeout(), specificErr.Temporary():
			return reqErr
		default:
			return backoff.Permanent(reqErr)
		}
	default:
		return backoff.Permanent(reqErr)
	}
}

// OK signals that no error occurred and we do not need to retry
func OK() (bool, error) {
	return false, nil
}

// RetriableError signals that a retriable error ocurred
func RetriableError(msg string) (bool, error) {
	return true, errors.New(msg)
}

// RetriableErrorf is like RetriableError but allows string formatting
func RetriableErrorf(msg string, values ...interface{}) (bool, error) {
	return true, fmt.Errorf(msg, values...)
}

// PermanentError signals that an error occurred that cannot be retried
func PermanentError(msg string) (bool, error) {
	return false, errors.New(msg)
}

// PermanentErrorf is like PermanentError but allows string formatting
func PermanentErrorf(msg string, values ...interface{}) (bool, error) {
	return false, fmt.Errorf(msg, values...)
}

// NewDefaultBackoffClient5XX retries requests if they result in 5XXs and accepts them if they result in 2XXs.
// If they are neither they return an error and retry no longer.
func NewDefaultBackoffClient5XX(httpClient http.Client) Client {
	return NewBackoffClient(httpClient, backoff.NewExponentialBackOff(), func(resp *http.Response) (bool, error) {
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			return RetriableErrorf("bad status code %d", resp.StatusCode)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return OK()
		}

		return PermanentErrorf("bad status code %d", resp.StatusCode)
	})
}

// Attempts can be used to tell how many attempts a response took for its execution.
func Attempts(resp *http.Response) int {
	attempts, _ := resp.Request.Context().Value(contextKeyAttempts{}).(int)
	return attempts
}

func addAttemptsToRequest(resp *http.Response, attempts int) {
	resp.Request = resp.Request.WithContext(context.WithValue(resp.Request.Context(), contextKeyAttempts{}, attempts))
}
