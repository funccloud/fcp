package yamlutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyManifestFromURL downloads a YAML manifest from a URL and applies its resources.
func ApplyManifestFromURL(ctx context.Context, k8sClient client.Client, ioStreams genericiooptions.IOStreams, url string) error {
	manifestBytes, err := DownloadYAMLFromURL(ctx, url, ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error downloading manifest", "url", url, "error", err)
		return fmt.Errorf("error downloading manifest from %s: %w", url, err)
	}
	err = ApplyManifestYAML(ctx, k8sClient, string(manifestBytes), ioStreams)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error applying manifest", "url", url, "error", err)
		return fmt.Errorf("error applying manifest from %s: %w", url, err)
	}

	_, _ = fmt.Fprintln(ioStreams.Out, "Manifest application finished.", "url", url)
	return nil
}

// ApplyManifestYAML applies a Kubernetes manifest provided as a YAML string.
// It decodes the YAML and applies each object using Server-Side Apply.
func ApplyManifestYAML(ctx context.Context, k8sClient client.Client, manifestYAML string, ioStreams genericiooptions.IOStreams) error {
	decoder := yaml.NewYAMLToJSONDecoder(strings.NewReader(manifestYAML))
	for {
		obj := &unstructured.Unstructured{}
		err := decoder.Decode(obj)
		if err != nil {
			if err == io.EOF {
				break // End of YAML stream
			}
			return fmt.Errorf("failed to decode YAML object: %w", err)
		}

		if obj.Object == nil {
			continue // Skip empty objects
		}

		_, _ = fmt.Fprintln(ioStreams.Out, "Applying object", "kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace())

		patch := client.Apply
		opts := []client.PatchOption{client.ForceOwnership, client.FieldOwner("fcp-manager")}
		err = k8sClient.Patch(ctx, obj, patch, opts...)
		if err != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Failed to apply object", "kind", obj.GetKind(), "name", obj.GetName(), "namespace", obj.GetNamespace(), "error", err)
			return fmt.Errorf("failed to apply object %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}
	return nil
}

func DownloadYAMLFromURL(ctx context.Context, url string, ioStreams genericiooptions.IOStreams) ([]byte, error) {
	httpClient := &http.Client{
		Timeout: 30 * time.Second, // Set a timeout for the HTTP request
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error creating HTTP request", "url", url, "error", err)
		return nil, fmt.Errorf("error creating request to download manifest: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error downloading manifest", "url", url, "error", err)
		return nil, fmt.Errorf("error downloading manifest from %s: %w", url, err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error closing response body for manifest download", "url", url, "error", cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("status code %d", resp.StatusCode)
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error downloading manifest", "url", url, "error", err)
		return nil, fmt.Errorf("non-OK status (%d) downloading manifest from %s", resp.StatusCode, url)
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		_, _ = fmt.Fprintln(ioStreams.ErrOut, "Error reading manifest response body", "url", url, "error", err)
		return nil, fmt.Errorf("error reading manifest: %w", err)
	}
	return manifestBytes, nil
}
