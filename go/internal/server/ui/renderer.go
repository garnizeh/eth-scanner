package ui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
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
			"percentStyle": func(p float64) template.HTMLAttr {
				if p < 0 {
					p = 0
				}
				if p > 100 {
					p = 100
				}
				// #nosec G203 -- calculated percentage is safe
				return template.HTMLAttr(fmt.Sprintf("style=\"width: %.2f%%\"", p))
			},
			"chartHeightStyle": func(current int64, maxi int64) template.HTMLAttr {
				if maxi <= 0 {
					return template.HTMLAttr("style=\"height: 4px; min-height: 4px;\"")
				}
				p := (float64(current) / float64(maxi)) * 100
				if p < 1 && current > 0 {
					p = 1
				}
				if p > 100 {
					p = 100
				}
				// #nosec G203 -- calculated height percentage is safe
				return template.HTMLAttr(fmt.Sprintf("style=\"height: %.1f%%; min-height: 4px;\"", p))
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
			"subtract": func(a, b int64) int64 {
				return a - b
			},
			"formatCount": func(n int64) string {
				if n < 0 {
					return fmt.Sprintf("%d", n)
				}
				s := fmt.Sprintf("%d", n)
				var res []byte
				for i, j := len(s)-1, 0; i >= 0; i, j = i-1, j+1 {
					if j > 0 && j%3 == 0 {
						res = append([]byte{','}, res...)
					}
					res = append([]byte{s[i]}, res...)
				}
				return string(res)
			},
			"int": func(v any) int64 {
				switch val := v.(type) {
				case int64:
					return val
				case int:
					return int64(val)
				case uint32:
					return int64(val)
				case sql.NullInt64:
					if val.Valid {
						return val.Int64
					}
				}
				return 0
			},
			"float64": func(v any) float64 {
				switch val := v.(type) {
				case float64:
					return val
				case int64:
					return float64(val)
				case int:
					return float64(val)
				case uint32:
					return float64(val)
				case sql.NullFloat64:
					if val.Valid {
						return val.Float64
					}
				case sql.NullInt64:
					if val.Valid {
						return float64(val.Int64)
					}
				}
				return 0
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
			"truncateHex": func(v any) string {
				var s string
				switch val := v.(type) {
				case []byte:
					s = fmt.Sprintf("%x", val)
				case [28]byte:
					s = fmt.Sprintf("%x", val[:])
				case string:
					s = strings.TrimPrefix(val, "0x")
				default:
					s = fmt.Sprintf("%x", val)
				}
				if len(s) > 12 {
					return fmt.Sprintf("0x%s...%s", s[:4], s[len(s)-4:])
				}
				return "0x" + s
			},
			"fullHex": func(v any) string {
				var s string
				switch val := v.(type) {
				case []byte:
					s = fmt.Sprintf("%x", val)
				case [28]byte:
					s = fmt.Sprintf("%x", val[:])
				case string:
					s = strings.TrimPrefix(val, "0x")
				default:
					s = fmt.Sprintf("%x", val)
				}
				return "0x" + s
			},
			"prefixLinkAttr": func(v any) template.HTMLAttr {
				var s string
				switch val := v.(type) {
				case []byte:
					s = fmt.Sprintf("%x", val)
				case [28]byte:
					s = fmt.Sprintf("%x", val[:])
				case string:
					s = strings.TrimPrefix(val, "0x")
				default:
					s = fmt.Sprintf("%x", val)
				}
				// #nosec G203 -- hardcoded link path with hex value is safe
				return template.HTMLAttr(fmt.Sprintf(`href="/dashboard/prefixes/0x%s"`, s))
			},
			"workerLinkAttr": func(id any) template.HTMLAttr {
				// #nosec G203 -- hardcoded link path with id
				return template.HTMLAttr(fmt.Sprintf(`href="/dashboard/workers/%v"`, id))
			},
			"workerStatsLinkAttr": func(id any) template.HTMLAttr {
				// #nosec G203 -- hardcoded link path with id
				return template.HTMLAttr(fmt.Sprintf(`href="/dashboard/daily?worker_id=%v"`, id))
			},
			"errorStatusAttr": func(errVal any) template.HTMLAttr {
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

				classes := "text-gray-400 font-bold"
				if count > 0 {
					classes = "text-red-500 font-black"
				}
				// #nosec G203 -- classes are safe
				return template.HTMLAttr(fmt.Sprintf(`class="%s"`, classes))
			},
			"dataAttr": func(name string, val any) template.HTMLAttr {
				// #nosec G203 -- name is internal, val is escaped by %v
				return template.HTMLAttr(fmt.Sprintf(`data-%s="%v"`, name, val))
			},
			"dataFloatAttr": func(name string, v any, precision int) template.HTMLAttr {
				val := float64(0)
				if v != nil {
					switch vt := v.(type) {
					case float64:
						val = vt
					case float32:
						val = float64(vt)
					case int64:
						val = float64(vt)
					case int:
						val = float64(vt)
					case uint32:
						val = float64(vt)
					case sql.NullFloat64:
						if vt.Valid {
							val = vt.Float64
						}
					case sql.NullInt64:
						if vt.Valid {
							val = float64(vt.Int64)
						}
					}
				}
				format := fmt.Sprintf(`data-%%s="%%.%df"`, precision)
				// #nosec G203 -- name is internal, value is formatted float
				return template.HTMLAttr(fmt.Sprintf(format, name, val))
			},
			"titleAttr": func(v any) template.HTMLAttr {
				var s string
				switch val := v.(type) {
				case []byte:
					s = fmt.Sprintf("0x%x", val)
				case [28]byte:
					s = fmt.Sprintf("0x%x", val[:])
				case string:
					s = val
				default:
					s = fmt.Sprintf("%v", val)
				}
				// #nosec G203 -- value is escaped by %s
				return template.HTMLAttr(fmt.Sprintf(`title="%s"`, s))
			},
			"historyStatusAttr": func(msg sql.NullString) template.HTMLAttr {
				base := "px-6 py-3 whitespace-nowrap uppercase text-[10px] font-black"
				if msg.Valid && msg.String != "" {
					// #nosec G203 -- hardcoded classes are safe
					return template.HTMLAttr(fmt.Sprintf(`class="%s text-red-500"`, base))
				}
				// #nosec G203 -- hardcoded classes are safe
				return template.HTMLAttr(fmt.Sprintf(`class="%s text-green-600"`, base))
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
