package httpeeve

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
)

type (
	Client interface {
		Do(*http.Request) (*http.Response, error)
	}

	clientFunc func(*http.Request) (*http.Response, error)

	backoffConditioner func(resp *http.Response) (shouldRetry bool, err error)

	contextKeyAttempts struct{}
)

func (c clientFunc) Do(req *http.Request) (*http.Response, error) {
	return c(req)
}

func NewBackoffClient(httpClient http.Client, backoffer backoff.BackOff, conditioner backoffConditioner) Client {
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

			defer resp.Body.Close()

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
		if specificErr.Timeout() {
			return reqErr
		} else if specificErr.Temporary() {
			return reqErr
		} else {
			return backoff.Permanent(reqErr)
		}
	default:
		return backoff.Permanent(reqErr)
	}
}

// NewDefaultBackoffClient5XX retries
func NewDefaultBackoffClient5XX(httpClient http.Client) Client {
	return NewBackoffClient(httpClient, backoff.NewExponentialBackOff(), func(resp *http.Response) (bool, error) {
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			return true, errors.Errorf("bad status code %d", resp.StatusCode)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return false, nil
		}

		return false, errors.Errorf("bad status code %d", resp.StatusCode)
	})
}

func Attempts(resp *http.Response) int {
	attempts, _ := resp.Request.Context().Value(contextKeyAttempts{}).(int)
	return attempts
}

func addAttemptsToRequest(resp *http.Response, attempts int) {
	resp.Request = resp.Request.WithContext(context.WithValue(resp.Request.Context(), contextKeyAttempts{}, attempts))
}
