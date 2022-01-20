module github.com/xenolog/k8s-utils

go 1.14

require (
	github.com/google/uuid v1.3.0
	github.com/k0kubun/pp v3.1.0+incompatible
	k8s.io/apimachinery v0.21.9
	k8s.io/client-go v0.21.9
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.9.7
	k8s.io/code-generator v0.21.9
)

replace github.com/k0kubun/pp => github.com/k0kubun/pp/v3 v3.1.0
