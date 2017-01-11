// Copyright 2014-2015 Apcera Inc. All rights reserved.

package auth

import (
	"strings"

	"github.com/nats-io/gnatsd/server"
	"golang.org/x/crypto/bcrypt"
)

const bcryptPrefix = "$2a$"

func isBcrypt(password string) bool {
	return strings.HasPrefix(password, bcryptPrefix)
}

// Plain authentication is a basic username and password
type Plain struct {
	Username string
	Password string
}

// Check authenticates the client using a username and password
func (p *Plain) Check(c server.ClientAuth) bool {
	opts := c.GetOpts()
	if p.Username != opts.Username {
		return false
	}
	// Check to see if the password is a bcrypt hash
	if isBcrypt(p.Password) {
		if err := bcrypt.CompareHashAndPassword([]byte(p.Password), []byte(opts.Password)); err != nil {
			return false
		}
	} else if p.Password != opts.Password {
		return false
	}

	return true
}
