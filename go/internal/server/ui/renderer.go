package ui

import (
	"database/sql"
	"encoding/json"
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
	var partialFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == "base.html" {
			layoutFiles = append(layoutFiles, filepath.Join("templates", entry.Name()))
		} else if entry.Name() == "fragments.html" || entry.Name() == "active_workers.html" {
			partialFiles = append(partialFiles, filepath.Join("templates", entry.Name()))
		}
	}

	// For each template file that isn't a shared one, parse it together with shared ones
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "base.html" || entry.Name() == "fragments.html" || entry.Name() == "active_workers.html" {
			// We still want to parse fragments and active_workers as their own sets so RenderFragment works
			if entry.Name() == "base.html" {
				continue
			}
		}

		name := entry.Name()
		// Page file must come first so ParseFS names the template set after it.
		files := make([]string, 0, 1+len(layoutFiles)+len(partialFiles))
		files = append(files, filepath.Join("templates", name))
		files = append(files, layoutFiles...)
		files = append(files, partialFiles...)

		tmpl := template.New(name).Funcs(template.FuncMap{
			"navAttr": func(current, target string, extraClasses string) template.HTMLAttr {
				classes := "px-3 py-2 rounded-md text-sm font-medium transition"
				if extraClasses != "" {
					classes += " " + extraClasses
				}
				if current == target {
					classes += " bg-gray-700 text-white"
				} else {
					classes += " text-gray-300 hover:text-white hover:bg-gray-700"
				}
				// #nosec G203 -- classes are hardcoded or controlled internal strings
				return template.HTMLAttr(fmt.Sprintf(`class="%s"`, classes))
			},
			"navClass": func(current, target string) string {
				if current == target {
					return "px-3 py-2 rounded-md text-sm font-medium bg-gray-700 text-white transition"
				}
				return "px-3 py-2 rounded-md text-sm font-medium text-gray-300 hover:text-white hover:bg-gray-700 transition"
			},
			"multiply": func(a, b float64) float64 {
				return a * b
			},
			"percentage": func(current, start, end int64) float64 {
				if end == start {
					return 0
				}
				p := float64(current-start) / float64(end-start)
				if p < 0 {
					return 0
				}
				if p > 1 {
					return 1
				}
				return p
			},
			"progressStyle": func(current, start, end int64) template.HTMLAttr {
				if end == start {
					// #nosec G203 -- hardcoded safe attribute
					return template.HTMLAttr("style=\"width: 0%\"")
				}
				p := float64(current-start) / float64(end-start)
				if p < 0 {
					p = 0
				}
				if p > 1 {
					p = 1
				}
				// #nosec G203 -- calculated width percentage is safe
				return template.HTMLAttr(fmt.Sprintf("style=\"width: %.1f%%\"", p*100))
			},
			"workerIconClass": func(workerType any) string {
				wt := ""
				switch v := workerType.(type) {
				case string:
					wt = v
				case sql.NullString:
					if v.Valid {
						wt = v.String
					}
				}

				if wt == "pc" {
					return "flex-shrink-0 h-10 w-10 flex items-center justify-center rounded-lg bg-blue-100 text-blue-600"
				}
				return "flex-shrink-0 h-10 w-10 flex items-center justify-center rounded-lg bg-purple-100 text-purple-600"
			},
			"add": func(a, b int) int {
				return a + b
			},
			"rankBadgeAttr": func(index int) template.HTMLAttr {
				base := "inline-flex items-center justify-center h-6 w-6 rounded-full text-[11px] font-black"
				classes := ""
				switch index {
				case 0:
					classes = base + " bg-amber-100 text-amber-700"
				case 1:
					classes = base + " bg-slate-200 text-slate-700"
				case 2:
					classes = base + " bg-orange-100 text-orange-700"
				default:
					classes = base + " bg-gray-100 text-gray-500"
				}
				// #nosec G203 -- classes are hardcoded or controlled internal strings
				return template.HTMLAttr(fmt.Sprintf(`class="%s"`, classes))
			},
			"workerBadgeAttr": func(workerType any) template.HTMLAttr {
				wt := ""
				switch v := workerType.(type) {
				case string:
					wt = v
				case sql.NullString:
					if v.Valid {
						wt = v.String
					}
				}
				base := "inline-flex items-center px-2 py-0.5 rounded text-[10px] font-black uppercase tracking-widest"
				classes := ""
				if wt == "pc" {
					classes = base + " bg-blue-100 text-blue-700"
				} else {
					classes = base + " bg-green-100 text-green-700"
				}
				// #nosec G203 -- classes are hardcoded or controlled internal strings
				return template.HTMLAttr(fmt.Sprintf(`class="%s"`, classes))
			},
			"bgStyle": func(color string) template.HTMLAttr {
				// #nosec G203 -- hex colors are controlled and safe
				return template.HTMLAttr(fmt.Sprintf("style=\"background-color: %s\"", color))
			},
			"strokeStyle": func(color string) template.HTMLAttr {
				// #nosec G203
				return template.HTMLAttr(fmt.Sprintf("style=\"stroke: %s\"", color))
			},
			"json": func(v any) string {
				b, _ := json.Marshal(v)
				return string(b)
			},
			"errorTextClass": func(errVal any) string {
				var count float64
				switch v := errVal.(type) {
				case int:
					count = float64(v)
				case int64:
					count = float64(v)
				case float64:
					count = v
				case sql.NullInt64:
					if v.Valid {
						count = float64(v.Int64)
					}
				case sql.NullFloat64:
					if v.Valid {
						count = v.Float64
					}
				}

				if count > 0 {
					return "text-red-500 font-black"
				}
				return "text-gray-400 font-bold"
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
