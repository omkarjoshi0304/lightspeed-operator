/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	_ "embed"
	"fmt"

	common_helper "github.com/openstack-k8s-operators/lib-common/modules/common/helper"
	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// systemPrompt - system prompt tailored to the needs of OpenStack Lightspeed.
//
//go:embed assets/system_prompt.txt
var systemPrompt string

// mcpServerConfigTemplate stores the embedded config template for the MCP server.
//
//go:embed assets/mcp_server_config.yaml.tmpl
var mcpServerConfigTemplate string

// getSystemPrompt returns the OpenStackLightspeed system prompt
func getSystemPrompt() string {
	return systemPrompt
}

// lcoreProvider represents an LLM provider configuration.
type lcoreProvider struct {
	Name                string
	URL                 string
	Type                string
	CredentialsSecret   string
	Models              []lcoreModel
	AzureDeploymentName string
	APIVersion          string
	WatsonProjectID     string
}

// lcoreModel represents a model configuration.
type lcoreModel struct {
	Name                 string
	MaxTokensForResponse int
}

// buildProvider creates an lcoreProvider from an OpenStackLightspeed instance.
func buildProvider(instance *apiv1beta1.OpenStackLightspeed) lcoreProvider {
	return lcoreProvider{
		Name:              OpenStackLightspeedDefaultProvider,
		URL:               instance.Spec.LLMEndpoint,
		Type:              instance.Spec.LLMEndpointType,
		CredentialsSecret: instance.Spec.LLMCredentials,
		Models: []lcoreModel{
			{
				Name:                 instance.Spec.ModelName,
				MaxTokensForResponse: instance.Spec.MaxTokensForResponse,
			},
		},
		AzureDeploymentName: instance.Spec.LLMDeploymentName,
		APIVersion:          instance.Spec.LLMAPIVersion,
		WatsonProjectID:     instance.Spec.LLMProjectID,
	}
}

func buildLCoreServiceConfig(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	return map[string]interface{}{
		"host":         "0.0.0.0",
		"port":         OpenStackLightspeedAppServerContainerPort,
		"auth_enabled": true,
		"workers":      1,
		"color_log":    false,
		"access_log":   true,
		"tls_config": map[string]interface{}{
			"tls_certificate_path": OpenStackLightspeedTLSCertPath,
			"tls_key_path":         OpenStackLightspeedTLSKeyPath,
		},
	}
}

func buildLCoreLlamaStackConfig() map[string]interface{} {
	llamaStackConfig := map[string]interface{}{
		"use_as_library_client": false,
		"url":                   fmt.Sprintf("http://localhost:%d", LlamaStackContainerPort),
	}

	return llamaStackConfig
}

func buildLCoreUserDataCollectionConfig(_ *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	feedbackEnabled := instance.Spec.FeedbackEnabled == nil || *instance.Spec.FeedbackEnabled
	transcriptsEnabled := instance.Spec.TranscriptsEnabled

	return map[string]interface{}{
		"feedback_enabled":    feedbackEnabled,
		"feedback_storage":    LCoreUserDataMountPath + "/feedback",
		"transcripts_enabled": transcriptsEnabled,
		"transcripts_storage": LCoreUserDataMountPath + "/transcripts",
	}
}

func buildLCoreAuthenticationConfig(_ *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	return map[string]interface{}{
		"module":                 "k8s",
		"skip_for_health_probes": true,
	}
}

// buildLCoreDatabaseConfig configures persistent database storage (PostgreSQL)
func buildLCoreDatabaseConfig(h *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	return map[string]interface{}{
		"postgres": map[string]interface{}{
			"host":         PostgresServiceName + "." + h.GetBeforeObject().GetNamespace() + ".svc",
			"port":         PostgresServicePort,
			"db":           PostgresLightspeedStackDbName,
			"user":         "${env.POSTGRESQL_USER}",
			"ssl_mode":     PostgresDefaultSSLMode,
			"gss_encmode":  "disable",
			"ca_cert_path": CABundleMountPath,

			// Environment variable substitution via llama_stack.core.stack.replace_env_vars
			"password": "${env.POSTGRESQL_PASSWORD}",

			// Separate schema for LCore to avoid conflicts with App Server
			"namespace": "lcore",
		},
	}
}

// buildLCoreCustomizationConfig configures system prompt customization
// Uses config field if set, otherwise falls back to default
func buildLCoreCustomizationConfig() map[string]interface{} {
	return map[string]interface{}{
		"system_prompt": getSystemPrompt(),
		// Prevent users from overriding via API
		"disable_query_system_prompt": true,
	}
}

// buildLCoreConversationCacheConfig configures chat history caching (PostgreSQL)
func buildLCoreConversationCacheConfig(h *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	return map[string]interface{}{
		"type": "postgres",
		"postgres": map[string]interface{}{
			"host":         PostgresServiceName + "." + h.GetBeforeObject().GetNamespace() + ".svc",
			"port":         PostgresServicePort,
			"db":           PostgresLightspeedStackDbName,
			"user":         "${env.POSTGRESQL_USER}",
			"password":     "${env.POSTGRESQL_PASSWORD}",
			"ssl_mode":     PostgresDefaultSSLMode,
			"gss_encmode":  "disable",
			"ca_cert_path": CABundleMountPath,
			"namespace":    "conversation_cache",
		},
	}
}

// pythonToolProviderType maps operator provider type names to the type names
// expected by the llama_stack_configuration.py Python tool's PROVIDER_TYPE_MAP.
var pythonToolProviderType = map[string]string{
	OpenAIProviderName:      "openai",
	GeminiProviderName:      "vertexai",
	RHOAIVLLMProviderName:   "vllm_rhaiis",
	RHELAIVLLMProviderName:  "vllm_rhel_ai",
	AzureOpenAIProviderName: "azure",
	WatsonXProviderName:     "watsonx",
}

// buildInferenceProvidersConfig builds the high-level inference.providers list
// that the llama_stack_configuration.py synthesis tool uses to generate the
// full OGX inference provider config.
func buildInferenceProvidersConfig(_ *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	provider := buildProvider(instance)
	envVarName := providerNameToEnvVarName(provider.Name)

	toolType, ok := pythonToolProviderType[provider.Type]
	if !ok {
		toolType = provider.Type
	}

	providerEntry := map[string]interface{}{
		"type":        toolType,
		"api_key_env": envVarName + EnvVarSuffixAPIKey,
	}

	extra := map[string]interface{}{}

	if provider.URL != "" {
		extra["base_url"] = provider.URL
	}

	switch provider.Type {
	case AzureOpenAIProviderName:
		if provider.AzureDeploymentName != "" {
			extra["deployment_name"] = provider.AzureDeploymentName
		}
		if provider.APIVersion != "" {
			extra["api_version"] = provider.APIVersion
		}
		extra["client_id"] = fmt.Sprintf("${env.%s_CLIENT_ID:=}", envVarName)
		extra["tenant_id"] = fmt.Sprintf("${env.%s_TENANT_ID:=}", envVarName)
		extra["client_secret"] = fmt.Sprintf("${env.%s_CLIENT_SECRET:=}", envVarName)
	case WatsonXProviderName:
		if provider.WatsonProjectID != "" {
			extra["project_id"] = provider.WatsonProjectID
		}
	}

	if len(extra) > 0 {
		providerEntry["extra"] = extra
	}

	modelNames := make([]interface{}, 0, len(provider.Models))
	for _, m := range provider.Models {
		modelNames = append(modelNames, m.Name)
	}
	if len(modelNames) > 0 {
		providerEntry["allowed_models"] = modelNames
	}

	return map[string]interface{}{
		"default_provider": provider.Name,
		"default_model":    instance.Spec.ModelName,
		"providers": []interface{}{
			providerEntry,
		},
	}
}

// buildNativeOverrideConfig builds overrides deep-merged onto default_run.yaml during synthesis.
func buildNativeOverrideConfig(_ *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	namespace := instance.GetNamespace()

	return map[string]interface{}{
		"image_name":             "openstack-lightspeed-configuration",
		"external_providers_dir": ExternalProvidersDir,
		"server": map[string]interface{}{
			"host":         "0.0.0.0",
			"port":         LlamaStackContainerPort,
			"auth":         nil,
			"quota":        nil,
			"tls_cafile":   nil,
			"tls_certfile": nil,
			"tls_keyfile":  nil,
		},
		"storage": map[string]interface{}{
			"backends": map[string]interface{}{
				"postgres_backend": map[string]interface{}{
					"type":     "sql_postgres",
					"host":     fmt.Sprintf("%s.%s.svc", PostgresServiceName, namespace),
					"port":     PostgresServicePort,
					"user":     "${env.POSTGRESQL_USER}",
					"password": "${env.POSTGRESQL_PASSWORD}",
					"db":       PostgresLlamaStackDbName,
				},
				"kv_default": map[string]interface{}{
					"db_path": "/tmp/llama-stack/kv_store.db",
				},
				"sql_default": map[string]interface{}{
					"db_path": "/tmp/llama-stack/sql_store.db",
				},
			},
			"stores": map[string]interface{}{
				"conversations": map[string]interface{}{
					"backend": "postgres_backend",
				},
			},
		},
		"telemetry": map[string]interface{}{
			"enabled": false,
		},
		"scoring_fns": []interface{}{},
		"safety": map[string]interface{}{
			"default_shield_id": nil,
		},
		"vector_stores": map[string]interface{}{
			"default_embedding_model": nil,
		},
		"registered_resources": map[string]interface{}{
			"shields": []interface{}{},
		},
		"providers": map[string]interface{}{
			"safety": []interface{}{},
			"files": []interface{}{
				map[string]interface{}{
					"provider_id":   "meta-reference-files",
					"provider_type": "inline::localfs",
					"config": map[string]interface{}{
						"storage_dir": "/tmp/llama-stack/files",
						"metadata_store": map[string]interface{}{
							"backend":    "sql_default",
							"table_name": "files_metadata",
						},
					},
				},
			},
		},
	}
}

// isDataCollectionEnabled returns true if at least one of feedback or transcripts is enabled.
func isDataCollectionEnabled(instance *apiv1beta1.OpenStackLightspeed) bool {
	return (instance.Spec.FeedbackEnabled == nil || *instance.Spec.FeedbackEnabled) || instance.Spec.TranscriptsEnabled
}

// buildExporterConfigMap creates the ConfigMap for the dataverse exporter sidecar.
func buildExporterConfigMap(h *common_helper.Helper, _ *apiv1beta1.OpenStackLightspeed) *corev1.ConfigMap {
	exporterConfig := fmt.Sprintf(`service_id: "%s"
ingress_server_url: "https://console.redhat.com/api/ingress/v1/upload"
allowed_subdirs:
  - feedback
  - transcripts
  - config_status
collection_interval: 300
cleanup_after_send: true
ingress_connection_timeout: 30
`, ServiceIDRHOSO)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ExporterConfigCmName,
			Namespace: h.GetBeforeObject().GetNamespace(),
			Labels:    generateAppServerSelectorLabels(),
		},
		Data: map[string]string{
			ExporterConfigFilename: exporterConfig,
		},
	}
}

func buildOKPConfig(ctx context.Context, h *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) map[string]interface{} {
	offline := true
	if instance.Spec.OKP != nil && instance.Spec.OKP.Offline != nil {
		offline = *instance.Spec.OKP.Offline
	}

	return map[string]interface{}{
		"rhokp_url":          "${env.RH_SERVER_OKP}",
		"offline":            offline,
		"chunk_filter_query": getOKPChunkFilterQuery(ctx, h, instance),
	}
}

// buildLCoreMCPServersConfig generates the mcp_servers section for lightspeed-stack config.
// The OpenShift MCP (rhoso-ocp-tools) is always included.
// The OpenStack MCP (rhoso-osp-tools) is only included when openStackReady is true.
func buildLCoreMCPServersConfig(openStackReady bool) []interface{} {
	mcpServers := []interface{}{
		map[string]interface{}{
			"name": "rhoso-ocp-tools",
			"url":  fmt.Sprintf("%s/openshift/", GetMCPServerURL()),
			"authorization_headers": map[string]interface{}{
				"OCP_TOKEN": "kubernetes",
			},
		},
	}

	if openStackReady {
		mcpServers = append(mcpServers, map[string]interface{}{
			"name": "rhoso-osp-tools",
			"url":  fmt.Sprintf("%s/openstack/", GetMCPServerURL()),
		})
	}

	return mcpServers
}

func buildLCoreMCPServersConfigIfEnabled(instance *apiv1beta1.OpenStackLightspeed) ([]interface{}, error) {
	enabled, err := isRHOSOMCPEnabled(instance)
	if err != nil {
		return nil, fmt.Errorf("failed to parse dev config: %w", err)
	}
	if !enabled {
		return []interface{}{}, nil
	}
	return buildLCoreMCPServersConfig(instance.Status.OpenStackReady), nil
}

// buildLCoreConfigYAML assembles the complete Lightspeed Core Service configuration and converts to YAML.
// NOTE: quota handlers, and tools approval features are disabled for OpenStack Lightspeed.
func buildLCoreConfigYAML(ctx context.Context, h *common_helper.Helper, instance *apiv1beta1.OpenStackLightspeed) (string, error) {

	ragInline := []interface{}{"okp"}
	ragConfig := map[string]interface{}{
		"inline": ragInline,
	}

	mcpServers, err := buildLCoreMCPServersConfigIfEnabled(instance)
	if err != nil {
		return "", err
	}

	llamaStackConfig := buildLCoreLlamaStackConfig()
	llamaStackConfig["config"] = map[string]interface{}{
		"native_override": buildNativeOverrideConfig(h, instance),
	}

	// Build the complete config as a map
	config := map[string]interface{}{
		"name":                 "Lightspeed Core Service (LCS)",
		"service":              buildLCoreServiceConfig(h, instance),
		"llama_stack":          llamaStackConfig,
		"user_data_collection": buildLCoreUserDataCollectionConfig(h, instance),
		"authentication":       buildLCoreAuthenticationConfig(h, instance),
		"inference":            buildInferenceProvidersConfig(h, instance),
		"database":             buildLCoreDatabaseConfig(h, instance),
		"customization":        buildLCoreCustomizationConfig(),
		"conversation_cache":   buildLCoreConversationCacheConfig(h, instance),
		"byok_rag":             []interface{}{},
		"rag":                  ragConfig,
		"mcp_servers":          mcpServers,
	}

	config["okp"] = buildOKPConfig(ctx, h, instance)

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal LCore config to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
