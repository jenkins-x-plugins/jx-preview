module github.com/jenkins-x/jx-preview

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/jenkins-x/go-scm v1.5.188
	github.com/jenkins-x/jx-api/v3 v3.0.1
	github.com/jenkins-x/jx-gitops v0.0.397
	github.com/jenkins-x/jx-helpers/v3 v3.0.8
	github.com/jenkins-x/jx-kube-client/v3 v3.0.1
	github.com/jenkins-x/jx-logging/v3 v3.0.2
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
)

replace k8s.io/client-go => k8s.io/client-go v0.19.2

go 1.15
