package scenarios

import (
	"embed"
	"io/fs"

	"github.com/aalvaropc/upkeep/internal/fixture"
	"github.com/aalvaropc/upkeep/internal/scenario"
)

//go:embed all:notes
var files embed.FS

func NotesFS() fs.FS {
	sub, err := fs.Sub(files, "notes")
	if err != nil {
		panic(err)
	}
	return sub
}

func Notes() (scenario.Scenario, error) {
	f, err := NotesFS().Open("scenario.json")
	if err != nil {
		return scenario.Scenario{}, err
	}
	defer f.Close()
	return scenario.Load(f)
}

func Fixtures() (fixture.Store, bool) {
	sub, err := fs.Sub(files, "notes/fixtures")
	if err != nil {
		return nil, false
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil || len(entries) == 0 {
		return nil, false
	}
	return fixture.FS{FS: sub}, true
}
