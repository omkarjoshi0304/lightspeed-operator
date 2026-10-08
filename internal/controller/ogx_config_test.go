package controller

import (
	"context"
	"fmt"

	apiv1beta1 "github.com/openstack-k8s-operators/lightspeed-operator/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func expectSentenceTransformersProvider(providers []interface{}) {
	sentenceTransformers := providers[0].(map[string]interface{})
	gomega.Expect(sentenceTransformers["provider_id"]).To(gomega.Equal("sentence-transformers"))
	gomega.Expect(sentenceTransformers["provider_type"]).To(gomega.Equal("inline::sentence-transformers"))
}

func getOpenStackLightspeedProvidersInstance(provider string) *apiv1beta1.OpenStackLightspeed {
	instance := &apiv1beta1.OpenStackLightspeed{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openstack-lightspeed",
			Namespace: "openstack-lightspeed",
		},
		Spec: apiv1beta1.OpenStackLightspeedSpec{
			OpenStackLightspeedCore: apiv1beta1.OpenStackLightspeedCore{
				DefaultModel: "default-model",
			},
		},
	}

	model := apiv1beta1.OpenStackLightspeedModelSpec{
		Name:      "default-model",
		ModelName: "gpt-4o",
	}

	switch provider {
	case OpenAIProviderName:
		model.LLMEndpointType = OpenAIProviderName
		model.LLMEndpoint = "https://api.openai.com/v1"
	case GeminiProviderName:
		model.LLMEndpointType = GeminiProviderName
		model.ModelName = "gemini-2.0-flash"
	case RHOAIVLLMProviderName:
		model.LLMEndpointType = RHOAIVLLMProviderName
		model.LLMEndpoint = "https://vllm.example.com/v1"
		model.ModelName = "meta-llama/Llama-3.1-70B-Instruct"
	case RHELAIVLLMProviderName:
		model.LLMEndpointType = RHELAIVLLMProviderName
		model.LLMEndpoint = "https://rhelai-vllm.example.com/v1"
		model.ModelName = "meta-llama/Llama-3.1-70B-Instruct"
	case AzureOpenAIProviderName:
		model.LLMEndpointType = AzureOpenAIProviderName
		model.LLMEndpoint = "https://my-resource.openai.azure.com"
		model.LLMDeploymentName = "gpt-4o-deployment"
		model.LLMAPIVersion = "2024-02-01"
	case WatsonXProviderName:
		model.LLMEndpointType = WatsonXProviderName
		model.LLMEndpoint = "https://watsonx.example.com"
		model.LLMProjectID = "test-project-id"
		model.ModelName = "ibm/granite-13b-chat-v2"
	default:
		ginkgo.Fail(fmt.Sprintf("Unknown provider %s", provider))
		return nil
	}

	instance.Spec.Models = []apiv1beta1.OpenStackLightspeedModelSpec{model}
	return instance
}

func checkModelCommonConfig(modelConfig map[string]interface{}, model apiv1beta1.OpenStackLightspeedModelSpec) {
	gomega.Expect(modelConfig["model_id"]).To(gomega.Equal(model.Name))
	gomega.Expect(modelConfig["model_type"]).To(gomega.Equal("llm"))
	gomega.Expect(modelConfig["provider_id"]).To(gomega.Equal(modelProviderName(model.Name)))
	gomega.Expect(modelConfig["provider_model_id"]).To(gomega.Equal(model.ModelName))
	gomega.Expect(modelConfig).NotTo(gomega.HaveKey("metadata"))
}

var _ = ginkgo.Describe("OGX config", func() {
	ginkgo.Describe("buildOGXInferenceProviders", func() {
		ginkgo.DescribeTable("should return correct inference providers config",
			func(provider, providerType string, checkConfig func(map[string]interface{}, *apiv1beta1.OpenStackLightspeed)) {
				instance := getOpenStackLightspeedProvidersInstance(provider)
				inferenceProvidersConfig, err := buildOGXInferenceProviders(context.Background(), nil, instance)

				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(inferenceProvidersConfig).To(gomega.HaveLen(2))

				expectSentenceTransformersProvider(inferenceProvidersConfig)

				inferenceProvider := inferenceProvidersConfig[1].(map[string]interface{})
				gomega.Expect(inferenceProvider["provider_id"]).To(gomega.Equal(modelProviderName("default-model")))
				gomega.Expect(inferenceProvider["provider_type"]).To(gomega.Equal(providerType))

				checkConfig(inferenceProvider["config"].(map[string]interface{}), instance)
			},
			ginkgo.Entry("for openai", OpenAIProviderName, "remote::openai",
				func(config map[string]interface{}, _ *apiv1beta1.OpenStackLightspeed) {
					gomega.Expect(config["api_key"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_API_KEY}"))
				}),
			ginkgo.Entry("for gemini", GeminiProviderName, "remote::gemini",
				func(config map[string]interface{}, _ *apiv1beta1.OpenStackLightspeed) {
					gomega.Expect(config["api_key"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_API_KEY}"))
					gomega.Expect(config).NotTo(gomega.HaveKey("base_url"))
				}),
			ginkgo.Entry("for rhoai_vllm", RHOAIVLLMProviderName, "remote::vllm",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					gomega.Expect(config["api_token"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_API_KEY}"))
					gomega.Expect(config["base_url"]).To(gomega.Equal(instance.Spec.Models[0].LLMEndpoint))
				}),
			ginkgo.Entry("for rhelai_vllm", RHELAIVLLMProviderName, "remote::vllm",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					gomega.Expect(config["api_token"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_API_KEY}"))
					gomega.Expect(config["base_url"]).To(gomega.Equal(instance.Spec.Models[0].LLMEndpoint))
				}),
			ginkgo.Entry("for azure_openai", AzureOpenAIProviderName, "remote::azure",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					gomega.Expect(config["api_key"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_API_KEY}"))
					gomega.Expect(config["client_id"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_CLIENT_ID:=}"))
					gomega.Expect(config["tenant_id"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_TENANT_ID:=}"))
					gomega.Expect(config["client_secret"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_CLIENT_SECRET:=}"))
					gomega.Expect(config["base_url"]).To(gomega.Equal(instance.Spec.Models[0].LLMEndpoint))
					gomega.Expect(config["deployment_name"]).To(gomega.Equal(instance.Spec.Models[0].LLMDeploymentName))
					gomega.Expect(config["api_version"]).To(gomega.Equal(instance.Spec.Models[0].LLMAPIVersion))
				}),
			ginkgo.Entry("for watsonx", WatsonXProviderName, "remote::watsonx",
				func(config map[string]interface{}, instance *apiv1beta1.OpenStackLightspeed) {
					gomega.Expect(config["base_url"]).To(gomega.Equal(instance.Spec.Models[0].LLMEndpoint))
					gomega.Expect(config["project_id"]).To(gomega.Equal(instance.Spec.Models[0].LLMProjectID))
					gomega.Expect(config["api_key"]).To(gomega.Equal("${env.PROVIDER_DEFAULT_MODEL_API_KEY}"))
				}),
		)
	})

	ginkgo.Describe("buildOGXModels", func() {
		ginkgo.DescribeTable("should return correct models config",
			func(provider string) {
				instance := getOpenStackLightspeedProvidersInstance(provider)
				modelsConfig := buildOGXModels(nil, instance)

				gomega.Expect(modelsConfig).To(gomega.HaveLen(2))

				modelConfig := modelsConfig[0].(map[string]interface{})
				checkModelCommonConfig(modelConfig, instance.Spec.Models[0])

				okpModel := modelsConfig[1].(map[string]interface{})
				gomega.Expect(okpModel["model_id"]).To(gomega.Equal("solr_embedding"))
				gomega.Expect(okpModel["model_type"]).To(gomega.Equal("embedding"))
				gomega.Expect(okpModel["provider_id"]).To(gomega.Equal("sentence-transformers"))
			},
			ginkgo.Entry("for openai", OpenAIProviderName),
			ginkgo.Entry("for gemini", GeminiProviderName),
			ginkgo.Entry("for rhoai_vllm", RHOAIVLLMProviderName),
			ginkgo.Entry("for rhelai_vllm", RHELAIVLLMProviderName),
			ginkgo.Entry("for azure_openai", AzureOpenAIProviderName),
			ginkgo.Entry("for watsonx", WatsonXProviderName),
		)
	})

	ginkgo.Describe("buildOGXToolRuntime", func() {
		ginkgo.It("always includes file-search", func() {
			instance := getOpenStackLightspeedProvidersInstance(OpenAIProviderName)
			providers := buildOGXToolRuntime(nil, instance)
			gomega.Expect(providers).To(gomega.HaveLen(2))
			gomega.Expect(providers[0].(map[string]interface{})["provider_id"]).To(gomega.Equal("model-context-protocol"))
			gomega.Expect(providers[1].(map[string]interface{})["provider_id"]).To(gomega.Equal("file-search"))
		})
	})

	ginkgo.Describe("buildOGXToolGroups", func() {
		ginkgo.It("always includes file_search tool group", func() {
			instance := getOpenStackLightspeedProvidersInstance(OpenAIProviderName)
			groups := buildOGXToolGroups(nil, instance)
			gomega.Expect(groups).To(gomega.HaveLen(1))
			gomega.Expect(groups[0].(map[string]interface{})["toolgroup_id"]).To(gomega.Equal("builtin::file_search"))
			gomega.Expect(groups[0].(map[string]interface{})["provider_id"]).To(gomega.Equal("file-search"))
		})
	})

	ginkgo.Describe("OKP always enabled", func() {
		ginkgo.It("includes okp_solr in vector_io", func() {
			instance := getOpenStackLightspeedProvidersInstance(OpenAIProviderName)
			providers := buildOGXVectorIO(nil, instance, "product:openstack")
			ids := make([]string, 0, len(providers))
			for _, p := range providers {
				ids = append(ids, p.(map[string]interface{})["provider_id"].(string))
			}
			gomega.Expect(ids).To(gomega.ContainElement("okp_solr"))
		})

		ginkgo.It("includes portal-rag vector store and solr_embedding model", func() {
			instance := getOpenStackLightspeedProvidersInstance(OpenAIProviderName)
			stores := buildOGXVectorStores(nil, instance)
			gomega.Expect(stores).To(gomega.HaveLen(1))
			gomega.Expect(stores[0].(map[string]interface{})["vector_store_id"]).To(gomega.Equal("portal-rag"))

			models := buildOGXModels(nil, instance)
			modelIDs := make([]string, 0, len(models))
			for _, m := range models {
				modelIDs = append(modelIDs, m.(map[string]interface{})["model_id"].(string))
			}
			gomega.Expect(modelIDs).To(gomega.ContainElement("solr_embedding"))
		})
	})
})
