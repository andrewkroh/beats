// Package ec2query provides serialization of AWS EC2 requests and responses.
package ec2query

//go:generate go run -tags codegen ../../../models/protocol_tests/generate.go ../../../models/protocol_tests/input/ec2.json build_test.go

import (
	"net/url"

	request "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/awserr"
	"github.com/aws/aws-sdk-go-v2/private/protocol/query/queryutil"
)

// BuildHandler is a named request handler for building ec2query protocol requests
var BuildHandler = request.NamedHandler{Name: "awssdk.ec2query.Build", Fn: Build}

// Build builds a request for the EC2 protocol.
func Build(r *request.Request) {
	body := url.Values{
		"Action":  {r.Operation.Name},
		"Version": {r.Metadata.APIVersion},
	}
	if err := queryutil.Parse(body, r.Params, true); err != nil {
		r.Error = awserr.New("SerializationError", "failed encoding EC2 Query request", err)
	}

	if r.ExpireTime == 0 {
		r.HTTPRequest.Method = "POST"
		r.HTTPRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		r.SetBufferBody([]byte(body.Encode()))
	} else { // This is a pre-signed request
		r.HTTPRequest.Method = "GET"
		r.HTTPRequest.URL.RawQuery = body.Encode()
	}
}
