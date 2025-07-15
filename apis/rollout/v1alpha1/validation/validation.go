package validation

import (
	"net/url"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// ValidateWebhookURL validates webhook's URL.
func ValidateWebhookURL(fldPath *field.Path, URL string, forceHttps bool) field.ErrorList {
	var allErrors field.ErrorList
	const form = "; desired format: https://host[/path]"
	if u, err := url.Parse(URL); err != nil {
		allErrors = append(allErrors, field.Required(fldPath, "url must be a valid URL: "+err.Error()+form))
	} else {
		if forceHttps && u.Scheme != "https" {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Scheme, "'https' is the only allowed URL scheme"+form))
		}
		if len(u.Host) == 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Host, "host must be provided"+form))
		}
		if u.User != nil {
			allErrors = append(allErrors, field.Invalid(fldPath, u.User.String(), "user information is not permitted in the URL"))
		}
		if len(u.Fragment) != 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.Fragment, "fragments are not permitted in the URL"))
		}
		if len(u.RawQuery) != 0 {
			allErrors = append(allErrors, field.Invalid(fldPath, u.RawQuery, "query parameters are not permitted in the URL"))
		}
	}
	return allErrors
}
