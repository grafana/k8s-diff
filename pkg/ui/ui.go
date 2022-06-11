package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
)

type UI interface {
	ReportError(error)
	SummarizeResults(string, func(UI) error)
	ListItems(length int, itemsFunc func(int, UI) error)
	Print(msg string)
	io.Writer
}

type TermUI struct {
	output io.Writer
}

func NewUI(output io.Writer) UI {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return &GithubActionsUI{output}
	}

	return &TermUI{output: output}
}

func (u *TermUI) ReportError(err error) {
	fmt.Fprintln(u.output, err.Error())
}

func (u *TermUI) SummarizeResults(header string, bodyFunc func(UI) error) {
	bodyBuffer := &bytes.Buffer{}
	subUI := NewUI(bodyBuffer)

	err := bodyFunc(subUI)
	if err != nil {
		u.ReportError(err)
	}

	if bodyBuffer.Len() == 0 {
		return
	}

	fmt.Fprintln(u.output, header)
	fmt.Fprintln(u.output, bodyBuffer.String())
}

func (u *TermUI) ListItems(length int, itemsFunc func(int, UI) error) {
	for i := 0; i < length; i++ {
		err := itemsFunc(i, u)
		if err != nil {
			u.ReportError(err)
		}
	}
}

func (u *TermUI) Print(msg string) {
	fmt.Fprintln(u.output, msg)
}

func (u *TermUI) Write(p []byte) (n int, err error) {
	return u.output.Write(p)
}

// GithubActionsUI takes advantage of the fact that GithubActions can display markdown
type GithubActionsUI struct {
	output io.Writer
}

func (u *GithubActionsUI) ReportError(err error) {
	fmt.Fprintf(u.output, "**Error occurred during processing: %s**\n", err.Error())
}

var summaryTemplate = template.Must(template.New("root").Parse(`
<details>
<summary>
{{.Header}}
</summary>

<blockquote>

{{.Body}}

</blockquote>
</details>
`))

func (u *GithubActionsUI) SummarizeResults(header string, bodyFunc func(UI) error) {
	bodyBuffer := &bytes.Buffer{}
	subUI := NewUI(bodyBuffer)

	err := bodyFunc(subUI)
	if err != nil {
		u.ReportError(errors.Wrap(err, "failed to summarize results"))
	}

	if bodyBuffer.Len() == 0 {
		return
	}

	err = summaryTemplate.Execute(u.output, struct {
		Header string
		Body   template.HTML
	}{
		Header: header,
		Body:   template.HTML(bodyBuffer.String()),
	})
	if err != nil {
		u.ReportError(errors.Wrap(err, "failed to render results"))
	}
}

func (u *GithubActionsUI) ListItems(length int, itemsFunc func(int, UI) error) {
	bodyBuffer := &bytes.Buffer{}
	for i := 0; i < length; i++ {
		bodyBuffer.Reset()
		subUI := NewUI(bodyBuffer)
		err := itemsFunc(i, subUI)
		if err != nil {
			u.ReportError(err)
		}

		fmt.Fprintf(u.output, "* %s\n", strings.ReplaceAll(bodyBuffer.String(), "\n", "\n  "))
	}
}

func (u *GithubActionsUI) Print(msg string) {
	fmt.Fprintln(u.output, msg)
}

func (u *GithubActionsUI) Write(p []byte) (n int, err error) {
	return u.output.Write(p)
}
