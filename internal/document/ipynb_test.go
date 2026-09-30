package document

import (
	"encoding/base64"
	"strings"
	"testing"
)

func notebook(t *testing.T) []byte {
	t.Helper()
	png := base64.StdEncoding.EncodeToString(pngBytes(t))
	return []byte(`{
  "cells": [
    {"cell_type": "markdown", "metadata": {}, "source": ["# Title\n", "\n", "Some *text*."]},
    {"cell_type": "code", "execution_count": 3, "metadata": {}, "source": "print('hi')\nx = 1",
     "outputs": [
       {"output_type": "stream", "name": "stdout", "text": ["hi\n"]},
       {"output_type": "execute_result", "execution_count": 3, "data": {"text/plain": ["1"]}, "metadata": {}},
       {"output_type": "display_data", "data": {"image/png": "` + png + `", "text/plain": ["<Figure>"]}, "metadata": {}},
       {"output_type": "error", "ename": "ValueError", "evalue": "bad", "traceback": ["\u001b[31mTraceback\u001b[0m", "ValueError: bad"]}
     ]},
    {"cell_type": "raw", "metadata": {}, "source": "raw text"},
    {"cell_type": "code", "execution_count": null, "metadata": {}, "source": "", "outputs": []}
  ],
  "metadata": {"kernelspec": {"display_name": "Python 3", "language": "python", "name": "python3"}, "language_info": {"name": "python", "version": "3.12.1"}},
  "nbformat": 4, "nbformat_minor": 5
}`)
}

func TestNotebookCellsBecomeMarkdown(t *testing.T) {
	nb, err := ReadNotebook(notebook(t))
	if err != nil {
		t.Fatal(err)
	}
	md := string(nb.Markdown)
	for _, want := range []string{
		"# Title\n\nSome *text*.\n",
		"```python\nprint('hi')\nx = 1\n```\n",
		"```text\nhi\n```\n",
		"```text\n1\n```\n",
		"![output](dropped/cell-2-output-3.png)",
		"```text\nTraceback\nValueError: bad\n```\n",
		"```\nraw text\n```\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "\x1b") || strings.Contains(md, "<Figure>") {
		t.Errorf("escapes or the picture's text stand-in were kept:\n%s", md)
	}
	if strings.Contains(md, "In [3]") == false {
		t.Errorf("the execution count is not shown:\n%s", md)
	}
	if data, err := readFileFrom(nb.Files, "dropped/cell-2-output-3.png"); err != nil || len(data) == 0 {
		t.Fatalf("picture: %v", err)
	}
	fields := nb.fields()
	for label, want := range map[string]string{"Format": "Jupyter notebook 4.5", "Language": "python 3.12.1", "Kernel": "Python 3", "Cells": "4 (2 code, 1 markdown, 1 raw)", "Outputs": "4", "Pictures": "1"} {
		if got := field(fields, label); got != want {
			t.Errorf("%s = %q, want %q", label, got, want)
		}
	}
}

func TestNotebookThatIsNotOne(t *testing.T) {
	if _, err := ReadNotebook([]byte(`{"cells": "no"}`)); err == nil {
		t.Fatal("accepted")
	}
	if _, err := ReadNotebook([]byte(`{"nbformat": 3, "worksheets": []}`)); err == nil || !strings.Contains(err.Error(), "nbformat 4") {
		t.Fatalf("v3: %v", err)
	}
}
