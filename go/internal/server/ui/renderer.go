package ui

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"sync"
)

// TemplateRenderer handles the rendering of HTML templates from the embedded filesystem.
type TemplateRenderer struct {
	templates map[string]*template.Template
	mu        sync.RWMutex
}

// NewTemplateRenderer initializes a new renderer from the embedded FS.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	r := &TemplateRenderer{
		templates: make(map[string]*template.Template),
	}

	if err := r.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return r, nil
}

// Render renders a template by name with the provided data.
func (r *TemplateRenderer) Render(w io.Writer, name string, data any) error {
	r.mu.RLock()
	tmpl, ok := r.templates[name]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("template %s not found", name)
	}

	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("failed to execute template %s: %w", name, err)
	}
	return nil
}

// RenderFragment renders a specific template from a set by name.
func (r *TemplateRenderer) RenderFragment(w io.Writer, fileName string, templateName string, data any) error {
	r.mu.RLock()
	tmpl, ok := r.templates[fileName]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("template set %s not found", fileName)
	}

	if err := tmpl.ExecuteTemplate(w, templateName, data); err != nil {
		return fmt.Errorf("failed to execute fragment %s in %s: %w", templateName, fileName, err)
	}
	return nil
}

func (r *TemplateRenderer) loadTemplates() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// List all templates in the templates directory
	entries, err := FS.ReadDir("templates")
	if err != nil {
		return fmt.Errorf("failed to read templates directory: %w", err)
	}

	var layoutFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() == "base.html" {
			layoutFiles = append(layoutFiles, filepath.Join("templates", entry.Name()))
		}
	}

	// For each template file that isn't base.html, parse it together with base.html
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "base.html" {
			continue
		}

		name := entry.Name()
		// Page file must come first so ParseFS names the template set after it.
		// When tmpl.Execute() is called, it will run the page template, which
		// in turn invokes the "base" layout template.
		files := append([]string{filepath.Join("templates", name)}, layoutFiles...)

		tmpl := template.New(name).Funcs(template.FuncMap{
			"navClass": func(current, target string, isSidebar bool) string {
				if isSidebar {
					if current == target {
						return "group flex items-center px-3 py-2 text-sm font-medium rounded-md bg-gray-200 text-gray-900 border-l-4 border-blue-600"
					}
					return "group flex items-center px-3 py-2 text-sm font-medium rounded-md text-gray-600 hover:bg-gray-50 hover:text-gray-900"
				}
				// Top Nav
				if current == target {
					return "px-3 py-2 rounded-md text-sm font-medium bg-gray-700 text-white transition"
				}
				return "px-3 py-2 rounded-md text-sm font-medium text-gray-300 hover:text-white hover:bg-gray-700 transition"
			},
		})

		tmpl, err = tmpl.ParseFS(FS, files...)
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", name, err)
		}

		r.templates[name] = tmpl
	}

	return nil
}

// Middleware is a helper to serve standard templates easily.
func (r *TemplateRenderer) Handler(name string, data any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := r.Render(w, name, data); err != nil {
			http.Error(w, fmt.Sprintf("failed to render template: %v", err), http.StatusInternalServerError)
		}
	}
}
