module github.com/jenkins-x/jx-preview

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/jenkins-x/go-scm v1.5.209
	github.com/jenkins-x/jx-api/v4 v4.0.20
	github.com/jenkins-x/jx-gitops v0.0.524
	github.com/jenkins-x/jx-helpers/v3 v3.0.61
	github.com/jenkins-x/jx-kube-client/v3 v3.0.1
	github.com/jenkins-x/jx-logging/v3 v3.0.3
	github.com/pkg/errors v0.9.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	k8s.io/api v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v11.0.1-0.20190805182717-6502b5e7b1b5+incompatible
	k8s.io/kube-openapi v0.0.0-20200923155610-8b5066479488 // indirect
	knative.dev/serving v0.19.0
)

replace k8s.io/client-go => k8s.io/client-go v0.19.2

go 1.15
