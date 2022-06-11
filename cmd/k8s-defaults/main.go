package main

import (
	"flag"
	"os"

	"github.com/grafana/k8s-diff/pkg/differ"
	k8s_defaulter "github.com/grafana/k8s-diff/pkg/k8s-defaulter"
	"github.com/grafana/k8s-diff/pkg/ui"
	"github.com/pkg/errors"
)

type Config struct {
	InputDir  string
	OutputDir string
}

func (c *Config) RegisterFlags(f *flag.FlagSet) {
	f.StringVar(&c.InputDir, "input-dir", "", "Input directory")
	f.StringVar(&c.OutputDir, "output-dir", "", "Output directory")
}

func main() {
	ui := ui.NewUI(os.Stdout)
	var config = &Config{}
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

	client, err := k8s_defaulter.NewDryRunK8sClient()
	if err != nil {
		ui.ReportError(errors.Wrap(err, "failed to create k8s client"))
		os.Exit(1)
	}

	rule := k8s_defaulter.NewDefaultSettingRule(client)
	objects, err = differ.MapObjects(objects, rule, nil)
	if err != nil {
		ui.ReportError((errors.Wrap(err, "failed to apply defaults")))
		os.Exit(1)
	}

	err = differ.WriteStateToDirectory(objects, config.OutputDir, "")
	if err != nil {
		ui.ReportError(errors.New("failed to write state: " + err.Error()))
		os.Exit(1)
	}
}
