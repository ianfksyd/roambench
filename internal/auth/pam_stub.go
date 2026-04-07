//go:build !pam

package auth

import "errors"

// PAMAuth is a stub when built without the pam tag.
type PAMAuth struct{}

func NewPAMAuth() (*PAMAuth, error) {
	return nil, errors.New("PAM auth not compiled; rebuild with: go build -tags pam")
}

func (p *PAMAuth) Authenticate(username, password string) error {
	return errors.New("PAM auth not available")
}
