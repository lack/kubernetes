// This is a generated file. Do not edit directly.

module k8s.io/cloud-provider

go 1.13

require (
	google.golang.org/appengine v1.5.0 // indirect
	k8s.io/api v0.18.0-beta.2
	k8s.io/apimachinery v0.18.0-beta.2
	k8s.io/client-go v0.18.0-beta.2
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200229041039-0a110f9eb7ab
)

replace (
	golang.org/x/net => golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // pinned to release-branch.go1.13
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7 // pinned to release-branch.go1.13
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/cloud-provider => ../cloud-provider
)
