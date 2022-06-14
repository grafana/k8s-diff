package differ

import (
	"io"
	"testing"

	"github.com/grafana/k8s-diff/pkg/ui"
	"github.com/stretchr/testify/assert"
)

func TestValidateMatches(t *testing.T) {
	ui := ui.NewUI(io.Discard)

	t.Run("matches that match at least one object", func(t *testing.T) {
		object := newDeploymentWithLabels("querier", map[string]string{
			"app.kubernetes.io/name": "mimir",
		})

		ignoreRule := IgnoreRule{
			Match: Json6902Match{
				{
					Json6902PatchOperation: Json6902PatchOperation{
						Op:    "test",
						Path:  "/metadata/labels/app.kubernetes.io~1name",
						Value: "mimir",
					},
				},
			},
		}

		debugInfo := NewDebugInfo(RuleSet{IgnoreRules: []IgnoreRule{ignoreRule}}).NewRuleDebugInfo(0, ignoreRule)
		debugInfo.Parent.AddInitialObjects([]*YamlObject{object})
		result, err := ignoreRule.MapObject(object, debugInfo)
		assert.NoError(t, err, "should not fail")
		assert.Nil(t, result, "should not return a result since it's ignored")

		err = debugInfo.ValidateAllStepsWereEffective(ui)
		assert.NoError(t, err, "all steps were effective")
	})

	t.Run("matches that don't match at least one object", func(t *testing.T) {
		object := newDeploymentWithLabels("querier", map[string]string{
			"app.kubernetes.io/name": "mimir",
		})

		ignoreRule := IgnoreRule{
			Match: Json6902Match{
				{
					Json6902PatchOperation: Json6902PatchOperation{
						Op:    "test",
						Path:  "/metadata/labels/app.kubernetes.io~1name",
						Value: "loki",
					},
				},
			},
		}

		debugInfo := NewDebugInfo(RuleSet{IgnoreRules: []IgnoreRule{ignoreRule}}).NewRuleDebugInfo(0, ignoreRule)
		debugInfo.Parent.AddInitialObjects([]*YamlObject{object})
		result, err := ignoreRule.MapObject(object, debugInfo)
		assert.NoError(t, err, "should not fail")
		assert.NotNil(t, result, "should return a result since it's not ignored")

		err = debugInfo.ValidateAllStepsWereEffective(ui)
		assert.Error(t, err, "the match step should be flagged as ineffective")
	})

}

func TestValidatePatches(t *testing.T) {
	ui := ui.NewUI(io.Discard)

	t.Run("Patches that mutate at least one object", func(t *testing.T) {
		object := newDeploymentWithLabels("querier", map[string]string{
			"app.kubernetes.io/name": "mimir",
		})

		patchRule := Json6902PatchRule{
			Steps: Json6902Patch{
				Json6902PatchOperation{
					Op:    "replace",
					Path:  "/metadata/labels/app.kubernetes.io~1name",
					Value: "loki",
				},
			},
		}

		debugInfo := NewDebugInfo(RuleSet{PatchRules: []Json6902PatchRule{patchRule}}).NewRuleDebugInfo(0, patchRule)
		result, err := patchRule.MapObject(object, debugInfo)
		assert.NoError(t, err, "should not fail")
		assert.NotNil(t, result, "should return a result since it's not ignored")

		err = debugInfo.ValidateAllStepsWereEffective(ui)
		assert.NoError(t, err, "all steps were effective")
	})

	t.Run("Patches that don't mutate at least one object", func(t *testing.T) {
		object := newDeploymentWithLabels("querier", map[string]string{
			"app.kubernetes.io/name": "mimir",
		})

		patchRule := Json6902PatchRule{
			Steps: Json6902Patch{
				Json6902PatchOperation{
					Op:    "replace",
					Path:  "/metadata/labels/app.kubernetes.io~1name",
					Value: "mimir",
				},
			},
		}

		debugInfo := NewDebugInfo(RuleSet{PatchRules: []Json6902PatchRule{patchRule}}).NewRuleDebugInfo(0, patchRule)
		result, err := patchRule.MapObject(object, debugInfo)
		assert.NoError(t, err, "should not fail")
		assert.NotNil(t, result, "should return a result since it's not ignored")

		err = debugInfo.ValidateAllStepsWereEffective(ui)
		assert.Error(t, err, "the match step should be flagged as ineffective")
	})
}
