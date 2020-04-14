package amazon

import "github.com/aws/aws-sdk-go/aws/session"

type Client struct {
	sess *session.Session
}
