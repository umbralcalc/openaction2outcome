package site

import (
	_ "embed"
	"html/template"
)

//go:embed assets/style.css
var styleCSS string

//go:embed assets/layout.html.tmpl
var layoutTmpl string

//go:embed assets/home.html.tmpl
var homeTmpl string

//go:embed assets/downloads.html.tmpl
var downloadsTmpl string

//go:embed assets/dossiers.html.tmpl
var dossiersTmpl string

// layout is the shared page chrome (header, nav, footer). Every page renders its
// body separately and is wrapped by this template.
var layout = template.Must(template.New("layout").Parse(layoutTmpl))

// navItems are the top-level navigation links, in display order. Href is
// site-relative (a page rendered prepends its path-to-root prefix).
var navItems = []struct{ Href, Label string }{
	{"index.html", "Home"},
	{"downloads.html", "Datasets"},
	{"schema.html", "Schema"},
	{"dossiers/index.html", "Dossiers"},
	{"changelog.html", "Changelog"},
}

// page is the data a single rendered page hands to the layout template.
type page struct {
	Title   string        // <title> and header line
	Active  string        // nav Href of the current page (for highlighting)
	Prefix  string        // path-to-root: "" for a root page, "../" one level down
	Body    template.HTML // the page-specific HTML
	RepoURL string
	HFURL   string
	Nav     []navItem
}

type navItem struct {
	Href, Label string
	Current     bool
}
