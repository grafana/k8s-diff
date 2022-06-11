package differ

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"github.com/grafana/k8s-diff/pkg/ui"
	"gopkg.in/yaml.v2"
)

func ReadStateFromPath(output ui.UI, path string) ([]*YamlObject, error) {
	state := []*YamlObject{}
	skippedFiles := []string{}
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".yaml" {
			skippedFiles = append(skippedFiles, path)
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to read k8s resource from yaml: %w", err)
		}
		defer f.Close()

		decoder := yaml.NewDecoder(f)
		for {
			var obj = NewYamlObject(path)
			err = decoder.Decode(&obj.Object)
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			state = append(state, obj)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(skippedFiles) > 0 {
		output.SummarizeResults("Skipped files", func(u ui.UI) error {
			u.ListItems(len(skippedFiles), func(i int, u ui.UI) error {
				u.Print(skippedFiles[i])
				return nil
			})
			return nil
		})
	}
	return state, nil
}

func WriteStateToDirectory(objects []*YamlObject, path, outputTemplate string) error {
	var generateFileName = func(obj *YamlObject) string {
		return filepath.Base(obj.ResourceKey.Source)
	}

	if outputTemplate != "" {
		var tmpl *template.Template
		var err error
		if tmpl, err = template.New("output").Parse(outputTemplate); err != nil {
			return fmt.Errorf("failed to parse output template: %w", err)
		}
		generateFileName = func(obj *YamlObject) string {
			var buf = &bytes.Buffer{}
			err := tmpl.Execute(buf, obj.Object)
			if err != nil {
				panic(err)
			}
			return buf.String()
		}
	}

	err := os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	err = os.MkdirAll(path, 0755)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		f, err := os.Create(filepath.Join(path, generateFileName(obj)))
		if err != nil {
			return err
		}
		defer f.Close()
		err = EncodeYamlObject(f, obj)
		if err != nil {
			return err
		}
	}
	return nil
}
