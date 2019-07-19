package httpeeve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
)

var backoffer = backoff.NewExponentialBackOff()

func TestRequestRetries(t *testing.T) {
	client := NewBackoffClient(http.Client{}, backoffer, func(resp *http.Response) (bool, error) {
		if resp.StatusCode == 500 {
			return true, errors.New("bad")
		}
		return false, nil
	})

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		} else {
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 2, requestCount)
	assert.Equal(t, 2, Attempts(resp))
}

func TestRequestNoRetryOn200(t *testing.T) {
	client := NewBackoffClient(http.Client{}, backoffer, func(resp *http.Response) (bool, error) {
		if resp.StatusCode == 500 {
			return RetriableError("bad")
		}
		return OK()
	})

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		return
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, requestCount)
}

func TestRequestReturnsErrImmediately(t *testing.T) {
	client := NewBackoffClient(http.Client{}, backoffer, func(resp *http.Response) (bool, error) {
		if resp.StatusCode == 404 {
			return PermanentError("bad")
		}
		return OK()
	})

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusNotFound)
		return
	}))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)

	resp, err := client.Do(req)
	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, 1, requestCount)
	assert.EqualError(t, err, "bad")
}
