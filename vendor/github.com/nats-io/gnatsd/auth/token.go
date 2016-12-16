package auth

import (
	"github.com/nats-io/gnatsd/server"
	"golang.org/x/crypto/bcrypt"
)

// Token holds a string token used for authentication
type Token struct {
	Token string
}

// Check authenticates a client from a token
func (p *Token) Check(c server.ClientAuth) bool {
	opts := c.GetOpts()
	// Check to see if the token is a bcrypt hash
	if isBcrypt(p.Token) {
		if err := bcrypt.CompareHashAndPassword([]byte(p.Token), []byte(opts.Authorization)); err != nil {
			return false
		}
	} else if p.Token != opts.Authorization {
		return false
	}

	return true
}
