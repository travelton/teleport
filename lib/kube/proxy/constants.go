/*
Copyright 2016 The Kubernetes Authors.

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

import "time"

// TODO(awly): use constants from vendor/k8s.io/api/core/v1/types.go

const (
	// impersonateHeaderPrefix is K8s impersonation prefix for impersonation feature:
	// https://kubernetes.io/docs/reference/access-authn-authz/authentication/#user-impersonation
	impersonateHeaderPrefix = "Impersonate-"
	// impersonateUserHeader is impersonation header for users
	impersonateUserHeader = "Impersonate-User"
	// impersonateGroupHeader is K8s impersonation header for user
	impersonateGroupHeader = "Impersonate-Group"
	// impersonationRequestDeniedMessage is access denied message for impersonation
	impersonationRequestDeniedMessage = "impersonation request has been denied"

	idleTimeout = 15 * time.Minute
)
