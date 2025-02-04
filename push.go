package expo

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// ErrMalformedToken is returned if a token does not start with 'ExponentPushToken'
var ErrMalformedToken = errors.New("Token should start with ExponentPushToken")

// NewExponentPushToken returns a token and may return an error if the input token is invalid
func NewExponentPushToken(token string) (string, error) {
	if !strings.HasPrefix(token, "ExponentPushToken") {
		return "", ErrMalformedToken
	}
	return token, nil
}

const (
	// DefaultPriority is the standard priority used in PushMessage
	DefaultPriority = "default"
	// NormalPriority is a priority used in PushMessage
	NormalPriority = "normal"
	// HighPriority is a priority used in PushMessage
	HighPriority = "high"
)

// PushMessage is an object that describes a push notification request.
// https://github.com/expo/expo/blob/f14ebb06b858e893ed569fd29b60be6146057c10/docs/pages/push-notifications/sending-notifications.mdx#message-request-format
type PushMessage struct {
	To             []string          `json:"to"`                       // An Expo push token or an array of Expo push tokens specifying the recipient(s) of this message.
	Body           string            `json:"body"`                     // The message to display in the notification.
	Data           map[string]string `json:"data,omitempty"`           // A JSON object delivered to your app.
	Sound          string            `json:"sound,omitempty"`          // Play a sound when the recipient receives this notification.
	Title          string            `json:"title,omitempty"`          // The title to display in the notification.
	TTLSeconds     int               `json:"ttl,omitempty"`            // Time to Live: the number of seconds for which the message may be kept around for redelivery if it hasn't been delivered yet.
	Expiration     int64             `json:"expiration,omitempty"`     // Timestamp since the Unix epoch specifying when the message expires.
	Priority       string            `json:"priority,omitempty"`       // The delivery priority of the message.
	Badge          int               `json:"badge,omitempty"`          // Number to display in the badge on the app icon.
	ChannelID      string            `json:"channelId,omitempty"`      // ID of the Notification Channel through which to display this notification.
	CategoryID     string            `json:"categoryId,omitempty"`     // ID of the notification category that this notification is associated with.
	MutableContent bool              `json:"mutableContent,omitempty"` // Specifies whether this notification can be intercepted by the client app.
}

// Response is the HTTP response returned from an Expo publish HTTP request
type Response struct {
	Data   []PushResponse      `json:"data"`
	Errors []map[string]string `json:"errors"`
}

// SuccessStatus is the status returned from Expo on a success
const SuccessStatus = "ok"

// ErrorDeviceNotRegistered indicates the token is invalid
const ErrorDeviceNotRegistered = "DeviceNotRegistered"

// ErrorMessageTooBig indicates the message went over payload size of 4096 bytes
const ErrorMessageTooBig = "MessageTooBig"

// ErrorMessageRateExceeded indicates messages have been sent too frequently
const ErrorMessageRateExceeded = "MessageRateExceeded"

// MismatchSenderId indicates that there is an issue with your FCM push credentials
const MismatchSenderId = "MismatchSenderId"

// Invalid credentials indicates your push notification credentials for your standalone app are invalid
const InvalidCredentials = "InvalidCredentials"

// ErrorProviderError indicates the provider (FCM or APNs) respond error
const ErrorProviderError = "ProviderError"

// PushResponse is a wrapper class for a push notification response.
// A successful single push notification:
//
//	{'status': 'ok'}
//
// An invalid push token
//
//	{'status': 'error',
//	 'message': '"adsf" is not a registered push notification recipient'}
type PushResponse struct {
	PushMessage PushMessage
	ID          string                     `json:"id"`
	Status      string                     `json:"status"`
	Message     string                     `json:"message"`
	Details     map[string]json.RawMessage `json:"details"`
}

func (r *PushResponse) isSuccess() bool {
	return r.Status == SuccessStatus
}

// ValidateResponse returns an error if the response indicates that one occurred.
// Clients should handle these errors, since these require custom handling
// to properly resolve.
func (r *PushResponse) ValidateResponse() error {
	if r.isSuccess() {
		return nil
	}
	err := &PushResponseError{
		Response: r,
	}
	// Handle specific errors if we have information
	if r.Details != nil {
		e := string(r.Details["error"])
		if e == ErrorDeviceNotRegistered {
			return &DeviceNotRegisteredError{
				PushResponseError: *err,
			}
		} else if e == ErrorMessageTooBig {
			return &MessageTooBigError{
				PushResponseError: *err,
			}
		} else if e == ErrorMessageRateExceeded {
			return &MessageRateExceededError{
				PushResponseError: *err,
			}
		} else if e == ErrorProviderError {
			return &ProviderError{
				PushResponseError: *err,
			}
		} else if e == MismatchSenderId {
			return &MismatchSenderIdError{
				PushResponseError: *err,
			}
		} else if e == InvalidCredentials {
			return &InvalidCredentialsError{
				PushResponseError: *err,
			}
		}
	}
	return err
}

// ProviderError is raised when the provider (FCM or APNs) respond error
// On Android, error message is json string. for example: {"fcm":{"error":"MismatchSenderId"}}
type ProviderError struct {
	PushResponseError
}

type MismatchSenderIdError struct {
	PushResponseError
}

type InvalidCredentialsError struct {
	PushResponseError
}

// PushResponseError is a base class for all push reponse errors
type PushResponseError struct {
	Response *PushResponse
}

func (e *PushResponseError) Error() string {
	if e.Response != nil {
		return e.Response.Message
	}
	return "Unknown push response error"
}

// DeviceNotRegisteredError is raised when the push token is invalid
// To handle this error, you should stop sending messages to this token.
type DeviceNotRegisteredError struct {
	PushResponseError
}

// MessageTooBigError is raised when the notification was too large.
// On Android and iOS, the total payload must be at most 4096 bytes.
type MessageTooBigError struct {
	PushResponseError
}

// MessageRateExceededError is raised when you are sending messages too frequently to a device
// You should implement exponential backoff and slowly retry sending messages.
type MessageRateExceededError struct {
	PushResponseError
}

// PushServerError is raised when the push token server is not behaving as expected
// For example, invalid push notification arguments result in a different
// style of error. Instead of a "data" array containing errors per
// notification, an "error" array is returned.
// {"errors": [
//
//	{"code": "API_ERROR",
//	 "message": "child \"to\" fails because [\"to\" must be a string]. \"value\" must be an array."
//	}
//
// ]}
type PushServerError struct {
	Message      string
	Response     *http.Response
	ResponseData *Response
	Errors       []map[string]string
}

// NewPushServerError creates a new PushServerError object
func NewPushServerError(message string, response *http.Response,
	responseData *Response,
	errors []map[string]string) *PushServerError {
	return &PushServerError{
		Message:      message,
		Response:     response,
		ResponseData: responseData,
		Errors:       errors,
	}
}

func (e *PushServerError) Error() string {
	return e.Message
}
