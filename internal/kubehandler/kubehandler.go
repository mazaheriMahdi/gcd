package kubehandler

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/pointer" // For pointer.Bool()
	"sigs.k8s.io/yaml"     // For YAML to JSON conversion
)

// KubeHandler provides methods to interact with a Kubernetes cluster.
type KubeHandler struct {
	clientset       kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	// namespace    string // Default namespace, can be added later if needed
}

// NewKubeHandler creates a new KubeHandler instance.
// It initializes connections to the Kubernetes cluster.
// Priority:
// 1. kubeconfigContent (if provided)
// 2. kubeconfigPath (if provided)
// 3. In-cluster configuration
func NewKubeHandler(kubeconfigPath string, kubeconfigContent []byte) (*KubeHandler, error) {
	var config *rest.Config
	var err error

	if len(kubeconfigContent) > 0 {
		log.Println("Using kubeconfig from provided content")
		config, err = clientcmd.RESTConfigFromKubeConfig(kubeconfigContent)
	} else if kubeconfigPath != "" {
		log.Printf("Using kubeconfig from path: %s\n", kubeconfigPath)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		log.Println("Using in-cluster Kubernetes config")
		config, err = rest.InClusterConfig()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes discovery client: %w", err)
	}

	return &KubeHandler{
		clientset:       clientset,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
	}, nil
}

// ApplyManifestFile reads a YAML manifest file, splits it into individual documents,
// and applies each document to the Kubernetes cluster using Server-Side Apply.
func (kh *KubeHandler) ApplyManifestFile(filePath string) error {
	log.Printf("Applying manifest file: %s\n", filePath)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read manifest file %s: %w", filePath, err)
	}

	// Split multi-document YAML. A simple split by "---" works for many cases.
	// More robust parsing might be needed for complex YAML structures or comments around "---".
	yamlDocs := strings.Split(string(content), "---")
	var applyErrors []string

	for i, doc := range yamlDocs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue // Skip empty documents (e.g., after a trailing ---)
		}

		log.Printf("Applying document #%d from %s\n", i+1, filePath)

		// 1. Convert YAML to JSON
		jsonData, err := yaml.YAMLToJSON([]byte(doc))
		if err != nil {
			log.Printf("Error converting YAML doc #%d to JSON: %v. Skipping.\n", i+1, err)
			applyErrors = append(applyErrors, fmt.Sprintf("doc #%d: YAML to JSON conversion failed: %v", i+1, err))
			continue
		}

		// 2. Decode JSON into an Unstructured object
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(jsonData); err != nil {
			log.Printf("Error unmarshalling JSON for doc #%d: %v. Skipping.\n", i+1, err)
			applyErrors = append(applyErrors, fmt.Sprintf("doc #%d: JSON unmarshalling failed: %v", i+1, err))
			continue
		}

		if obj.GetKind() == "" || obj.GetAPIVersion() == "" {
			log.Printf("Doc #%d (%s) is missing kind or apiVersion, skipping.\n", i+1, obj.GetName())
			applyErrors = append(applyErrors, fmt.Sprintf("doc #%d (%s): missing kind or apiVersion", i+1, obj.GetName()))
			continue
		}

		gvk := obj.GroupVersionKind()
		log.Printf("Processing GVK: %s, Name: %s, Namespace: %s\n", gvk, obj.GetName(), obj.GetNamespace())

		// 3. Discover the APIResource for this GVK
		apiResource, err := kh.findAPIResource(gvk)
		if err != nil {
			log.Printf("Error finding API resource for GVK %s (doc #%d): %v. Skipping.\n", gvk, i+1, err)
			applyErrors = append(applyErrors, fmt.Sprintf("doc #%d GVK %s: API discovery failed: %v", i+1, gvk, err))
			continue
		}

		// 4. Get the dynamic resource interface
		gvr := schema.GroupVersionResource{Group: gvk.Group, Version: gvk.Version, Resource: apiResource.Name}
		var dr dynamic.ResourceInterface
		if apiResource.Namespaced {
			namespace := obj.GetNamespace()
			if namespace == "" {
				namespace = "default" // Or use kh.namespace if defined and no namespace in manifest
				log.Printf("No namespace found for %s %s, defaulting to '%s'", gvk.Kind, obj.GetName(), namespace)
			}
			dr = kh.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			dr = kh.dynamicClient.Resource(gvr)
		}

		// 5. Apply using Server-Side Apply
		log.Printf("Applying %s %s (namespace: %s) with Server-Side Apply...\n", obj.GetKind(), obj.GetName(), obj.GetNamespace())
		_, err = dr.Patch(context.TODO(), obj.GetName(), types.ApplyPatchType, jsonData, metav1.PatchOptions{
			FieldManager: "go-argo-lite",     // Replace with your application's name
			Force:        pointer.Bool(true), // Optional: Force ownership conflicts
		})

		if err != nil {
			log.Printf("Error applying doc #%d (%s %s): %v\n", i+1, obj.GetKind(), obj.GetName(), err)
			applyErrors = append(applyErrors, fmt.Sprintf("doc #%d (%s %s): apply failed: %v", i+1, obj.GetKind(), obj.GetName(), err))
		} else {
			log.Printf("Successfully applied/configured doc #%d (%s %s)\n", i+1, obj.GetKind(), obj.GetName())
		}
	}

	if len(applyErrors) > 0 {
		return fmt.Errorf("encountered errors during manifest application:\n - %s", strings.Join(applyErrors, "\n - "))
	}

	return nil
}

// findAPIResource discovers the metav1.APIResource for a given GroupVersionKind.
func (kh *KubeHandler) findAPIResource(gvk schema.GroupVersionKind) (*metav1.APIResource, error) {
	apiResourceList, err := kh.discoveryClient.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return nil, fmt.Errorf("failed to discover server resources for GVG %s: %w", gvk.GroupVersion().String(), err)
	}

	for i := range apiResourceList.APIResources {
		resource := &apiResourceList.APIResources[i] // Get a pointer to the resource
		if resource.Kind == gvk.Kind {
			// The discovered resource already has Group and Version, but let's ensure GVK matches
			// The primary check is Kind. Group and Version are inherent to the list fetched.
			// We need to return the GroupVersionResource for the dynamic client.
			// The metav1.APIResource itself is what we need for its 'Name' (plural) and 'Namespaced' bool.
			return resource, nil
		}
	}
	return nil, fmt.Errorf("resource kind '%s' not found in group version '%s'", gvk.Kind, gvk.GroupVersion().String())
}
