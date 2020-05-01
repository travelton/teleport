package proxy

import (
	"net/http"
	"strings"

	"github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/trace"
)

const (
	// ImpersonateHeaderPrefix is K8s impersonation prefix for impersonation feature:
	// https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation
	ImpersonateHeaderPrefix = "Impersonate-"
	// ImpersonateUserHeader is impersonation header for users
	ImpersonateUserHeader = "Impersonate-User"
	// ImpersonateGroupHeader is K8s impersonation header for user
	ImpersonateGroupHeader = "Impersonate-Group"
	// ImpersonationRequestDeniedMessage is access denied message for impersonation
	ImpersonationRequestDeniedMessage = "impersonation request has been denied"
)

// wrapSetupHeaders returns a new http.RoundTripper which adds impersonation
// and forwarding headers to requests before passing them to rt.
func wrapSetupHeaders(rt http.RoundTripper, ctx *authContext, sess *clusterSession) http.RoundTripper {
	return headerRoundTripper{
		next: rt,
		ctx:  ctx,
		sess: sess,
	}
}

type headerRoundTripper struct {
	next http.RoundTripper
	ctx  *authContext
	sess *clusterSession
}

func (rt headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := setupHeaders(rt.ctx, rt.sess, req); err != nil {
		return nil, trace.Wrap(err)
	}
	return rt.next.RoundTrip(req)
}

// setupHeaders adds impersonation and forwarding headers to req.
func setupHeaders(ctx *authContext, sess *clusterSession, req *http.Request) error {
	if err := setupImpersonationHeaders(ctx, req.Header); err != nil {
		return trace.Wrap(err)
	}

	// Setup scheme, override target URL to the destination address
	req.URL.Scheme = "https"
	req.URL.Host = sess.cluster.targetAddr
	req.RequestURI = req.URL.Path + "?" + req.URL.RawQuery

	// add origin headers so the service consuming the request on the other site
	// is aware of where it came from
	req.Header.Add("X-Forwarded-Proto", "https")
	req.Header.Add("X-Forwarded-Host", req.Host)
	req.Header.Add("X-Forwarded-Path", req.URL.Path)

	return nil
}

// setupImpersonationHeaders sets up Impersonate-User and Impersonate-Group headers
func setupImpersonationHeaders(ctx *authContext, headers http.Header) error {
	var impersonateUser string
	var impersonateGroups []string
	for header, values := range headers {
		if !strings.HasPrefix(header, ImpersonateHeaderPrefix) {
			continue
		}
		switch header {
		case ImpersonateUserHeader:
			if impersonateUser != "" {
				return trace.AccessDenied("%v, user already specified to %q", ImpersonationRequestDeniedMessage, impersonateUser)
			}
			if len(values) == 0 || len(values) > 1 {
				return trace.AccessDenied("%v, invalid user header %q", ImpersonationRequestDeniedMessage, values)
			}
			impersonateUser = values[0]
			if _, ok := ctx.kubeUsers[impersonateUser]; !ok {
				return trace.AccessDenied("%v, user header %q is not allowed in roles", ImpersonationRequestDeniedMessage, impersonateUser)
			}
		case ImpersonateGroupHeader:
			for _, group := range values {
				if _, ok := ctx.kubeGroups[group]; !ok {
					return trace.AccessDenied("%v, group header %q value is not allowed in roles", ImpersonationRequestDeniedMessage, group)
				}
				impersonateGroups = append(impersonateGroups, group)
			}
		default:
			return trace.AccessDenied("%v, unsupported impersonation header %q", ImpersonationRequestDeniedMessage, header)
		}
	}

	impersonateGroups = utils.Deduplicate(impersonateGroups)

	// By default, if no kubernetes_users is set (which will be a majority),
	// user will impersonate themselves, which is the backwards-compatible behavior.
	//
	// As long as at least one `kubernetes_users` is set, the forwarder will start
	// limiting the list of users allowed by the client to impersonate.
	//
	// If the users' role set does not include actual user name, it will be rejected,
	// otherwise there will be no way to exclude the user from the list).
	//
	// If the `kubernetes_users` role set includes only one user
	// (quite frequently that's the real intent), teleport will default to it,
	// otherwise it will refuse to select.
	//
	// This will enable the use case when `kubernetes_users` has just one field to
	// link the user identity with the IAM role, for example `IAM#{{external.email}}`
	//
	if impersonateUser == "" {
		switch len(ctx.kubeUsers) {
		// this is currently not possible as kube users have at least one
		// user (user name), but in case if someone breaks it, catch here
		case 0:
			return trace.AccessDenied("assumed at least one user to be present")
		// if there is deterministic choice, make it to improve user experience
		case 1:
			for user := range ctx.kubeUsers {
				impersonateUser = user
				break
			}
		default:
			return trace.AccessDenied(
				"please select a user to impersonate, refusing to select a user due to several kuberenetes_users set up for this user")
		}
	}

	if len(impersonateGroups) == 0 {
		for group := range ctx.kubeGroups {
			impersonateGroups = append(impersonateGroups, group)
		}
	}

	if !ctx.cluster.isRemote {
		headers.Set(ImpersonateUserHeader, impersonateUser)

		// Make sure to overwrite the exiting headers, instead of appending to
		// them.
		headers[ImpersonateGroupHeader] = nil
		for _, group := range impersonateGroups {
			headers.Add(ImpersonateGroupHeader, group)
		}
	}
	return nil
}
