package differ

import (
	"bytes"
	"fmt"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/grafana/k8s-diff/pkg/ui"
)

type DebugInfo struct {
	RuleDebugInfos []*RuleDebugInfo
	InitialObjects []*YamlObject
}

func (d *DebugInfo) Print(output ui.UI) {
	var todoRules []*RuleDebugInfo
	for _, ruleDebugInfo := range d.RuleDebugInfos {
		if ruleDebugInfo.Rule.Describe().Todo {
			todoRules = append(todoRules, ruleDebugInfo)
		}
	}

	output.SummarizeResults(`
## Future TODO Items
These differences are known and planned to be fixed in the future.
`, func(output ui.UI) error {
		printRuleDebugInfo(output, todoRules)
		return nil
	})
}

func printRuleDebugInfo(output ui.UI, ruleDebugInfos []*RuleDebugInfo) {
	var patchesByName = make(map[string][]objectPatch)
	for _, ruleDebugInfo := range ruleDebugInfos {
		for _, patches := range ruleDebugInfo.Patches {
			patchesByName[ruleDebugInfo.Rule.Describe().Name] = append(patchesByName[ruleDebugInfo.Rule.Describe().Name], patches.patchedObjects...)
		}
	}

	for ruleName, patches := range patchesByName {
		var changesBySource = make(map[ResourceKey][]objectPatch)

		for _, change := range patches {
			changesBySource[change.newObj.ResourceKey] = append(changesBySource[change.newObj.ResourceKey], change)
		}

		output.SummarizeResults(ruleName, func(output ui.UI) error {
			for _, v := range changesBySource {
				printChanges(output, v)
			}
			return nil
		})
	}
}

func printChanges(output ui.UI, changes []objectPatch) {
	finalPatch := NewYamlObject("patch")

	for _, oc := range changes {
		// intentionally reversed to isolate only the fields that changed
		patchJson := createPatch(oc.newObj, oc.oldObj)
		err := DecodeYamlObject(bytes.NewReader(patchJson), finalPatch)
		if err != nil {
			panic(err)
		}
	}

	if len(finalPatch.Object) == 0 {
		return
	}

	output.Print("```yaml")
	output.Print("# " + changes[0].newObj.ResourceKey.Source)
	EncodeYamlObject(output, finalPatch)
	output.Print("```")
}

func NewDebugInfo(ruleSet RuleSet) *DebugInfo {
	debugInfo := &DebugInfo{
		RuleDebugInfos: make([]*RuleDebugInfo, len(ruleSet.IgnoreRules)+len(ruleSet.PatchRules)),
	}
	return debugInfo
}

func (d *DebugInfo) AddInitialObjects(objects []*YamlObject) {
	d.InitialObjects = append(d.InitialObjects, objects...)
}

func (d *DebugInfo) ValidateAllRulesWereEffective(output ui.UI) error {
	if d == nil {
		return nil
	}
	var multiError = new(MultiError)
	output.SummarizeResults("Rule Validation", func(output ui.UI) error {
		for _, ruleDebugInfo := range d.RuleDebugInfos {
			if err := ruleDebugInfo.ValidateAllStepsWereEffective(output); err != nil {
				multiError.Errors = append(multiError.Errors, err)
			}
		}
		return nil
	})
	if multiError.Errors != nil {
		return multiError
	}
	return nil
}

func (d *DebugInfo) NewRuleDebugInfo(i int, rule ObjectRule) *RuleDebugInfo {
	if d == nil {
		return nil
	}

	rdi := d.RuleDebugInfos[i]
	if rdi == nil {
		rdi = &RuleDebugInfo{
			Parent:  d,
			Rule:    rule,
			Matches: make([]IncrementalMatchDebugInfo, len(rule.Describe().MatchRules)),
			Patches: make([]IncrementalPatchDebugInfo, len(rule.Describe().PatchRules)),
		}

		d.RuleDebugInfos[i] = rdi
	}

	if rdi.Rule.Describe().Name != rule.Describe().Name {
		// This should never happen, but this interface is not very well designed, so it'd be pretty easy to call this method with the wrong rule.
		panic("rule mismatch, this is a bug")
	}

	return rdi
}

// RuleDebugInfo is created during the rule application process and can be used to
// understand the state of the system after each rule application or to debug
// the rule application process.
type RuleDebugInfo struct {
	Parent  *DebugInfo
	Rule    ObjectRule
	Matches []IncrementalMatchDebugInfo
	Patches []IncrementalPatchDebugInfo
	Ignored []*YamlObject
}

type MultiError struct {
	Errors []error
}

func (e *MultiError) Error() string {
	var sb strings.Builder
	for _, err := range e.Errors {
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}

type IneffectiveMatchError struct {
	RuleName  string
	Step      int
	Matched   []*YamlObject
	MatchRule Json6902MatchOperation
}

func (e IneffectiveMatchError) Error() string {
	candidateStrings := []string{}
	for _, u := range e.Matched {
		candidateStrings = append(candidateStrings, ResourceKeyForObject(u).String())
	}

	return fmt.Sprintf("rule %q matching step %d:\n\t %s did not match any objects in:\n\t\t%s", e.RuleName, e.Step, e.MatchRule, strings.Join(candidateStrings, "\n\t\t"))
}

type IneffectivePatchError struct {
	RuleName  string
	Step      int
	Matched   []*YamlObject
	PatchRule Json6902PatchOperation
}

func (e IneffectivePatchError) Error() string {
	candidateStrings := []string{}
	for _, u := range e.Matched {
		candidateStrings = append(candidateStrings, ResourceKeyForObject(u).String())
	}

	return fmt.Sprintf("rule %q patching step %d:\n\t %s did not change any objects in:\n\t\t%s", e.RuleName, e.Step, e.PatchRule, strings.Join(candidateStrings, "\n\t\t"))
}

func (d *RuleDebugInfo) ValidateAllStepsWereEffective(output ui.UI) error {
	return output.SummarizeResults(d.Rule.Describe().Name, func(output ui.UI) error {

		previousMatchedObjects := d.Parent.InitialObjects
		// Validate that all matches matched at least one object.
		for step, debugInfo := range d.Matches {
			if len(debugInfo.matchedObjects) == 0 {

				output.SummarizeResults(fmt.Sprintf("Match rule #%d doesn't match any objects", step), func(output ui.UI) error {
					output.Print("Rule: " + d.Rule.Describe().MatchRules[step].String())
					output.Print("Objects:")
					output.ListItems(len(previousMatchedObjects), func(i int, output ui.UI) error {
						output.Print(previousMatchedObjects[i].ResourceKey.String())
						return nil
					})
					return nil
				})

				return IneffectiveMatchError{
					RuleName:  d.Rule.Describe().Name,
					Step:      step,
					Matched:   previousMatchedObjects,
					MatchRule: d.Rule.Describe().MatchRules[step],
				}
			}

			previousMatchedObjects = debugInfo.matchedObjects
		}

		// Validate that all patches changed at least one object.
		for step, debugInfo := range d.Patches {
			if len(debugInfo.patchedObjects) == 0 {

				output.SummarizeResults(fmt.Sprintf("Patch rule #%d doesn't change any objects", step), func(output ui.UI) error {
					output.Print("Rule: " + d.Rule.Describe().PatchRules[step].String())
					output.Print("Objects:")
					output.ListItems(len(previousMatchedObjects), func(i int, output ui.UI) error {
						output.Print(previousMatchedObjects[i].ResourceKey.String())
						return nil
					})
					return nil
				})

				return IneffectivePatchError{
					RuleName:  d.Rule.Describe().Name,
					Step:      step,
					Matched:   previousMatchedObjects,
					PatchRule: d.Rule.Describe().PatchRules[step],
				}
			}

			// Validate that patches aren't all empty.
			actualPatchedObjects := []objectPatch{}
			for _, op := range debugInfo.patchedObjects {
				if !bytes.Equal(op.patch, []byte("{}")) {
					actualPatchedObjects = append(actualPatchedObjects, op)
				}
			}

			if len(actualPatchedObjects) == 0 {
				output.SummarizeResults(fmt.Sprintf("Patch rule #%d doesn't change any objects", step), func(output ui.UI) error {
					output.Print("Rule: " + d.Rule.Describe().PatchRules[step].String())
					output.Print("Objects:")
					output.ListItems(len(previousMatchedObjects), func(i int, output ui.UI) error {
						output.Print(previousMatchedObjects[i].ResourceKey.String())
						return nil
					})
					return nil
				})

				return IneffectivePatchError{
					RuleName:  d.Rule.Describe().Name,
					Step:      step,
					Matched:   previousMatchedObjects,
					PatchRule: d.Rule.Describe().PatchRules[step],
				}
			}

		}
		return nil
	})
}

func (d *RuleDebugInfo) RecordIncrementalMatch(step int, obj *YamlObject) {
	if d == nil {
		return
	}
	d.Matches[step].matchedObjects = append(d.Matches[step].matchedObjects, obj)
}

func (d *RuleDebugInfo) RecordIncrementalPatch(step int, oldObj, newObj *YamlObject) {
	if d == nil {
		return
	}
	d.Patches[step].patchedObjects = append(d.Patches[step].patchedObjects, objectPatch{
		oldObj: oldObj.DeepCopy(),
		newObj: newObj.DeepCopy(),
		patch:  createPatch(oldObj, newObj),
	})
}

func (d *RuleDebugInfo) RecordIgnore(obj *YamlObject) {
	d.Ignored = append(d.Ignored, obj)
}

type IncrementalMatchDebugInfo struct {
	matchedObjects []*YamlObject
}

type IncrementalPatchDebugInfo struct {
	patchedObjects []objectPatch
}

type objectPatch struct {
	oldObj, newObj *YamlObject
	patch          []byte
}

func createPatch(oldObj, newObj *YamlObject) []byte {
	oldObjBuf := new(bytes.Buffer)
	err := EncodeYamlObjectAsJson(oldObjBuf, oldObj)
	if err != nil {
		panic(err)
	}

	newObjBuf := new(bytes.Buffer)
	err = EncodeYamlObjectAsJson(newObjBuf, newObj)
	if err != nil {
		panic(err)
	}

	patchResult, err := jsonpatch.CreateMergePatch(oldObjBuf.Bytes(), newObjBuf.Bytes())
	if err != nil {
		panic(err)
	}

	return patchResult
}
