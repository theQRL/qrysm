package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
)

type jsonRestHandler interface {
	GetRestJsonResponse(ctx context.Context, query string, responseJson any) (*apimiddleware.DefaultErrorJson, error)
	PostRestJson(ctx context.Context, apiEndpoint string, headers map[string]string, data *bytes.Buffer, responseJson any) (*apimiddleware.DefaultErrorJson, error)
}

type beaconApiJsonRestHandler struct {
	timeout time.Duration
	host    string
	hostSet *beaconAPIHostSet
}

type beaconAPIHostSet struct {
	mu         sync.RWMutex
	hosts      []string
	activeHost int
}

func newBeaconAPIHostSet(host string) *beaconAPIHostSet {
	hosts := splitBeaconAPIHosts(host)
	if len(hosts) == 0 {
		hosts = []string{""}
	}
	return &beaconAPIHostSet{hosts: hosts}
}

func splitBeaconAPIHosts(host string) []string {
	parts := strings.Split(host, ",")
	hosts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		hosts = append(hosts, part)
	}
	return hosts
}

func (s *beaconAPIHostSet) orderedHosts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.hosts) == 0 {
		return nil
	}

	ordered := make([]string, 0, len(s.hosts))
	for i := 0; i < len(s.hosts); i++ {
		idx := (s.activeHost + i) % len(s.hosts)
		ordered = append(ordered, s.hosts[idx])
	}
	return ordered
}

func (s *beaconAPIHostSet) promote(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for idx, candidate := range s.hosts {
		if candidate == host {
			s.activeHost = idx
			return
		}
	}
}

func newBeaconAPIJSONRestHandler(host string, timeout time.Duration) beaconApiJsonRestHandler {
	return beaconApiJsonRestHandler{
		timeout: timeout,
		host:    host,
		hostSet: newBeaconAPIHostSet(host),
	}
}

func (c beaconApiJsonRestHandler) effectiveHostSet() *beaconAPIHostSet {
	if c.hostSet != nil {
		return c.hostSet
	}
	return newBeaconAPIHostSet(c.host)
}

func (c beaconApiJsonRestHandler) requestContext(ctx context.Context, hostCount int) (context.Context, context.CancelFunc) {
	if hostCount <= 1 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			return context.WithTimeout(ctx, c.timeout)
		}
		return ctx, func() {}
	}

	if c.timeout <= 0 {
		return ctx, func() {}
	}

	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx, func() {}
		}
		if remaining < c.timeout {
			return context.WithTimeout(ctx, remaining)
		}
	}

	return context.WithTimeout(ctx, c.timeout)
}

func (c beaconApiJsonRestHandler) doGetRestJsonResponse(ctx context.Context, host, apiEndpoint string, responseJson any) (*apimiddleware.DefaultErrorJson, error) {
	url := host + apiEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request with context")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to query REST API %s", url)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			return
		}
	}()

	return decodeJsonResp(resp, responseJson)
}

func (c beaconApiJsonRestHandler) doPostRestJson(
	ctx context.Context,
	host string,
	apiEndpoint string,
	headers map[string]string,
	data []byte,
	responseJson any,
) (*apimiddleware.DefaultErrorJson, error) {
	url := host + apiEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request with context")
	}

	for headerKey, headerValue := range headers {
		req.Header.Set(headerKey, headerValue)
	}
	req.Header.Set("Content-Type", api.JsonMediaType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to send POST data to REST endpoint %s", url)
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			return
		}
	}()

	return decodeJsonResp(resp, responseJson)
}

// GetRestJsonResponse sends a GET requests to apiEndpoint and decodes the response body as a JSON object into responseJson.
// If an HTTP error is returned, the body is decoded as a DefaultErrorJson JSON object instead and returned as the first return value.
// TODO: GetRestJsonResponse and PostRestJson have converged to the point of being nearly identical, but with some inconsistencies
// (like responseJson is being checked for nil one but not the other). We should merge them into a single method
// with variadic functional options for headers and data.
func (c beaconApiJsonRestHandler) GetRestJsonResponse(ctx context.Context, apiEndpoint string, responseJson any) (*apimiddleware.DefaultErrorJson, error) {
	if responseJson == nil {
		return nil, errors.New("responseJson is nil")
	}

	hostSet := c.effectiveHostSet()
	hosts := hostSet.orderedHosts()

	var (
		errorJSON *apimiddleware.DefaultErrorJson
		err       error
	)

	for _, host := range hosts {
		reqCtx, cancel := c.requestContext(ctx, len(hosts))
		errorJSON, err = c.doGetRestJsonResponse(reqCtx, host, apiEndpoint, responseJson)
		cancel()
		if err == nil {
			hostSet.promote(host)
			return errorJSON, nil
		}
		if ctx.Err() != nil {
			break
		}
	}

	return errorJSON, err
}

// PostRestJson sends a POST requests to apiEndpoint and decodes the response body as a JSON object into responseJson. If responseJson
// is nil, nothing is decoded. If an HTTP error is returned, the body is decoded as a DefaultErrorJson JSON object instead and returned
// as the first return value.
func (c beaconApiJsonRestHandler) PostRestJson(ctx context.Context, apiEndpoint string, headers map[string]string, data *bytes.Buffer, responseJson any) (*apimiddleware.DefaultErrorJson, error) {
	if data == nil {
		return nil, errors.New("POST data is nil")
	}

	hostSet := c.effectiveHostSet()
	hosts := hostSet.orderedHosts()
	payload := data.Bytes()

	var (
		errorJSON *apimiddleware.DefaultErrorJson
		err       error
	)

	for _, host := range hosts {
		reqCtx, cancel := c.requestContext(ctx, len(hosts))
		errorJSON, err = c.doPostRestJson(reqCtx, host, apiEndpoint, headers, payload, responseJson)
		cancel()
		if err == nil {
			hostSet.promote(host)
			return errorJSON, nil
		}
		if ctx.Err() != nil {
			break
		}
	}

	return errorJSON, err
}

func decodeJsonResp(resp *http.Response, responseJson any) (*apimiddleware.DefaultErrorJson, error) {
	if resp.StatusCode != http.StatusOK {
		// Read the body up-front so we can surface it verbatim if it isn't
		// JSON (e.g. an HTML 502 from a reverse proxy in front of the beacon
		// node). Otherwise operators see "failed to decode error json" with
		// no status code or body.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read error response body for %s", resp.Request.URL)
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		errorJson := &apimiddleware.DefaultErrorJson{}
		if err := decoder.Decode(errorJson); err != nil {
			return nil, errors.Errorf("HTTP request for %s unsuccessful (%d: %s)", resp.Request.URL, resp.StatusCode, string(body))
		}

		return errorJson, errors.Errorf("error %d: %s", errorJson.Code, errorJson.Message)
	}

	if responseJson != nil {
		decoder := json.NewDecoder(resp.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(responseJson); err != nil {
			return nil, errors.Wrapf(err, "failed to decode response json for %s", resp.Request.URL)
		}
	}

	return nil, nil
}
