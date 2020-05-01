/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package proxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/gravitational/teleport/lib/utils"

	"github.com/gravitational/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/third_party/forked/golang/netutil"
)

// SPDYRoundTripper knows how to upgrade an HTTP request to one that supports
// multiplexed streams. After RoundTrip() is invoked, Conn will be set
// and usable. SPDYRoundTripper implements the UpgradeRoundTripper interface.
type SPDYRoundTripper struct {
	//tlsConfig holds the TLS configuration settings to use when connecting
	//to the remote server.
	tlsConfig *tls.Config

	/* TODO according to http://golang.org/pkg/net/http/#RoundTripper, a RoundTripper
	   must be safe for use by multiple concurrent goroutines. If this is absolutely
	   necessary, we could keep a map from http.Request to net.Conn. In practice,
	   a client will create an http.Client, set the transport to a new insteace of
	   SPDYRoundTripper, and use it a single time, so this hopefully won't be an issue.
	*/
	// conn is the underlying network connection to the remote server.
	conn net.Conn

	// dial is the function used connect to remote address
	dialFunc utils.DialWithContextFunc
}

var _ utilnet.TLSClientConfigHolder = &SPDYRoundTripper{}
var _ httpstream.UpgradeRoundTripper = &SPDYRoundTripper{}
var _ utilnet.Dialer = &SPDYRoundTripper{}

// NewSPDYRoundTripperWithDialer creates a new SPDYRoundTripper that will use
// the specified tlsConfig. This function is mostly meant for unit tests.
func NewSPDYRoundTripperWithDialer(dial utils.DialWithContextFunc, tlsConfig *tls.Config) *SPDYRoundTripper {
	return &SPDYRoundTripper{
		tlsConfig: tlsConfig,
		dialFunc:  dial,
	}
}

// TLSClientConfig implements pkg/util/net.TLSClientConfigHolder for proper TLS checking during
// proxying with a spdy roundtripper.
func (s *SPDYRoundTripper) TLSClientConfig() *tls.Config {
	return s.tlsConfig
}

// Dial implements k8s.io/apimachinery/pkg/util/net.Dialer.
func (s *SPDYRoundTripper) Dial(req *http.Request) (net.Conn, error) {
	dialAddr := netutil.CanonicalAddr(req.URL)

	if req.URL.Scheme == "http" {
		return s.dialFunc(req.Context(), "tcp", dialAddr)
	}

	conn, err := utils.TLSDial(req.Context(), s.dialFunc, "tcp", dialAddr, s.tlsConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Client handshake will verify the server hostname and cert chain. That
	// way we can err our before first read/write.
	if err := conn.Handshake(); err != nil {
		return nil, trace.Wrap(err)
	}

	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// RoundTrip executes the Request and upgrades it. After a successful upgrade,
// clients may call SPDYRoundTripper.Connection() to retrieve the upgraded
// connection.
func (s *SPDYRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	header := utilnet.CloneHeader(req.Header)
	header.Add(httpstream.HeaderConnection, httpstream.HeaderUpgrade)
	header.Add(httpstream.HeaderUpgrade, streamspdy.HeaderSpdy31)

	conn, rawResponse, err := utilnet.ConnectWithRedirects(req.Method, req.URL, header, req.Body, s, false)
	if err != nil {
		return nil, err
	}

	responseReader := bufio.NewReader(
		io.MultiReader(
			bytes.NewBuffer(rawResponse),
			conn,
		),
	)

	resp, err := http.ReadResponse(responseReader, nil)
	if err != nil {
		if conn != nil {
			conn.Close()
		}
		return nil, err
	}

	s.conn = conn

	return resp, nil
}

// NewConnection validates the upgrade response, creating and returning a new
// httpstream.Connection if there were no errors.
func (s *SPDYRoundTripper) NewConnection(resp *http.Response) (httpstream.Connection, error) {
	connectionHeader := strings.ToLower(resp.Header.Get(httpstream.HeaderConnection))
	upgradeHeader := strings.ToLower(resp.Header.Get(httpstream.HeaderUpgrade))
	if resp.StatusCode != http.StatusSwitchingProtocols ||
		!strings.Contains(connectionHeader, strings.ToLower(httpstream.HeaderUpgrade)) ||
		!strings.Contains(upgradeHeader, strings.ToLower(streamspdy.HeaderSpdy31)) {
		defer resp.Body.Close()

		respBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unable to upgrade connection or read the error from server response: %v", err)
		}
		// TODO: I don't belong here, I should be abstracted from this class
		if obj, _, err := statusCodecs.UniversalDecoder().Decode(respBytes, nil, &metav1.Status{}); err == nil {
			if status, ok := obj.(*metav1.Status); ok {
				return nil, &apierrors.StatusError{ErrStatus: *status}
			}
		}

		return nil, fmt.Errorf("unable to upgrade connection: %s", bytes.TrimSpace(respBytes))
	}

	return streamspdy.NewClientConnection(s.conn)
}

// statusScheme is private scheme for the decoding here until someone fixes the TODO in NewConnection
var statusScheme = runtime.NewScheme()

// ParameterCodec knows about query parameters used with the meta v1 API spec.
var statusCodecs = serializer.NewCodecFactory(statusScheme)

func init() {
	statusScheme.AddUnversionedTypes(metav1.SchemeGroupVersion,
		&metav1.Status{},
	)
}
