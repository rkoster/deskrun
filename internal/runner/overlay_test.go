package runner

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rkoster/deskrun/pkg/templates"
	"github.com/rkoster/deskrun/pkg/types"
	"gopkg.in/yaml.v3"
)

var _ = Describe("ytt Overlay Processing", func() {
	var (
		processor *templates.Processor
	)

	BeforeEach(func() {
		processor = templates.NewProcessor()
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

			It("should apply kubernetes-specific overlay transformations", func() {
				config := templates.Config{
					Installation: baseInstallation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())

				// Parse the YAML to verify kubernetes-specific configurations
				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
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

			It("should apply dind-specific overlay transformations", func() {
				config := templates.Config{
					Installation: baseInstallation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())

				// Parse the YAML to verify dind-specific configurations
				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
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
						Expect(envMap["value"]).To(Equal("unix:///var/run/docker.sock"))
					}
				}
				Expect(hasDockerHost).To(BeTrue(), "Should have DOCKER_HOST environment variable")

				// Verify runner container volume mounts
				volumeMounts := runnerContainer["volumeMounts"].([]interface{})
				hasWorkMount := false
				hasDindSockMount := false
				for _, mount := range volumeMounts {
					mountMap := mount.(map[string]interface{})
					if mountMap["name"] == "work" && mountMap["mountPath"] == "/home/runner/_work" {
						hasWorkMount = true
					}
					if mountMap["name"] == "dind-sock" && mountMap["mountPath"] == "/var/run" {
						hasDindSockMount = true
					}
				}
				Expect(hasWorkMount).To(BeTrue(), "Should have work volume mount")
				Expect(hasDindSockMount).To(BeTrue(), "Should have dind-sock volume mount for unix socket")

				// Verify DinD init container exists (upstream uses native sidecar pattern with initContainers)
				initContainers := podSpec["initContainers"].([]interface{})
				var dindContainer map[string]interface{}
				for _, ic := range initContainers {
					icMap := ic.(map[string]interface{})
					if icMap["name"] == "dind" {
						dindContainer = icMap
						break
					}
				}
				Expect(dindContainer).NotTo(BeNil(), "Should have dind init container")
				Expect(dindContainer["image"]).To(Equal("docker:dind"))

				// Verify DinD container security context
				dindSecurityContext := dindContainer["securityContext"].(map[string]interface{})
				Expect(dindSecurityContext["privileged"]).To(BeTrue())

				// Verify volumes
				volumes := podSpec["volumes"].([]interface{})
				expectedVolumes := []string{"work", "dind-sock", "dind-externals"}
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

			It("should apply privileged-specific overlay transformations", func() {
				config := templates.Config{
					Installation: baseInstallation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())

				// Parse the YAML
				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
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

				// Verify runner container security context (privileged)
				runnerSecurityContext := runnerContainer["securityContext"].(map[string]interface{})
				Expect(runnerSecurityContext["privileged"]).To(BeTrue())
				Expect(runnerSecurityContext["allowPrivilegeEscalation"]).To(BeTrue())
				Expect(runnerSecurityContext["runAsNonRoot"]).To(BeFalse())

				// Verify privileged environment variables
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

				// Verify volume mounts
				volumeMounts := runnerContainer["volumeMounts"].([]interface{})
				expectedMounts := map[string]string{
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

				// Verify volumes
				volumes := podSpec["volumes"].([]interface{})
				expectedVolumes := map[string]string{
					"hook-extension": "configMap",
					"cache-0":        "hostPath",
					"cache-1":        "hostPath",
					"cache-2":        "emptyDir",
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
				config := templates.Config{
					Installation: baseInstallation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				processedStr := string(processedYAML)
				Expect(processedStr).To(ContainSubstring("privileged: true"))
				Expect(processedStr).To(ContainSubstring("k8s-novolume/index.js"))
			})
		})

		Describe("Template Validation", func() {
			It("should generate valid output for all container modes", func() {
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

					config := templates.Config{
						Installation: installation,
						InstanceName: "test-runner",
						InstanceNum:  0,
					}

					processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed for mode %s", mode))

					// Verify it's valid YAML
					var parsed map[string]interface{}
					err = yaml.Unmarshal(processedYAML, &parsed)
					Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Invalid YAML for mode %s", mode))
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

				config := templates.Config{
					Installation: installation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				Expect(string(processedYAML)).To(ContainSubstring("https://github.com/owner/repo-with-dashes_and_underscores"))
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
					CachePaths:    []types.CachePath{},
				}

				config := templates.Config{
					Installation: installation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())
			})

			It("should handle nil cache paths", func() {
				installation := &types.RunnerInstallation{
					Name:          "test-runner",
					Repository:    "https://github.com/owner/repo",
					ContainerMode: types.ContainerModePrivileged,
					AuthType:      types.AuthTypePAT,
					AuthValue:     "test-token",
					CachePaths:    nil,
				}

				config := templates.Config{
					Installation: installation,
					InstanceName: "test-runner",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(processedYAML).NotTo(BeEmpty())
			})
		})

		Describe("Environment Variable Configuration", func() {
			BeforeEach(func() {
				baseInstallation = &types.RunnerInstallation{
					Name:          "test-env-runner",
					Repository:    "https://github.com/test/repo",
					AuthType:      types.AuthTypePAT,
					AuthValue:     "test-token",
					ContainerMode: types.ContainerModePrivileged,
					CachePaths:    []types.CachePath{{Source: "/test", Target: "/test"}},
					MinRunners:    1,
					MaxRunners:    3,
				}
			})

			It("should set ACTIONS_RUNNER_POD_NAME for cached-privileged-kubernetes mode", func() {
				config := templates.Config{
					Installation: baseInstallation,
					InstanceName: "test-podname",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

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

				runnerContainer := containers[0].(map[string]interface{})
				env := runnerContainer["env"].([]interface{})

				var podNameEnv map[string]interface{}
				for _, envVar := range env {
					envMap := envVar.(map[string]interface{})
					if envMap["name"] == "ACTIONS_RUNNER_POD_NAME" {
						podNameEnv = envMap
						break
					}
				}

				Expect(podNameEnv).NotTo(BeNil(), "ACTIONS_RUNNER_POD_NAME environment variable should be present")

				valueFrom := podNameEnv["valueFrom"].(map[string]interface{})
				fieldRef := valueFrom["fieldRef"].(map[string]interface{})
				fieldPath := fieldRef["fieldPath"].(string)

				Expect(fieldPath).To(Equal("metadata.name"), "ACTIONS_RUNNER_POD_NAME should use metadata.name fieldRef")
			})

			It("should have ACTIONS_RUNNER_POD_NAME for kubernetes mode (upstream default)", func() {
				// NOTE: Upstream kubernetes mode now includes ACTIONS_RUNNER_POD_NAME by default
				// This is expected behavior from the Helm template
				installation := &types.RunnerInstallation{
					Name:          "test-no-podname",
					Repository:    "https://github.com/test/repo",
					AuthType:      types.AuthTypePAT,
					AuthValue:     "test-token",
					ContainerMode: types.ContainerModeKubernetes,
					CachePaths:    []types.CachePath{},
					MinRunners:    1,
					MaxRunners:    3,
				}

				config := templates.Config{
					Installation: installation,
					InstanceName: "test-no-podname",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

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

				runnerContainer := containers[0].(map[string]interface{})
				env := runnerContainer["env"].([]interface{})

				// Kubernetes mode should have ACTIONS_RUNNER_POD_NAME (upstream default)
				var podNameEnv map[string]interface{}
				for _, envVar := range env {
					envMap := envVar.(map[string]interface{})
					if envMap["name"] == "ACTIONS_RUNNER_POD_NAME" {
						podNameEnv = envMap
						break
					}
				}
				Expect(podNameEnv).NotTo(BeNil(), "ACTIONS_RUNNER_POD_NAME should be present for kubernetes mode (upstream default)")
			})

			It("should not have ACTIONS_RUNNER_POD_NAME for dind mode", func() {
				installation := &types.RunnerInstallation{
					Name:          "test-no-podname",
					Repository:    "https://github.com/test/repo",
					AuthType:      types.AuthTypePAT,
					AuthValue:     "test-token",
					ContainerMode: types.ContainerModeDinD,
					CachePaths:    []types.CachePath{},
					MinRunners:    1,
					MaxRunners:    3,
				}

				config := templates.Config{
					Installation: installation,
					InstanceName: "test-no-podname",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

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

				runnerContainer := containers[0].(map[string]interface{})
				env := runnerContainer["env"].([]interface{})

				// DinD mode should NOT have ACTIONS_RUNNER_POD_NAME
				for _, envVar := range env {
					envMap := envVar.(map[string]interface{})
					Expect(envMap["name"]).NotTo(Equal("ACTIONS_RUNNER_POD_NAME"),
						"ACTIONS_RUNNER_POD_NAME should not be present for dind mode")
				}
			})

			It("should include all required hook environment variables for cached-privileged-kubernetes mode", func() {
				config := templates.Config{
					Installation: baseInstallation,
					InstanceName: "test-hook-vars",
					InstanceNum:  0,
				}

				processedYAML, err := processor.ProcessTemplate(templates.TemplateTypeScaleSet, config)
				Expect(err).NotTo(HaveOccurred())

				var resources []map[string]interface{}
				decoder := yaml.NewDecoder(strings.NewReader(string(processedYAML)))
				for {
					var resource map[string]interface{}
					if err := decoder.Decode(&resource); err != nil {
						break
					}
					if resource != nil {
						resources = append(resources, resource)
					}
				}

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

				runnerContainer := containers[0].(map[string]interface{})
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
							Expect(envMap["value"]).To(Equal(expectedValue),
								"Environment variable %s should have value %s", expectedName, expectedValue)
							break
						}
					}
					Expect(found).To(BeTrue(), "Environment variable %s should be present", expectedName)
				}

				var podNameEnvFound bool
				for _, envVar := range env {
					envMap := envVar.(map[string]interface{})
					if envMap["name"] == "ACTIONS_RUNNER_POD_NAME" {
						podNameEnvFound = true
						valueFrom := envMap["valueFrom"].(map[string]interface{})
						fieldRef := valueFrom["fieldRef"].(map[string]interface{})
						fieldPath := fieldRef["fieldPath"].(string)
						Expect(fieldPath).To(Equal("metadata.name"), "ACTIONS_RUNNER_POD_NAME should use metadata.name fieldRef")
						break
					}
				}
				Expect(podNameEnvFound).To(BeTrue(), "ACTIONS_RUNNER_POD_NAME should be present with fieldRef")
			})
		})
	})
})
