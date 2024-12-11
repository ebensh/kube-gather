module kube-query

go 1.23

require (
	github.com/mattn/go-sqlite3 v1.14.16
	k8s.io/api v0.27.3                  // Kubernetes API types
	k8s.io/apimachinery v0.27.3         // Kubernetes machinery for working with objects
	k8s.io/client-go v0.27.3            // Kubernetes client-go library
)
