package httpeeve

import (
	"context"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type (
	Client interface {
		Do(*http.Request) ([]byte, int, error)
	}

	clientFunc func(*http.Request) ([]byte, int, error)

	backoffConditioner func(*http.Response) bool
)

func (c clientFunc) Do(req *http.Request) ([]byte, int, error) {
	return c(req)
}

func NewBackoffClient(httpClient http.Client, shouldRetry backoffConditioner) Client {
	return clientFunc(func(req *http.Request) ([]byte, int, error) {
		backoffer := backoff.NewExponentialBackOff() // // i'm happy with the defaults for now
		logger := zerolog.Ctx(req.Context()).With().Str("_url", req.URL.String()).Logger()
		ctx := logger.WithContext(req.Context())

		var body []byte
		var code int
		err := backoff.Retry(func() error {
			resp, reqErr := httpClient.Do(req)
			if reqErr != nil {
				return categorizeRequestError(ctx, reqErr)
			}
			code = resp.StatusCode

			defer resp.Body.Close()

			var readErr error
			body, readErr = ioutil.ReadAll(resp.Body)
			if readErr != nil {
				return readErr
			}

			if code == http.StatusOK {
				return nil
			}

			reqErr = errors.Errorf("received unexpected status code: %d", resp.StatusCode)

			if shouldRetry(resp) {
				logger.Debug().Err(reqErr).Msg("retrying request")
				return reqErr
			}

			return backoff.Permanent(reqErr)
		}, backoffer)

		return body, code, err
	})
}

func categorizeRequestError(ctx context.Context, reqErr error) error {
	logger := zerolog.Ctx(ctx)

	if strings.Contains(reqErr.Error(), "EOF") {
		logger.Debug().Err(reqErr).Msg("EOF err, retrying...")
		return reqErr
	}

	switch specificErr := reqErr.(type) {
	case net.Error:
		if specificErr.Timeout() {
			logger.Debug().Err(specificErr).Msg("timeout err, retrying...")
			return reqErr
		} else if specificErr.Temporary() {
			logger.Debug().Err(specificErr).Msg("temporary err, retrying...")
			return reqErr
		} else {
			logger.Debug().Err(reqErr).Msg("permanent error")
			return backoff.Permanent(reqErr)
		}
	default:
		logger.Debug().Err(reqErr).Msg("permanent error")
		return backoff.Permanent(reqErr)
	}
}

func NewDefaultBackoffClient5XX(httpClient http.Client) Client {
	return NewBackoffClient(httpClient, func(resp *http.Response) bool {
		return resp.StatusCode >= 500 && resp.StatusCode < 600
	})
}
