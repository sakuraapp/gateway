package gateway

import (
	"github.com/google/uuid"
	"github.com/sakuraapp/gateway/internal/client"
	"github.com/sakuraapp/shared/pkg/resource/opcode"
	log "github.com/sirupsen/logrus"
)

type ErrorCode int

const (
	ErrorDatabase ErrorCode = 100 + iota
	ErrorRedis
	ErrorClientSend
	ErrorDispatch
	ErrorNextItem
	ErrorSetCurrentItem
	ErrorSendState
	ErrorCrawler
	ErrorAddClient
	ErrorRemoveClient
	ErrorParse
	ErrorSerialize
)

var errorMessages = map[ErrorCode]string{
	ErrorDatabase: "Database Error",
	ErrorRedis: "Redis Error",
	ErrorClientSend: "Failed to send message",
	ErrorDispatch: "Failed to dispatch message",
	ErrorNextItem: "Failed to play next item",
	ErrorSetCurrentItem: "Failed to set current item",
	ErrorSendState: "Failed to send room state",
	ErrorCrawler: "Crawler Error",
	ErrorAddClient: "Failed to add client to room",
	ErrorRemoveClient: "Failed to remove client from room",
	ErrorParse: "Failed to parse data",
	ErrorSerialize: "Failed to serialize data",
}

type Error interface {
	Handle(c *client.Client) error
}

type BaseError struct {
	id   string
	err  error
	code ErrorCode
}

func (e *BaseError) dispatch(c *client.Client) error {
	return c.Send(opcode.Error, e.id)
}

func (e *BaseError) Handle(c *client.Client) error {
	log.WithError(e.err).
		WithField("error_id", e.id).
		Error(errorMessages[e.code])

	return e.dispatch(c) // report error to the client
}

type AuthError struct {
	err error
}

func (e *AuthError) Handle(c *client.Client) error {
	log.WithError(e.err).Error("Authentication Failed")
	c.Disconnect()

	return nil
}

func NewError(code ErrorCode, err error) *BaseError {
	return &BaseError{
		id:   uuid.NewString(),
		err:  err,
		code: code,
	}
}

func NewAuthError(err error) *AuthError {
	return &AuthError{err: err}
}