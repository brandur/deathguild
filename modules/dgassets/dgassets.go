package dgassets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/brandur/modulir"
	"github.com/brandur/modulir/modules/mfile"
	"github.com/yosssi/gcss"
)

// CompileJavascripts compiles a set of JS files into a single large file by
// appending them all to each other. Files are appended in alphabetical order
// so we depend on the fact that there aren't too many interdependencies
// between files. A common requirement can be given an underscore prefix to be
// loaded first.
func CompileJavascripts(c *modulir.Context, inPath, outPath string) error {
	sources, err := mfile.ReadDirWithOptions(c, inPath,
		&mfile.ReadDirOptions{ShowMeta: true})
	if err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, source := range sources {
		inFile, err := os.Open(source)
		if err != nil {
			return err
		}

		outFile.WriteString("/* " + filepath.Base(source) + " */\n\n")
		outFile.WriteString("(function() {\n\n")

		// Ignore non-JS files in the directory (I have a README in there)
		if filepath.Ext(source) == ".js" {
			_, err = io.Copy(outFile, inFile)
			if err != nil {
				return err
			}
		}

		outFile.WriteString("\n\n")
		outFile.WriteString("}).call(this);\n\n")
	}

	return nil
}

// CompileStylesheets compiles a set of stylesheet files into a single large
// file by appending them all to each other. Files are appended in alphabetical
// order so we depend on the fact that there aren't too many interdependencies
// between files. CSS reset in particular is given an underscore prefix so that
// it gets to load first.
//
// If a file has a ".sass" suffix, we attempt to render it as GCSS. This isn't
// a perfect symmetry, but works well enough for these cases.
func CompileStylesheets(c *modulir.Context, inPath, outPath string) error {
	sources, err := mfile.ReadDirWithOptions(c, inPath,
		&mfile.ReadDirOptions{ShowMeta: true})
	if err != nil {
		return err
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	for _, source := range sources {
		inFile, err := os.Open(source)
		if err != nil {
			return err
		}

		outFile.WriteString("/* " + filepath.Base(source) + " */\n\n")

		if filepath.Ext(source) == ".sass" {
			_, err := gcss.Compile(outFile, inFile)
			if err != nil {
				return fmt.Errorf("Error compiling '%v': %v", source, err)
			}
		} else {
			_, err := io.Copy(outFile, inFile)
			if err != nil {
				return err
			}
		}

		outFile.WriteString("\n\n")
	}

	return nil
}
