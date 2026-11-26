// Package buildfiles names the files the build side writes into dist and the
// manifest handlers read. generate and devcmd write and watch them; manifest reads them.
package buildfiles

const (
	// Manifest is the asset manifest file.
	Manifest = "tc.json"
	// DevError is written beside Manifest when a dev build fails.
	DevError = "tc-error.json"
	// Generations records the files of the last builds and marks the directory as
	// templatic's.
	Generations = ".generations.json"
)
