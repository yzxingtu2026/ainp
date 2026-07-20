package ai

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
)

func TestDeploymentStructuredOutputCapabilitiesMustBeExplicit(t *testing.T) {
	base := DeploymentCreateRequest{Code: "chat", ModelType: aimodel.ModelTypeChat}

	featureOnly := base
	featureOnly.Capabilities.Features = []aimodel.Feature{aimodel.FeatureStructuredOutput}
	if err := validateDeployment(featureOnly); err == nil {
		t.Fatal("structured_output without strategies must fail")
	}

	strategyOnly := base
	strategyOnly.Capabilities.StructuredOutputStrategies = []aimodel.StructuredOutputStrategy{aimodel.StructuredOutputResponseOnlyFunction}
	if err := validateDeployment(strategyOnly); err == nil {
		t.Fatal("structured output strategies without feature must fail")
	}

	valid := featureOnly
	valid.Capabilities.StructuredOutputStrategies = []aimodel.StructuredOutputStrategy{aimodel.StructuredOutputResponseOnlyFunction, aimodel.StructuredOutputJSONMode}
	if err := validateDeployment(valid); err != nil {
		t.Fatalf("explicit structured output capabilities must pass: %v", err)
	}

	duplicate := valid
	duplicate.Capabilities.StructuredOutputStrategies = []aimodel.StructuredOutputStrategy{aimodel.StructuredOutputJSONMode, aimodel.StructuredOutputJSONMode}
	if err := validateDeployment(duplicate); err == nil {
		t.Fatal("duplicate structured output strategies must fail")
	}
}
