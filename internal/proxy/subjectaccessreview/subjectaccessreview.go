package subjectaccessreview

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrorNoImpersonationUserFound = errors.New("no Impersonation-User header found for request")
)

// structure for storing the review data
type SubjectAccessReview struct {
	client client.Client
}

// create a new SubjectAccessReview structure
func New(client client.Client) *SubjectAccessReview {
	return &SubjectAccessReview{
		client: client,
	}
}

// checks the request for impersonation headers, validates that the user is able to perform that impersonation,
// and builds the target object
func (subjectAccessReview *SubjectAccessReview) CheckAuthorizedForImpersonation(req *http.Request, requester user.Info) (user.Info, error) {

	impersonatedUser := req.Header.Get("impersonate-user")

	hasImpersonatedUser := impersonatedUser != ""

	hasImpersonation := false

	targetUser := &user.DefaultInfo{
		Name:   "",
		Groups: make([]string, 0),
		Extra:  map[string][]string{},
		UID:    "",
	}

	headersToRemove := make(map[string]string)

	for key, values := range req.Header {
		keyToCheck := strings.ToLower(key)
		if strings.HasPrefix(keyToCheck, "impersonate-") {
			if !hasImpersonatedUser {
				// found impersonation header, but not a user
				return nil, ErrorNoImpersonationUserFound
			}

			headersToRemove[key] = key
			hasImpersonation = true
			if keyToCheck == "impersonate-user" {
				userToImpersonate := values[0]
				if userToImpersonate != "" {
					result, err := subjectAccessReview.checkRbacImpersonationAuthorization(req.Context(), "users", userToImpersonate, requester)
					if err != nil {
						return nil, err
					} else {
						if !result {
							return nil, fmt.Errorf("%s is not allowed to impersonate user '%s'", requester.GetName(), userToImpersonate)
						} else {
							targetUser.Name = userToImpersonate
						}
					}
				}
			} else if keyToCheck == "impersonate-group" {

				for i := range values {
					groupName := values[i]
					result, err := subjectAccessReview.checkRbacImpersonationAuthorization(req.Context(), "groups", groupName, requester)
					if err != nil {
						return nil, err
					} else {
						if !result {
							return nil, fmt.Errorf("%s is not allowed to impersonate group '%s'", requester.GetName(), groupName)
						} else {
							targetUser.Groups = append(targetUser.Groups, groupName)
						}
					}
				}
			} else if keyToCheck == "impersonate-uid" {
				uidToImpersonate := values[0]
				result, err := subjectAccessReview.checkRbacImpersonationAuthorization(req.Context(), "uids", uidToImpersonate, requester)
				if err != nil {
					return nil, err
				} else {
					if !result {
						return nil, fmt.Errorf("%s is not allowed to impersonate uid '%s'", requester.GetName(), uidToImpersonate)
					} else {
						targetUser.UID = uidToImpersonate
					}
				}
			} else if strings.HasPrefix(keyToCheck, "impersonate-extra-") {
				// according to https://github.com/kubernetes/kubernetes/blob/555623c07eabf22864f6147736fa191e020cca25/staging/src/k8s.io/apiserver/pkg/authentication/user/user.go#L31-L41
				// the extra name MUST be lowercase...so we'll force to lowercase for the rbac check
				extraName := strings.ToLower(key[18:])
				for i := range values {
					result, err := subjectAccessReview.checkRbacImpersonationAuthorization(req.Context(), "userextras/"+extraName, values[i], requester)
					if err != nil {
						return nil, err
					} else {
						if !result {

							return nil, fmt.Errorf("%s is not allowed to impersonate extra info '%s'='%s'", requester.GetName(), extraName, values[i])
						} else {
							infoVals, ok := targetUser.Extra[extraName]

							if !ok {
								infoVals = make([]string, 0)

							}

							infoVals = append(infoVals, values[i])
							targetUser.Extra[extraName] = infoVals
						}
					}
				}
			} else if strings.HasPrefix(keyToCheck, "impersonate-") {
				// unknown impersonation header, fail
				return nil, fmt.Errorf("unknown impersonation header '%s'", key)
			}

		}

	}

	if hasImpersonation {

		// first clearing out the old headers
		newHeaders := http.Header{}

		for k := range req.Header {
			if _, ok := headersToRemove[k]; !ok {
				for _, v := range req.Header.Values(k) {
					newHeaders.Add(k, v)
				}
			}
		}

		// haven't errored out, but has impersonation - returning target user
		req.Header = newHeaders

		return targetUser, nil
	} else {
		// no impersonation, no user to return
		return nil, nil
	}
}

// submit a SubjectAccessReview request to the API server to validate that impersonation can occur
func (subjectAccessReview *SubjectAccessReview) checkRbacImpersonationAuthorization(ctx context.Context, resource string, name string, requester user.Info) (bool, error) {
	extras := map[string]authzv1.ExtraValue{}
	var group string
	var subresource string

	for key, value := range requester.GetExtra() {
		extras[key] = value
	}

	slashIndex := strings.Index(resource, "/")

	if slashIndex > 0 {
		newResources := strings.Split(resource, "/")
		resource = newResources[0]
		subresource = newResources[1]
		group = "authentication.k8s.io"
	}

	clusterSubjectAccessReview := authzv1.SubjectAccessReview{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sar-" + resource + "-" + name, // Create a unique name
		},
		Spec: authzv1.SubjectAccessReviewSpec{
			User:   requester.GetName(),
			Groups: requester.GetGroups(),
			Extra:  extras,

			ResourceAttributes: &authzv1.ResourceAttributes{
				Verb:        "impersonate",
				Group:       group,
				Resource:    resource,
				Subresource: subresource,
				Name:        name,
			},
		},
	}

	err := subjectAccessReview.client.Create(ctx, &clusterSubjectAccessReview)

	if err != nil {
		return false, err
	} else {
		return clusterSubjectAccessReview.Status.Allowed, nil
	}
}
