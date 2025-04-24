package yamlutil

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("YAML Utils", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		log       = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel))
	)

	BeforeEach(func() {
		ctx = context.Background()
		logf.SetLogger(log)
		scheme = runtime.NewScheme()
		// Add necessary schemes if testing with specific K8s types
		// e.g., corev1.AddToScheme(scheme)
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	Describe("ApplyManifestYAML", func() {
		Context("with a valid single object manifest", func() {
			It("should apply the object using server-side apply", func() {
				manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: default
data:
  key: value
`
				err := ApplyManifestYAML(ctx, k8sClient, manifest, log)
				Expect(err).NotTo(HaveOccurred())

				// Verify the object was created/patched
				cm := &unstructured.Unstructured{}
				cm.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, cm)
				Expect(err).NotTo(HaveOccurred())
				Expect(cm.Object["data"]).To(Equal(map[string]interface{}{"key": "value"}))
			})
		})

		Context("with a valid multi-object manifest", func() {
			It("should apply all objects", func() {
				manifest := `
apiVersion: v1
kind: Namespace
metadata:
  name: test-ns
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm-multi
  namespace: test-ns
data:
  multi: obj
`
				err := ApplyManifestYAML(ctx, k8sClient, manifest, log)
				Expect(err).NotTo(HaveOccurred())

				// Verify namespace
				ns := &unstructured.Unstructured{}
				ns.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"})
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-ns"}, ns)
				Expect(err).NotTo(HaveOccurred())

				// Verify configmap
				cm := &unstructured.Unstructured{}
				cm.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-cm-multi", Namespace: "test-ns"}, cm)
				Expect(err).NotTo(HaveOccurred())
				Expect(cm.Object["data"]).To(Equal(map[string]interface{}{"multi": "obj"}))
			})
		})

		Context("with an empty manifest", func() {
			It("should return no error", func() {
				manifest := ``
				err := ApplyManifestYAML(ctx, k8sClient, manifest, log)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with only comments or empty documents", func() {
			It("should return no error", func() {
				manifest := `
# This is a comment
---
# Another comment
---

---
`
				err := ApplyManifestYAML(ctx, k8sClient, manifest, log)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with invalid YAML", func() {
			It("should return a decoding error", func() {
				manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
  namespace: default
data:
  key: value
invalid-yaml: :
`
				err := ApplyManifestYAML(ctx, k8sClient, manifest, log)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to decode YAML object"))
			})
		})

		Context("when the client fails to patch", func() {
			It("should return an error", func() {
				// Configure the fake client to return an error on Patch
				failingClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						return fmt.Errorf("simulated patch error")
					},
				}).Build()

				manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm-fail
  namespace: default
data:
  key: value
`
				err := ApplyManifestYAML(ctx, failingClient, manifest, log)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to apply object ConfigMap/test-cm-fail"))
				Expect(err.Error()).To(ContainSubstring("simulated patch error"))
			})
		})
	})

	Describe("ApplyManifestFromURL", func() {
		var (
			server *httptest.Server
			mux    *http.ServeMux
		)

		BeforeEach(func() {
			mux = http.NewServeMux()
			server = httptest.NewServer(mux)
		})

		AfterEach(func() {
			server.Close()
		})

		Context("with a valid URL and manifest", func() {
			It("should download and apply the manifest", func() {
				manifest := `
apiVersion: v1
kind: Service
metadata:
  name: test-svc
  namespace: default
spec:
  ports:
  - port: 80
`
				mux.HandleFunc("/manifest.yaml", func(w http.ResponseWriter, r *http.Request) {
					Expect(r.Method).To(Equal("GET"))
					_, err := fmt.Fprint(w, manifest)
					Expect(err).NotTo(HaveOccurred())
				})

				url := server.URL + "/manifest.yaml"
				err := ApplyManifestFromURL(ctx, k8sClient, log, url)
				Expect(err).NotTo(HaveOccurred())

				// Verify the object was created/patched
				svc := &unstructured.Unstructured{}
				svc.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"})
				err = k8sClient.Get(ctx, types.NamespacedName{Name: "test-svc", Namespace: "default"}, svc)
				Expect(err).NotTo(HaveOccurred())
				Expect(svc.Object["spec"]).To(HaveKey("ports"))
			})
		})

		Context("when the URL returns a non-200 status code", func() {
			It("should return an error", func() {
				mux.HandleFunc("/notfound.yaml", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
				})

				url := server.URL + "/notfound.yaml"
				err := ApplyManifestFromURL(ctx, k8sClient, log, url)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-OK status (404) downloading manifest"))
			})
		})

		Context("when the URL is invalid or unreachable", func() {
			It("should return an error", func() {
				// No server running at this address
				url := "http://invalid-address-that-does-not-exist/manifest.yaml"
				err := ApplyManifestFromURL(ctx, k8sClient, log, url)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("error downloading manifest"),
					// Depending on OS/network config, error might differ slightly
					ContainSubstring("no such host"),
					ContainSubstring("connection refused"),
				))
			})
		})

		Context("when reading the response body fails", func() {
			// This is harder to simulate reliably with httptest, but test the logic path
			// We can test the ApplyManifestYAML error path instead, which covers similar ground
			It("should return an error (tested via ApplyManifestYAML error)", func() {
				// Simulate ApplyManifestYAML failing after a successful download
				failingClient := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
					Patch: func(ctx context.Context, cl client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
						return fmt.Errorf("simulated patch error during URL apply")
					},
				}).Build()

				manifest := `
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: default
spec:
  containers:
  - name: test
    image: busybox
`
				mux.HandleFunc("/fail-apply.yaml", func(w http.ResponseWriter, r *http.Request) {
					_, err := fmt.Fprint(w, manifest)
					Expect(err).NotTo(HaveOccurred())
				})

				url := server.URL + "/fail-apply.yaml"
				err := ApplyManifestFromURL(ctx, failingClient, log, url)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error applying manifest from"))
				Expect(err.Error()).To(ContainSubstring("simulated patch error during URL apply"))

			})
		})

		Context("when ApplyManifestYAML fails", func() {
			It("should return the underlying error", func() {
				// Use invalid YAML content from the server
				manifest := `invalid: yaml: here`
				mux.HandleFunc("/invalid.yaml", func(w http.ResponseWriter, r *http.Request) {
					_, err := fmt.Fprint(w, manifest)
					Expect(err).NotTo(HaveOccurred())
				})

				url := server.URL + "/invalid.yaml"
				err := ApplyManifestFromURL(ctx, k8sClient, log, url)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error applying manifest from"))
				Expect(err.Error()).To(ContainSubstring("failed to decode YAML object"))
			})
		})
	})
})
