package main

import (
	"encoding/json"
	"io"
	"net/http"
)

func decodeJSON(resp *http.Response, dest any) error {
	defer io.Copy(io.Discard, resp.Body)
	return json.NewDecoder(resp.Body).Decode(dest)
}
