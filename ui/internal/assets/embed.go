package assets

import (
	"embed"
	"io/fs"
	"strings"
)

// templateFiles holds the page templates.
//
//go:embed templates/*.html
var templateFiles embed.FS

// servedFiles holds every file the asset handler may serve. Only the listed
// patterns are embedded, so folder documentation is never served.
//
//go:embed static/*.js static/*.css static/*.html
//go:embed static/leaflet/*.js static/leaflet/*.css static/leaflet/images/*.png
//go:embed licenses/*.txt
var servedFiles embed.FS

// catalog maps each servable path, relative to the asset base, to its
// embedded file name. It is built once and never modified.
var catalog = buildCatalog()

func buildCatalog() map[string]string {
	paths := map[string]string{}
	for _, root := range []string{"static", "licenses"} {
		err := fs.WalkDir(servedFiles, root, func(name string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			served := strings.TrimPrefix(name, "static/")
			paths[served] = name
			return nil
		})
		if err != nil {
			panic("assets: walk embedded files: " + err.Error())
		}
	}
	return paths
}

// Lookup returns the bytes of one servable asset. name is relative to the
// asset base without a leading slash, for example "ui.css" or
// "licenses/leaflet-LICENSE.txt". Unknown names, directories and traversal
// attempts report false.
func Lookup(name string) ([]byte, bool) {
	embedded, ok := catalog[name]
	if !ok {
		return nil, false
	}
	data, err := servedFiles.ReadFile(embedded)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Template returns the text of one page template, for example
// "manager.html".
func Template(name string) (string, error) {
	data, err := templateFiles.ReadFile("templates/" + name)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
