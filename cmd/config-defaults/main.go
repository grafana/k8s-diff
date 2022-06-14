package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"

	"github.com/grafana/k8s-diff/pkg/differ"
	"github.com/grafana/k8s-diff/pkg/process"
	"github.com/grafana/k8s-diff/pkg/ui"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

type Config struct {
	InputDir  string
	OutputDir string
	ImageTag  string
}

const MimirImage = "grafana/mimir"

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&c.InputDir, "input-dir", "", "Input directory")
	f.StringVar(&c.OutputDir, "output-dir", "", "Output directory")
	f.StringVar(&c.ImageTag, "image-tag", "latest", "Image tag, e.g. latest or r190-abcde")
}

func main() {
	ui := ui.NewUI(os.Stdout)
	config := &Config{}
	config.RegisterFlags(flag.CommandLine)
	flag.Parse()

	if config.InputDir == "" || config.OutputDir == "" {
		ui.ReportError(errors.New("input-dir and output-dir are required"))
		flag.Usage()
		os.Exit(1)
	}

	objects, err := differ.ReadStateFromPath(ui, config.InputDir)
	if err != nil {
		ui.ReportError(errors.Wrap(err, "failed to read state "))
		os.Exit(1)
	}

	defaults, err := LoadDefaults(config.ImageTag)
	if err != nil {
		ui.ReportError(errors.Wrap(err, "failed to load defaults"))
		os.Exit(1)
	}

	for i, yo := range objects {
		annotateDefaults(defaults.Object, yo.Object)

		objects[i].RemoveNulls()
	}

	err = differ.WriteStateToDirectory(objects, config.OutputDir, "")
	if err != nil {
		ui.ReportError(errors.New("failed to write state: " + err.Error()))
		os.Exit(1)
	}
}

func annotateDefaults(defaults, config interface{}) interface{} {
	if defaults == nil {
		fmt.Println("defaults is nil")
		yaml.NewEncoder(os.Stdout).Encode(defaults)
		fmt.Println("config is:")
		yaml.NewEncoder(os.Stdout).Encode(config)
	}
	switch config := config.(type) {
	case map[string]interface{}:
		for k, v := range config {
			config[k] = annotateDefaults(defaults.(map[string]interface{})[k], v)
		}
	case map[interface{}]interface{}:
		for k, v := range config {
			config[k] = annotateDefaults(defaults.(map[interface{}]interface{})[k], v)
		}
	case []interface{}:
		for i, v := range config {
			defaultV := defaults.([]interface{})
			if len(defaultV) > i {
				config[i] = annotateDefaults(defaultV[i], v)
			}
		}
	default:
		if reflect.DeepEqual(config, defaults) {
			return fmt.Sprintf("%#v (default)", config)
		}
	}
	return config
}

func LoadDefaults(imageTag string) (*differ.YamlObject, error) {
	configObj, err := process.RunMimirAndCaptureConfigOutput(process.ProcessConfiguration{
		Image:          MimirImage + ":" + imageTag,
		Args:           []string{},
		ConfigFileText: ``,
	}, "default")
	if err != nil {
		return nil, err
	}

	return configObj, nil
}
