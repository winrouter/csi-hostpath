module github.com/winrouter/csi-hostpath

go 1.20

require (
	github.com/container-storage-interface/spec v1.8.0
	github.com/golang/protobuf v1.5.3
	github.com/google/credstore v0.0.0-20181218150457-e184c60ef875
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645
	github.com/kubernetes-csi/csi-lib-utils v0.13.0
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/openebs/lib-csi v0.7.0
	github.com/openebs/lvm-localpv v1.1.0
	github.com/opentracing/opentracing-go v1.1.0
	github.com/prometheus/client_golang v1.15.1
	github.com/spf13/cobra v1.6.0
	golang.org/x/net v0.10.0
	golang.org/x/sys v0.8.0
	google.golang.org/grpc v1.55.0
	google.golang.org/protobuf v1.30.0
	k8s.io/api v0.27.2
	k8s.io/apimachinery v0.27.2
	k8s.io/client-go v11.0.1-0.20190409021438-1a26190bd76a+incompatible
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.100.1
	k8s.io/mount-utils v0.24.5
	k8s.io/utils v0.0.0-20230505201702-9f6742963106
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emicklei/go-restful/v3 v3.10.2 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-microservice-helpers v0.0.0-20170611213619-4a88aaa13aa1 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.10.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
	golang.org/x/oauth2 v0.8.0 // indirect
	golang.org/x/term v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230306155012-7f2fa6fef1f4 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/square/go-jose.v2 v2.2.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230525220651-2546d827e515 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.54.0
	cloud.google.com/go/bigquery => cloud.google.com/go/bigquery v1.4.0
	cloud.google.com/go/datastore => cloud.google.com/go/datastore v1.1.0
	cloud.google.com/go/firestore => cloud.google.com/go/firestore v1.1.0
	cloud.google.com/go/pubsub => cloud.google.com/go/pubsub v1.2.0
	cloud.google.com/go/storage => cloud.google.com/go/storage v1.6.0
	github.com/go-logr/logr => github.com/go-logr/logr v0.4.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	google.golang.org/grpc => google.golang.org/grpc v1.43.0
	k8s.io/api => k8s.io/api v0.24.5
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.24.5
	k8s.io/apimachinery => k8s.io/apimachinery v0.24.5
	k8s.io/apiserver => k8s.io/apiserver v0.24.5
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.24.5
	k8s.io/client-go => k8s.io/client-go v0.24.5
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.24.5
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.24.5
	k8s.io/code-generator => k8s.io/code-generator v0.24.5
	k8s.io/component-base => k8s.io/component-base v0.24.5
	k8s.io/component-helpers => k8s.io/component-helpers v0.24.5
	k8s.io/controller-manager => k8s.io/controller-manager v0.24.5
	k8s.io/cri-api => k8s.io/cri-api v0.24.5
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.24.5
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.10.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.24.5
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.24.5
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.24.5
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.24.5
	k8s.io/kubectl => k8s.io/kubectl v0.24.5
	k8s.io/kubelet => k8s.io/kubelet v0.24.5
	k8s.io/kubernetes => k8s.io/kubernetes v1.24.5
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.24.5
	k8s.io/metrics => k8s.io/metrics v0.24.5
	k8s.io/mount-utils => k8s.io/mount-utils v0.21.0-beta.0
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.24.5
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.24.5
)
