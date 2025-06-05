package kubehandler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	// No K8s imports needed for these specific tests if we are careful
)

// TestApplyManifestFile_ReadFileError tests the scenario where the manifest file cannot be read.
func TestApplyManifestFile_ReadFileError(t *testing.T) {
	t.Helper()
	// Create a KubeHandler instance. For this test, client fields can be nil
	// as the error should occur before any Kubernetes API interaction.
	kh := &KubeHandler{
		clientset:       nil,
		dynamicClient:   nil,
		discoveryClient: nil,
	}

	nonExistentFilePath := filepath.Join(t.TempDir(), "non-existent-manifest.yaml")

	err := kh.ApplyManifestFile(nonExistentFilePath)
	if err == nil {
		t.Fatalf("ApplyManifestFile() was expected to return an error for a non-existent file, but it didn't")
	}

	// Check if the error message indicates a file reading problem
	// os.ErrNotExist would be ideal but it might be wrapped.
	if !strings.Contains(err.Error(), "failed to read manifest file") && !strings.Contains(err.Error(), "no such file or directory") {
		t.Errorf("Expected error message to indicate file read error, got: %v", err)
	}
}

// TestApplyManifestFile_InvalidYAML tests applying a file with invalid YAML content.
func TestApplyManifestFile_InvalidYAML(t *testing.T) {
	t.Helper()
	kh := &KubeHandler{
		clientset:       nil,
		dynamicClient:   nil,
		discoveryClient: nil,
	}

	tempDir := t.TempDir()
	invalidYAMLFilePath := filepath.Join(tempDir, "invalid.yaml")
	invalidYAMLContent := "this: is: not: valid: yaml :-\n  - foo: bar: baz" // Invalid due to unindented mapping item after sequence

	if err := os.WriteFile(invalidYAMLFilePath, []byte(invalidYAMLContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid YAML file: %v", err)
	}

	err := kh.ApplyManifestFile(invalidYAMLFilePath)
	if err == nil {
		t.Fatalf("ApplyManifestFile() was expected to return an error for invalid YAML, but it didn't")
	}

	// Check if the error message indicates a YAML parsing problem
	// The exact error comes from sigs.k8s.io/yaml
	if !strings.Contains(err.Error(), "YAML to JSON conversion failed") && !strings.Contains(err.Error(), "yaml: line ") {
		t.Errorf("Expected error message to indicate YAML parsing error, got: %v", err)
	}
}

// TestApplyManifestFile_EmptyFile tests applying an empty YAML file.
// An empty file, or one with just "---" separators, should not cause an error
// but should also not result in any attempts to apply resources.
func TestApplyManifestFile_EmptyOrWhitespaceYAML(t *testing.T) {
	t.Helper()
	kh := &KubeHandler{
		// No client interactions are expected for empty/whitespace YAML docs
		clientset:       nil,
		dynamicClient:   nil,
		discoveryClient: nil,
	}
	tempDir := t.TempDir()

	testCases := []struct {
		name    string
		content string
	}{
		{"EmptyFile", ""},
		{"OnlySeparator", "---"},
		{"MultipleSeparators", "---\n---\n---"},
		{"WhitespaceAndSeparators", "  \n---\n  \n"},
		{"CommentsOnly", "# This is a comment\n# So is this"},
		{"CommentsAndSeparator", "# Comment\n---\n# Another comment"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, tc.name+".yaml")
			if err := os.WriteFile(filePath, []byte(tc.content), 0644); err != nil {
				t.Fatalf("Failed to write test YAML file %s: %v", filePath, err)
			}

			// Use a distinct variable name for the error from ApplyManifestFile
			applyErr := kh.ApplyManifestFile(filePath)

			switch tc.name {
			case "EmptyFile", "OnlySeparator", "MultipleSeparators", "WhitespaceAndSeparators":
				// These cases should result in no documents being processed, hence no error.
				if applyErr != nil {
					t.Errorf("ApplyManifestFile() with %s was expected to return nil, but got: %v", tc.name, applyErr)
				}
			case "CommentsOnly", "CommentsAndSeparator":
				// These cases result in documents that are not empty strings but parse to "null" or empty objects,
				// which then fail GVK checks. This is an error condition.
				if applyErr == nil {
					t.Errorf("ApplyManifestFile() with %s was expected to return an error, but got nil", tc.name)
					return // Exit this sub-test
				}
				// Check for the specific error.
				// For comment-only YAML, sigs.k8s.io/yaml -> YAMLToJSON produces "null".
				// unstructured.Unstructured.UnmarshalJSON on "null" results in an error like "Object 'Kind' is missing in 'null'".
				expectedErrorContent := "JSON unmarshalling failed" // More general part of the error
				expectedErrorDetail := "Object 'Kind' is missing"   // More specific detail

				if !(strings.Contains(applyErr.Error(), expectedErrorContent) && strings.Contains(applyErr.Error(), expectedErrorDetail)) {
					t.Errorf("ApplyManifestFile() with %s expected error to contain '%s' and '%s', but got: %v", tc.name, expectedErrorContent, expectedErrorDetail, applyErr)
				}
				if !strings.Contains(applyErr.Error(), "doc #") {
					t.Errorf("Error message for %s did not seem to refer to a document: %v", tc.name, applyErr)
				}
			default:
				t.Fatalf("Unhandled test case name: %s", tc.name)
			}
			// Removed extra closing brace that was here
		})
	}
}
