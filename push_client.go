package expo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	// DefaultHost is the default Expo host
	DefaultHost = "https://exp.host"
	// DefaultBaseAPIURL is the default path for API requests
	DefaultBaseAPIURL = "/--/api/v2"
)

// DefaultHTTPClient is the default *http.Client for making API requests
var DefaultHTTPClient = &http.Client{}

// PushClient is an object used for making push notification requests
type PushClient struct {
	host        string
	apiURL      string
	accessToken string
	httpClient  *http.Client
}

// ClientConfig specifies params that can optionally be specified for alternate
// Expo config and path setup when sending API requests
type ClientConfig struct {
	Host        string
	APIURL      string
	AccessToken string
	HTTPClient  *http.Client
}

// NewPushClient creates a new Exponent push client
// See full API docs at https://docs.expo.dev/push-notifications/sending-notifications/
func NewPushClient(config *ClientConfig) *PushClient {
	c := &PushClient{
		host:        DefaultHost,
		apiURL:      DefaultBaseAPIURL,
		httpClient:  DefaultHTTPClient,
		accessToken: "",
	}
	if config != nil {
		if config.Host != "" {
			c.host = config.Host
		}
		if config.APIURL != "" {
			c.apiURL = config.APIURL
		}
		if config.AccessToken != "" {
			c.accessToken = config.AccessToken
		}
		if config.HTTPClient != nil {
			c.httpClient = config.HTTPClient
		}
	}
	return c
}

// Publish sends a single push notification
// @param push_message: A PushMessage object
// @return an array of PushResponse objects which contains the results (one per each recipient).
// @return error if any requests failed
func (c *PushClient) Publish(ctx context.Context, message *PushMessage) ([]PushResponse, error) {
	responses, err := c.PublishMultiple(ctx, []PushMessage{*message})
	if err != nil {
		return nil, err
	}
	return responses, nil
}

// PublishMultiple sends multiple push notifications at once
// @param push_messages: An array of PushMessage objects.
// @return an array of PushResponse objects which contains the results.
// @return error if the request failed
func (c *PushClient) PublishMultiple(ctx context.Context, messages []PushMessage) ([]PushResponse, error) {
	return c.publishInternal(ctx, messages)
}

// validate checks that the messages are valid
// valid messages have at least one recipient and all recipients have a valid push token
func (c *PushClient) validate(messages []PushMessage) (int, error) {
	var count int
	// Validate the messages
	for _, message := range messages {
		if len(message.To) == 0 {
			return 0, errors.New("No recipients")
		}
		for _, recipient := range message.To {
			if !strings.HasPrefix(recipient, "ExponentPushToken") {
				return 0, errors.New("Invalid push token")
			}
		}
		count += len(message.To)
	}
	return count, nil
}

func (c *PushClient) buildRequest(ctx context.Context, messages []PushMessage) (*http.Request, error) {
	url := fmt.Sprintf("%s%s/push/send", c.host, c.apiURL)
	jsonBytes, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}

	// Create request w/ body
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}

	// Add appropriate headers
	req.Header.Add("Content-Type", "application/json")
	if c.accessToken != "" {
		req.Header.Add("Authorization", "Bearer "+c.accessToken)
	}
	return req, nil
}

func (c *PushClient) publishInternal(ctx context.Context, messages []PushMessage) ([]PushResponse, error) {
	// Validate the messages
	expectedReceipts, err := c.validate(messages)
	if err != nil {
		return nil, err
	}
	req, err := c.buildRequest(ctx, messages)
	if err != nil {
		return nil, err
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Check that we didn't receive an invalid response
	err = checkStatus(resp)
	if err != nil {
		return nil, err
	}

	// Validate the response format first
	var r *Response
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		// The response isn't json
		return nil, err
	}
	// If there are errors with the entire request, raise an error now.
	if r.Errors != nil {
		return nil, NewPushServerError("Invalid server response", resp, r, r.Errors)
	}
	// We expect the response to have a 'data' field with the responses.
	if r.Data == nil {
		return nil, NewPushServerError("Invalid server response", resp, r, nil)
	}
	// Sanity check the response
	if expectedReceipts != len(r.Data) {
		message := "Mismatched response length. Expected %d receipts but only received %d"
		errorMessage := fmt.Sprintf(message, len(messages), len(r.Data))
		return nil, NewPushServerError(errorMessage, resp, r, nil)
	}
	// Add the original message to each response for reference
	i := 0
	for _, msg := range messages {
		for _, to := range msg.To {
			r.Data[i].PushMessage = msg
			r.Data[i].PushMessage.To = []string{to}
			i += 1
		}
	}
	return r.Data, nil
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}
	return fmt.Errorf("Invalid response (%d %s)", resp.StatusCode, resp.Status)
}
