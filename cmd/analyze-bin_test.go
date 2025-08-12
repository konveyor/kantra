package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGradleSourcesTaskFileConfiguration(t *testing.T) {
	a := analyzeCommand{}
	a.kantraDir = "kantraDir"
	configs, err := a.createProviderConfigsContainerless([]interface{}{})
	if err != nil {
		t.Fail()
	}

	assert.NotEmpty(t, configs)
	assert.Equal(t, configs[0].InitConfig[0].ProviderSpecificConfig["gradleSourcesTaskFile"], "kantraDir/task.gradle")
}
