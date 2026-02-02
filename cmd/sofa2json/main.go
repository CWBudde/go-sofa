// Command sofa2json exports SOFA files to JSON format.
// By default, exports metadata and dimensions only. Use --include-ir to include impulse response data.
//
// Usage:
//
//	sofa2json [--include-ir] <file.sofa>       # process single file
//	sofa2json [--include-ir]                   # process all .sofa files in current directory
//
// Output is written to <filename>.json (replacing .sofa extension).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MeKo-Christian/go-sofa"
)

func main() {
	args := os.Args[1:]
	includeIR := false

	// Parse --include-ir flag
	var files []string
	for _, arg := range args {
		if arg == "--include-ir" {
			includeIR = true
		} else {
			files = append(files, arg)
		}
	}

	if len(files) >= 1 {
		// Single file mode
		if err := processSofaFile(files[0], includeIR); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Batch mode: process all .sofa files in current directory
		matches, err := filepath.Glob("*.sofa")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if len(matches) == 0 {
			fmt.Fprintf(os.Stderr, "no .sofa files found in current directory\n")
			os.Exit(1)
		}
		for _, filename := range matches {
			fmt.Printf("Process file %s\n", filepath.Base(filename))
			if err := processSofaFile(filename, includeIR); err != nil {
				fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filename, err)
			}
		}
	}
}

func processSofaFile(filename string, includeIR bool) error {
	f, err := sofa.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Build JSON object
	jsonObj := buildJSONObject(f, includeIR)

	// Marshal to pretty JSON
	data, err := json.MarshalIndent(jsonObj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	// Write to output file
	outFile := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json"
	if err := os.WriteFile(outFile, data, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

func buildJSONObject(f *sofa.File, includeIR bool) map[string]interface{} {
	result := make(map[string]interface{})

	// Add all AES69 global attributes (if non-empty)
	if f.Title != "" {
		result["Title"] = f.Title
	}
	if f.DataType != "" {
		result["DataType"] = f.DataType
	}
	if f.RoomType != "" {
		result["RoomType"] = f.RoomType
	}
	if f.DateCreated != "" {
		result["DateCreated"] = f.DateCreated
	}
	if f.DateModified != "" {
		result["DateModified"] = f.DateModified
	}
	if f.APIName != "" {
		result["APIName"] = f.APIName
	}
	if f.APIVersion != "" {
		result["APIVersion"] = f.APIVersion
	}
	if f.AuthorContact != "" {
		result["AuthorContact"] = f.AuthorContact
	}
	if f.Organization != "" {
		result["Organization"] = f.Organization
	}
	if f.License != "" {
		result["License"] = f.License
	}
	if f.ApplicationName != "" {
		result["ApplicationName"] = f.ApplicationName
	}
	if f.ApplicationVersion != "" {
		result["ApplicationVersion"] = f.ApplicationVersion
	}
	if f.Comment != "" {
		result["Comment"] = f.Comment
	}
	if f.History != "" {
		result["History"] = f.History
	}
	if f.References != "" {
		result["References"] = f.References
	}
	if f.Origin != "" {
		result["Origin"] = f.Origin
	}

	// Add dimensions
	result["Measurements"] = f.M
	result["Receivers"] = f.R
	result["Emitters"] = f.E
	result["DataSamples"] = f.N

	// Add sampling rate array
	result["SampleRate"] = f.SamplingRate

	// Add delay array
	result["Delay"] = f.Delay

	// Add IR data if requested
	if includeIR {
		result["IR"] = f.ImpulseResponses
	}

	return result
}
