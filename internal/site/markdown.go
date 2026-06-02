package site

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// md is the shared markdown renderer: GFM so the dossier and schema tables
// render. Rendering is deterministic (no timestamps, stable element order).
var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// renderMarkdown converts CommonMark/GFM source to an HTML fragment, then
// rewrites every intra-repo link so it points at the generated site (for paths
// that became pages) or at the GitHub source (for everything else). prefix is
// the page's path-to-root ("" for a root page, "../" one level down) so links
// resolve under the GitHub Pages project base path.
func renderMarkdown(src []byte, repoURL, prefix string) (string, error) {
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		return "", err
	}
	return wrapTables(rewriteLinks(buf.String(), repoURL, prefix)), nil
}

// tableRE matches a complete rendered table block.
var tableRE = regexp.MustCompile(`(?s)<table>.*?</table>`)

// wrapTables puts every table in a horizontally-scrollable container so wide
// tables (e.g. the downloads manifest) side-scroll on narrow screens instead of
// overflowing the page.
func wrapTables(html string) string {
	return tableRE.ReplaceAllString(html, `<div class="table-wrap">$0</div>`)
}

var attrRE = regexp.MustCompile(`(href|src)="([^"]*)"`)

// sitePages maps a repo-relative path to the generated page that supersedes it.
// Anything not listed (and not external) is treated as source to browse on
// GitHub.
var sitePages = map[string]string{
	"README.md":                       "index.html",
	"docs/schema.md":                  "schema.html",
	"CHANGELOG.md":                    "changelog.html",
	"datasets/episodes.manifest.json": "downloads.html",
	"dossiers":                        "dossiers/index.html",
	"dossiers/":                       "dossiers/index.html",
}

func rewriteLinks(html, repoURL, prefix string) string {
	return attrRE.ReplaceAllStringFunc(html, func(m string) string {
		sm := attrRE.FindStringSubmatch(m)
		attr, target := sm[1], sm[2]
		return attr + `="` + rewriteTarget(target, repoURL, prefix) + `"`
	})
}

func rewriteTarget(target, repoURL, prefix string) string {
	// External, anchor, or mail links are left untouched.
	if target == "" || strings.HasPrefix(target, "#") ||
		strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
		return target
	}
	// Split off any anchor/query so it survives the rewrite.
	suffix := ""
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		suffix, target = target[i:], target[:i]
	}
	rel := strings.TrimLeft(target, "./") // collapse leading ./ and ../

	// A dossier write-up becomes its generated page.
	if strings.HasPrefix(rel, "dossiers/") && strings.HasSuffix(rel, ".md") {
		return prefix + strings.TrimSuffix(rel, ".md") + ".html" + suffix
	}
	// The logo is the one asset the site ships locally.
	if rel == "assets/logo.png" {
		return prefix + "assets/logo.png" + suffix
	}
	if page, ok := sitePages[rel]; ok {
		return prefix + page + suffix
	}
	// Everything else is repo source: link to it on GitHub.
	return strings.TrimRight(repoURL, "/") + "/blob/main/" + rel + suffix
}
