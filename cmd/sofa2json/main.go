// Command sofa2json exports SOFA files to JSON format.
// By default, exports metadata and dimensions only. Bulk audio data is
// gated by per-DataType flags:
//
//	--include-ir   FIR  files: include ImpulseResponses
//	--include-tf   TF / TF-E files: include TFReal/TFImag (TF-E adds emitter dim)
//	--include-sos  SOS  files: include SOSCoefficients
//
// Frequencies are included automatically (small) for TF / TF-E.
//
// Usage:
//
//	sofa2json [flags] <file.sofa>       # process single file
//	sofa2json [flags]                   # process all .sofa files in current directory
//
// Output is written to <filename>.json (replacing .sofa extension).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cwbudde/go-sofa"
)

// includeFlags carries the user's --include-* selections.
type includeFlags struct {
	IR, TF, SOS bool
}

func main() {
	args := os.Args[1:]
	var inc includeFlags

	var files []string
	for _, arg := range args {
		switch arg {
		case "--include-ir":
			inc.IR = true
		case "--include-tf":
			inc.TF = true
		case "--include-sos":
			inc.SOS = true
		default:
			files = append(files, arg)
		}
	}

	if len(files) >= 1 {
		// Single file mode
		if err := processSofaFile(files[0], inc); err != nil {
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
			if err := processSofaFile(filename, inc); err != nil {
				fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filename, err)
			}
		}
	}
}

func processSofaFile(filename string, inc includeFlags) error {
	f, err := sofa.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	// Build JSON object
	jsonObj := buildJSONObject(f, inc)

	// Marshal to pretty JSON
	data, err := json.MarshalIndent(jsonObj, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	// Write to output file (filename is provided by the user on the CLI).
	outFile := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".json"
	if err := os.WriteFile(outFile, data, 0o600); err != nil { //nolint:gosec // user-supplied output path
		return fmt.Errorf("write output: %w", err)
	}

	return nil
}

func buildJSONObject(f *sofa.File, inc includeFlags) map[string]interface{} {
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

	// FIR audio data
	if inc.IR && len(f.ImpulseResponses) > 0 {
		result["IR"] = f.ImpulseResponses
	}

	// TF / TF-E audio data
	switch f.DataType {
	case "TF":
		if len(f.Frequencies) > 0 {
			result["Frequencies"] = f.Frequencies
		}
		if inc.TF {
			result["TFReal"] = f.TFReal
			result["TFImag"] = f.TFImag
		}
	case "TF-E":
		if len(f.Frequencies) > 0 {
			result["Frequencies"] = f.Frequencies
		}
		if inc.TF {
			result["TFReal"] = f.TFRealE
			result["TFImag"] = f.TFImagE
		}
	case "SOS":
		if inc.SOS {
			result["SOSCoefficients"] = f.SOSCoefficients
		}
	}

	return result
}
