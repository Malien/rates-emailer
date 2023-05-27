package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/httplog"
	"io"
	"net/http"
)

type RateFetcher interface {
	FetchRate(ctx context.Context) (float64, error)
}

type fetcherKey int

var FetcherCtxKey fetcherKey

func FetcherFromContext(ctx context.Context) RateFetcher {
	return ctx.Value(FetcherCtxKey).(RateFetcher)
}

type GeckoAPI struct{}

func (g GeckoAPI) FetchRate(ctx context.Context) (float64, error) {
	oplog := httplog.LogEntry(ctx)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd",
		nil,
	)
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("Failed to fetch exchange rates, with an HTTP code of %s", resp.Status)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	oplog.Trace().Bytes("body", bytes).Msg("CoinGecko /simple/price response body")

	var cgResp *CoinGeckoResponse
	err = json.Unmarshal(bytes, &cgResp)
	if err != nil {
		return 0, err
	}

	return cgResp.Bitcoin.Usd, nil
}
