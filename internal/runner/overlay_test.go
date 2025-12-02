package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rkoster/deskrun/arcembedded"
	"github.com/rkoster/deskrun/internal/kapp"
	"github.com/rkoster/deskrun/pkg/types"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ytt Overlay Processing", func() {
	var (
		manager    *Manager
		tmpDir     string
		kappClient *kapp.Client
	)

	BeforeEach(func() {
		var err error
		manager = &Manager{}
		tmpDir, err = os.MkdirTemp("/tmp", "overlay-test-*")
		Expect(err).NotTo(HaveOccurred())
		kappClient = kapp.NewClient("test", "test-namespace")
	})

	AfterEach(func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	})

	Describe("Container Mode Configurations", func() {
		var baseInstallation *types.RunnerInstallation

		BeforeEach(func() {
			baseInstallation = &types.RunnerInstallation{
				Name:       "test-runner",
				Repository: "https://github.com/owner/repo",
				AuthType:   types.AuthTypePAT,
				AuthValue:  "test-token",
				MinRunners: 1,
				MaxRunners: 3,
				Instances:  1,
			}
		})

		Context("Kubernetes Container Mode", func() {
			BeforeEach(func() {
				baseInstallation.ContainerMode = types.ContainerModeKubernetes
			})

			It("should generate correct ytt data values for kubernetes mode", func() {
				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
				err := manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Read the generated data values
				dataValuesContent, err := os.ReadFile(dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				dataValuesString := string(dataValuesContent)

				Expect(dataValuesString).To(ContainSubstring("containerMode: kubernetes"))
				Expect(dataValuesString).To(ContainSubstring("cachePaths: []"))
				Expect(dataValuesString).To(ContainSubstring("installation:"))
				Expect(dataValuesString).NotTo(ContainSubstring("source:"))
				Expect(dataValuesString).NotTo(ContainSubstring("target:"))
			})

			It("should apply kubernetes-specific overlay transformations", func() {
				// Setup template directory
				templateDir, err := manager.setupYttTemplateDir(baseInstallation, tmpDir)
				Expect(err).NotTo(HaveOccurred())

				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
				err = manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Process with ytt
				processedYAML, err := kappClient.ProcessTemplate(templateDir, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())

				// Parse the YAML to verify kubernetes-specific configurations
				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(processedYAML))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break // EOF or end of documents
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

				// Find AutoscalingRunnerSet
				var autoscalingRunnerSet map[string]interface{}
				for _, resource := range resources {
					if resource["kind"] == "AutoscalingRunnerSet" {
						autoscalingRunnerSet = resource
						break
					}
				}
				Expect(autoscalingRunnerSet).NotTo(BeNil(), "AutoscalingRunnerSet should be present")

				// Verify kubernetes-specific environment variables
				spec := autoscalingRunnerSet["spec"].(map[string]interface{})
				template := spec["template"].(map[string]interface{})
				podSpec := template["spec"].(map[string]interface{})
				containers := podSpec["containers"].([]interface{})

				runnerContainer := containers[0].(map[string]interface{})
				Expect(runnerContainer["name"]).To(Equal("runner"))

				env := runnerContainer["env"].([]interface{})
				hasRequireJobContainer := false
				for _, envVar := range env {
					envMap := envVar.(map[string]interface{})
					if envMap["name"] == "ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER" {
						hasRequireJobContainer = true
						Expect(envMap["value"]).To(Equal("true"))
					}
				}
				Expect(hasRequireJobContainer).To(BeTrue(), "Should have ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER=true")

				// Verify no DinD container
				Expect(len(containers)).To(Equal(1), "Should only have runner container")

				// Verify no privileged security context
				_, hasSecurityContext := runnerContainer["securityContext"]
				Expect(hasSecurityContext).To(BeFalse(), "Runner container should not have security context in kubernetes mode")
			})
		})

		Context("DinD Container Mode", func() {
			BeforeEach(func() {
				baseInstallation.ContainerMode = types.ContainerModeDinD
			})

			It("should generate correct ytt data values for dind mode", func() {
				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
				err := manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Read the generated data values
				dataValuesContent, err := os.ReadFile(dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				dataValuesString := string(dataValuesContent)

				Expect(dataValuesString).To(ContainSubstring("containerMode: dind"))
				Expect(dataValuesString).To(ContainSubstring("cachePaths: []"))
				Expect(dataValuesString).To(ContainSubstring("installation:"))
			})

			It("should apply dind-specific overlay transformations", func() {
				// Setup template directory
				templateDir, err := manager.setupYttTemplateDir(baseInstallation, tmpDir)
				Expect(err).NotTo(HaveOccurred())

				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
				err = manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Process with ytt
				processedYAML, err := kappClient.ProcessTemplate(templateDir, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())

				// Parse the YAML to verify dind-specific configurations
				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(processedYAML))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

				// Find AutoscalingRunnerSet
				var autoscalingRunnerSet map[string]interface{}
				for _, resource := range resources {
					if resource["kind"] == "AutoscalingRunnerSet" {
						autoscalingRunnerSet = resource
						break
					}
				}
				Expect(autoscalingRunnerSet).NotTo(BeNil())

				spec := autoscalingRunnerSet["spec"].(map[string]interface{})
				template := spec["template"].(map[string]interface{})
				podSpec := template["spec"].(map[string]interface{})
				containers := podSpec["containers"].([]interface{})

				// Verify runner container environment variables
				runnerContainer := containers[0].(map[string]interface{})
				Expect(runnerContainer["name"]).To(Equal("runner"))

				env := runnerContainer["env"].([]interface{})
				hasDockerHost := false
				for _, envVar := range env {
					envMap := envVar.(map[string]interface{})
					if envMap["name"] == "DOCKER_HOST" {
						hasDockerHost = true
						Expect(envMap["value"]).To(Equal("tcp://localhost:2375"))
					}
				}
				Expect(hasDockerHost).To(BeTrue(), "Should have DOCKER_HOST environment variable")

				// Verify runner container volume mounts
				volumeMounts := runnerContainer["volumeMounts"].([]interface{})
				hasWorkMount := false
				for _, mount := range volumeMounts {
					mountMap := mount.(map[string]interface{})
					if mountMap["name"] == "work" && mountMap["mountPath"] == "/home/runner/_work" {
						hasWorkMount = true
					}
				}
				Expect(hasWorkMount).To(BeTrue(), "Should have work volume mount")

				// Verify DinD container exists
				Expect(len(containers)).To(Equal(2), "Should have both runner and dind containers")
				dindContainer := containers[1].(map[string]interface{})
				Expect(dindContainer["name"]).To(Equal("dind"))
				Expect(dindContainer["image"]).To(Equal("docker:dind"))

				// Verify DinD container security context
				dindSecurityContext := dindContainer["securityContext"].(map[string]interface{})
				Expect(dindSecurityContext["privileged"]).To(BeTrue())

				// Verify volumes
				volumes := podSpec["volumes"].([]interface{})
				expectedVolumes := []string{"work", "dind-storage"}
				actualVolumeNames := make([]string, len(volumes))
				for i, vol := range volumes {
					volMap := vol.(map[string]interface{})
					actualVolumeNames[i] = volMap["name"].(string)
				}
				for _, expectedVol := range expectedVolumes {
					Expect(actualVolumeNames).To(ContainElement(expectedVol))
				}
			})
		})

		Context("Privileged Container Mode", func() {
			BeforeEach(func() {
				baseInstallation.ContainerMode = types.ContainerModePrivileged
				baseInstallation.CachePaths = []types.CachePath{
					{Source: "/nix/store", Target: "/nix/store-host"},
					{Source: "/nix/var/nix/daemon-socket", Target: "/nix/var/nix/daemon-socket-host"},
					{Source: "", Target: "/var/lib/docker"},
				}
			})

			It("should generate correct ytt data values for privileged mode", func() {
				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
				err := manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Read the generated data values
				dataValuesContent, err := os.ReadFile(dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				dataValuesString := string(dataValuesContent)

				Expect(dataValuesString).To(ContainSubstring("containerMode: cached-privileged-kubernetes"))
				Expect(dataValuesString).To(ContainSubstring("cachePaths:"))
				Expect(dataValuesString).To(ContainSubstring("installation:"))
				Expect(dataValuesString).To(ContainSubstring("- source: /nix/store"))
				Expect(dataValuesString).To(ContainSubstring("target: /nix/store-host"))
				Expect(dataValuesString).To(ContainSubstring("- source: /nix/var/nix/daemon-socket"))
				Expect(dataValuesString).To(ContainSubstring("target: /nix/var/nix/daemon-socket-host"))
				Expect(dataValuesString).To(ContainSubstring(`- source: ""`))
				Expect(dataValuesString).To(ContainSubstring("target: /var/lib/docker"))
			})

			It("should apply privileged-specific overlay transformations", func() {
				// Setup template directory
				templateDir, err := manager.setupYttTemplateDir(baseInstallation, tmpDir)
				Expect(err).NotTo(HaveOccurred())

				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
				err = manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Process with ytt
				processedYAML, err := kappClient.ProcessTemplate(templateDir, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())

				// Parse the YAML
				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(processedYAML))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

				// Find AutoscalingRunnerSet
				var autoscalingRunnerSet map[string]interface{}
				for _, resource := range resources {
					if resource["kind"] == "AutoscalingRunnerSet" {
						autoscalingRunnerSet = resource
						break
					}
				}
				Expect(autoscalingRunnerSet).NotTo(BeNil())

				spec := autoscalingRunnerSet["spec"].(map[string]interface{})
				template := spec["template"].(map[string]interface{})
				podSpec := template["spec"].(map[string]interface{})

				// Verify pod-level security context
				podSecurityContext := podSpec["securityContext"].(map[string]interface{})
				Expect(podSecurityContext["fsGroup"]).To(Equal(123))

				containers := podSpec["containers"].([]interface{})
				runnerContainer := containers[0].(map[string]interface{})
				Expect(runnerContainer["name"]).To(Equal("runner"))

				// Verify runner container security context
				runnerSecurityContext := runnerContainer["securityContext"].(map[string]interface{})
				Expect(runnerSecurityContext["privileged"]).To(BeTrue())

				// Verify privileged-specific environment variables
				env := runnerContainer["env"].([]interface{})
				expectedEnvVars := map[string]string{
					"ACTIONS_RUNNER_CONTAINER_HOOKS":         "/home/runner/k8s-novolume/index.js",
					"ACTIONS_RUNNER_CONTAINER_HOOK_TEMPLATE": "/etc/hooks/content",
					"ACTIONS_RUNNER_REQUIRE_JOB_CONTAINER":   "false",
				}

				for expectedName, expectedValue := range expectedEnvVars {
					found := false
					for _, envVar := range env {
						envMap := envVar.(map[string]interface{})
						if envMap["name"] == expectedName {
							found = true
							Expect(envMap["value"]).To(Equal(expectedValue))
							break
						}
					}
					Expect(found).To(BeTrue(), fmt.Sprintf("Should have environment variable %s", expectedName))
				}

				// Verify volume mounts including cache paths
				volumeMounts := runnerContainer["volumeMounts"].([]interface{})
				expectedMounts := map[string]string{
					"docker-socket":  "/var/run/docker.sock",
					"hook-extension": "/etc/hooks",
					"cache-0":        "/nix/store-host",
					"cache-1":        "/nix/var/nix/daemon-socket-host",
					"cache-2":        "/var/lib/docker",
				}

				for expectedName, expectedPath := range expectedMounts {
					found := false
					for _, mount := range volumeMounts {
						mountMap := mount.(map[string]interface{})
						if mountMap["name"] == expectedName {
							found = true
							Expect(mountMap["mountPath"]).To(Equal(expectedPath))
							break
						}
					}
					Expect(found).To(BeTrue(), fmt.Sprintf("Should have volume mount %s at %s", expectedName, expectedPath))
				}

				// Verify volumes including cache paths
				volumes := podSpec["volumes"].([]interface{})
				expectedVolumes := map[string]string{
					"docker-socket":  "hostPath",
					"hook-extension": "configMap",
					"cache-0":        "hostPath",
					"cache-1":        "hostPath",
					"cache-2":        "hostPath",
				}

				for expectedName, expectedType := range expectedVolumes {
					found := false
					for _, vol := range volumes {
						volMap := vol.(map[string]interface{})
						if volMap["name"] == expectedName {
							found = true
							_, hasType := volMap[expectedType]
							Expect(hasType).To(BeTrue(), fmt.Sprintf("Volume %s should have %s configuration", expectedName, expectedType))
							break
						}
					}
					Expect(found).To(BeTrue(), fmt.Sprintf("Should have volume %s", expectedName))
				}
			})

			It("should handle cache paths with empty source correctly", func() {
				// This test verifies that empty source paths are handled correctly
				templateDir, err := manager.setupYttTemplateDir(baseInstallation, tmpDir)
				Expect(err).NotTo(HaveOccurred())

				dataValuesPath := filepath.Join(tmpDir, "data-values.yaml")
				err = manager.createDataValuesFile(baseInstallation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				processedYAML, err := kappClient.ProcessTemplate(templateDir, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Verify that the empty source path is preserved in the output
				Expect(processedYAML).To(ContainSubstring(`path: ""`))
				Expect(processedYAML).To(ContainSubstring("type: DirectoryOrCreate"))
			})
		})
	})

	Describe("Data Values Generation", func() {
		It("should generate valid YAML for all container modes", func() {
			installation := &types.RunnerInstallation{
				Name:       "test-runner",
				Repository: "https://github.com/owner/repo",
				AuthType:   types.AuthTypePAT,
				AuthValue:  "test-token",
				MinRunners: 1,
				MaxRunners: 3,
				Instances:  1,
			}

			containerModes := []types.ContainerMode{
				types.ContainerModeKubernetes,
				types.ContainerModeDinD,
				types.ContainerModePrivileged,
			}

			for _, mode := range containerModes {
				installation.ContainerMode = mode
				if mode == types.ContainerModePrivileged {
					installation.CachePaths = []types.CachePath{
						{Source: "/test/source", Target: "/test/target"},
					}
				} else {
					installation.CachePaths = nil
				}

				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, fmt.Sprintf("test-data-values-%s.yaml", mode))
				err := manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed for mode %s", mode))

				// Read and verify the generated data values
				dataValuesContent, err := os.ReadFile(dataValuesPath)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to read data values for mode %s", mode))
				dataValuesString := string(dataValuesContent)

				// Verify it's valid YAML
				var parsed map[string]interface{}
				err = yaml.Unmarshal([]byte(dataValuesString), &parsed)
				Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Invalid YAML for mode %s: %s", mode, dataValuesString))

				// Verify required fields are present
				installation := parsed["installation"].(map[string]interface{})
				Expect(installation["containerMode"]).To(Equal(string(mode)))
				Expect(installation["name"]).To(Equal("test-runner"))
				Expect(installation["repository"]).To(Equal("https://github.com/owner/repo"))
			}
		})

		It("should handle special characters in repository URLs", func() {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo-with-dashes_and_underscores",
				ContainerMode: types.ContainerModeKubernetes,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
				MinRunners:    1,
				MaxRunners:    3,
				Instances:     1,
			}

			// Create data values file
			dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
			err := manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
			Expect(err).NotTo(HaveOccurred())

			// Read the generated data values
			dataValuesContent, err := os.ReadFile(dataValuesPath)
			Expect(err).NotTo(HaveOccurred())
			dataValuesString := string(dataValuesContent)

			Expect(dataValuesString).To(ContainSubstring("repository: https://github.com/owner/repo-with-dashes_and_underscores"))
		})

		It("should handle instance numbers correctly", func() {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModeKubernetes,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
				MinRunners:    1,
				MaxRunners:    3,
				Instances:     5,
			}

			for instanceNum := 1; instanceNum <= 5; instanceNum++ {
				instanceName := fmt.Sprintf("test-runner-%d", instanceNum)

				// Create data values file
				dataValuesPath := filepath.Join(tmpDir, fmt.Sprintf("test-data-values-%d.yaml", instanceNum))
				err := manager.createDataValuesFile(installation, instanceName, instanceNum, dataValuesPath)
				Expect(err).NotTo(HaveOccurred())

				// Read the generated data values
				dataValuesContent, err := os.ReadFile(dataValuesPath)
				Expect(err).NotTo(HaveOccurred())
				dataValuesString := string(dataValuesContent)

				Expect(dataValuesString).To(ContainSubstring(fmt.Sprintf("name: %s", instanceName)))
				Expect(dataValuesString).To(ContainSubstring(fmt.Sprintf("instanceNum: %d", instanceNum)))
			}
		})
	})

	Describe("Template Setup", func() {
		It("should create template directory structure correctly", func() {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModeKubernetes,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
			}

			templateDir, err := manager.setupYttTemplateDir(installation, tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Verify template directory exists
			Expect(templateDir).To(BeADirectory())

			// Verify required files exist
			scaleSetPath := filepath.Join(templateDir, "scale-set.yaml")
			overlayPath := filepath.Join(templateDir, "overlay.yaml")

			Expect(scaleSetPath).To(BeARegularFile())
			Expect(overlayPath).To(BeARegularFile())

			// Verify scale-set template contains ytt data value expressions
			scaleSetContent, err := os.ReadFile(scaleSetPath)
			Expect(err).NotTo(HaveOccurred())
			scaleSetString := string(scaleSetContent)

			Expect(scaleSetString).To(ContainSubstring("#@ data.values.installation.repository"))
			Expect(scaleSetString).To(ContainSubstring("#@ data.values.installation.name"))
			Expect(scaleSetString).To(ContainSubstring("#@ data.values.installation.authValue"))

			// Verify overlay contains ytt overlay directives
			overlayContent, err := os.ReadFile(overlayPath)
			Expect(err).NotTo(HaveOccurred())
			overlayString := string(overlayContent)

			Expect(overlayString).To(ContainSubstring("#@ load(\"@ytt:data\", \"data\")"))
			Expect(overlayString).To(ContainSubstring("#@ load(\"@ytt:overlay\", \"overlay\")"))
			Expect(overlayString).To(ContainSubstring("#@ if data.values.installation.containerMode"))
		})

		It("should read overlay content from embedded files", func() {
			overlayContent, err := arcembedded.GetUniversalOverlay()
			Expect(err).NotTo(HaveOccurred())
			Expect(overlayContent).NotTo(BeEmpty())

			// Verify it contains expected ytt directives
			Expect(overlayContent).To(ContainSubstring("@ytt:overlay"))
			Expect(overlayContent).To(ContainSubstring("containerMode"))
			Expect(overlayContent).To(ContainSubstring("kubernetes"))
			Expect(overlayContent).To(ContainSubstring("dind"))
			Expect(overlayContent).To(ContainSubstring("cached-privileged-kubernetes"))
		})
	})

	Describe("Edge Cases", func() {
		It("should handle empty cache paths array", func() {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModePrivileged,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
				CachePaths:    []types.CachePath{}, // Empty array
			}

			// Create data values file
			dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
			err := manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
			Expect(err).NotTo(HaveOccurred())

			// Read the generated data values
			dataValuesContent, err := os.ReadFile(dataValuesPath)
			Expect(err).NotTo(HaveOccurred())
			dataValuesString := string(dataValuesContent)

			Expect(dataValuesString).To(ContainSubstring("cachePaths: []"))
		})

		It("should handle nil cache paths", func() {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModePrivileged,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "test-token",
				CachePaths:    nil, // nil slice
			}

			// Create data values file
			dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
			err := manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
			Expect(err).NotTo(HaveOccurred())

			// Read the generated data values
			dataValuesContent, err := os.ReadFile(dataValuesPath)
			Expect(err).NotTo(HaveOccurred())
			dataValuesString := string(dataValuesContent)

			Expect(dataValuesString).To(ContainSubstring("cachePaths: []"))
		})

		It("should escape special YAML characters in auth values", func() {
			installation := &types.RunnerInstallation{
				Name:          "test-runner",
				Repository:    "https://github.com/owner/repo",
				ContainerMode: types.ContainerModeKubernetes,
				AuthType:      types.AuthTypePAT,
				AuthValue:     "token:with:colons@and#special!chars",
			}

			// Create data values file
			dataValuesPath := filepath.Join(tmpDir, "test-data-values.yaml")
			err := manager.createDataValuesFile(installation, "test-runner", 0, dataValuesPath)
			Expect(err).NotTo(HaveOccurred())

			// Read the generated data values
			dataValuesContent, err := os.ReadFile(dataValuesPath)
			Expect(err).NotTo(HaveOccurred())
			dataValuesString := string(dataValuesContent)

			// Verify it's still valid YAML
			var parsed map[string]interface{}
			err = yaml.Unmarshal([]byte(dataValuesString), &parsed)
			Expect(err).NotTo(HaveOccurred())

			installationData := parsed["installation"].(map[string]interface{})
			Expect(installationData["authValue"]).To(Equal("token:with:colons@and#special!chars"))
		})
	})
})
