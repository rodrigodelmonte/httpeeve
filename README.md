# HTTPeeve

HTTPeeve is a library for making HTTP requests with exponential backoff. It wraps around <https://github.com/cenkalti/backoff> 
and provides some helper functions.


## Installation

```sh
go get github.com/Onefootball/httpeeve
```

## Usage

An example of how to use the library can be found in the helper function `NewDefaultBackoffClient5XX`:

```go
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
```

This function takes a `"net/http".Client`. It initializes a `NewBackoffClient` with this, as well as
an instance of `"cenkalti/backoff".Backoff` and a `Conditioner`.

In the example, the `Conditioner` determines that 5XX status codes can be retried, 2XXs are OK, and everything else 
results in an unretriable error.

This library also provides a helper function to see how many attempts a request took:

```go
resp, err := client.Do(req)
if err != nil {
    // something
}
fmt.Println(httpeeve.Attempts(resp))
```
