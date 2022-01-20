module github.com/xenolog/k8s-utils

go 1.16

require (
	github.com/google/uuid v1.2.0
	github.com/k0kubun/pp v3.0.1+incompatible
	github.com/mattn/go-colorable v0.1.8 // indirect
	github.com/stretchr/testify v1.7.0
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime v0.9.0
)

replace (
	github.com/k0kubun/pp => github.com/k0kubun/pp/v3 v3.0.7
	github.com/operator-framework/operator-sdk => github.com/operator-framework/operator-sdk v0.18.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.3
	k8s.io/client-go => k8s.io/client-go v0.21.3 // Required by prometheus-operator
	k8s.io/code-generator => k8s.io/code-generator v0.21.3
	k8s.io/klog => k8s.io/klog v1.0.0
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.9.0
)
