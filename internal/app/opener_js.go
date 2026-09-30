//go:build js

package app

import tea "charm.land/bubbletea/v2"

// The browser has no folders to browse, and the picker's text input depends
// on a clipboard package that does not build for WebAssembly.
type opener struct {
	dir   string
	going bool
	find  *finder
}

// The browser has no folders to search either.
type finder struct{}

type findResult struct{}

func (f *finder) answered(findResult) {}
func (f *finder) status() string      { return "" }

type openResult struct{}

var orderNames = []string{"name", "date", "kind"}

func (o *opener) view() string    { return "" }
func (o *opener) resize(int, int) {}
func (o *opener) stopGoing()      {}

func (m *Model) canBrowse() bool            { return false }
func (m *Model) openBrowser() tea.Cmd       { return nil }
func (m *Model) browse(tea.Msg) tea.Cmd     { return nil }
func (m *Model) browseFrom(string) tea.Cmd  { return nil }
func (m *Model) toggleUnsupported() tea.Cmd { return nil }
func (m *Model) reorder() tea.Cmd           { return nil }
func (m *Model) toggleText() tea.Cmd        { return nil }
func (m *Model) opened(openResult) tea.Cmd  { return nil }
