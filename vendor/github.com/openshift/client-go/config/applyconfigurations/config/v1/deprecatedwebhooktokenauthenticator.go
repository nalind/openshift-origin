// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

// DeprecatedWebhookTokenAuthenticatorApplyConfiguration represents a declarative configuration of the DeprecatedWebhookTokenAuthenticator type for use
// with apply.
type DeprecatedWebhookTokenAuthenticatorApplyConfiguration struct {
	KubeConfig *SecretNameReferenceApplyConfiguration `json:"kubeConfig,omitempty"`
}

// DeprecatedWebhookTokenAuthenticatorApplyConfiguration constructs a declarative configuration of the DeprecatedWebhookTokenAuthenticator type for use with
// apply.
func DeprecatedWebhookTokenAuthenticator() *DeprecatedWebhookTokenAuthenticatorApplyConfiguration {
	return &DeprecatedWebhookTokenAuthenticatorApplyConfiguration{}
}

// WithKubeConfig sets the KubeConfig field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the KubeConfig field is set to the value of the last call.
func (b *DeprecatedWebhookTokenAuthenticatorApplyConfiguration) WithKubeConfig(value *SecretNameReferenceApplyConfiguration) *DeprecatedWebhookTokenAuthenticatorApplyConfiguration {
	b.KubeConfig = value
	return b
}
