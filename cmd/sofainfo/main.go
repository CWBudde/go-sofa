// Command sofainfo prints metadata summary for SOFA files.
// It displays AES69 global attributes, dimensions, and basic audio parameters.
//
// Usage:
//
//	sofainfo <file.sofa>       # process single file
//	sofainfo                   # process all .sofa files in current directory
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cwbudde/go-sofa"
)

func main() {
	if len(os.Args) >= 2 {
		// Single file mode
		if err := processSofaFile(os.Args[1]); err != nil {
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
			if err := processSofaFile(filename); err != nil {
				fmt.Fprintf(os.Stderr, "error processing %s: %v\n", filename, err)
			}
		}
	}
}

func processSofaFile(filename string) error {
	f, err := sofa.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	printFileInformation(f)
	return nil
}

func printFileInformation(f *sofa.File) {
	// Print all AES69 global attributes (if non-empty)
	if f.Title != "" {
		fmt.Printf("Title: %s\n", f.Title)
	}
	if f.DataType != "" {
		fmt.Printf("DataType: %s\n", f.DataType)
	}
	if f.RoomType != "" {
		fmt.Printf("RoomType: %s\n", f.RoomType)
	}
	if f.DateCreated != "" {
		fmt.Printf("DateCreated: %s\n", f.DateCreated)
	}
	if f.DateModified != "" {
		fmt.Printf("DateModified: %s\n", f.DateModified)
	}
	if f.APIName != "" {
		fmt.Printf("APIName: %s\n", f.APIName)
	}
	if f.APIVersion != "" {
		fmt.Printf("APIVersion: %s\n", f.APIVersion)
	}
	if f.AuthorContact != "" {
		fmt.Printf("AuthorContact: %s\n", f.AuthorContact)
	}
	if f.Organization != "" {
		fmt.Printf("Organization: %s\n", f.Organization)
	}
	if f.License != "" {
		fmt.Printf("License: %s\n", f.License)
	}
	if f.ApplicationName != "" {
		fmt.Printf("ApplicationName: %s\n", f.ApplicationName)
	}
	if f.ApplicationVersion != "" {
		fmt.Printf("ApplicationVersion: %s\n", f.ApplicationVersion)
	}
	if f.Comment != "" {
		fmt.Printf("Comment: %s\n", f.Comment)
	}
	if f.History != "" {
		fmt.Printf("History: %s\n", f.History)
	}
	if f.References != "" {
		fmt.Printf("References: %s\n", f.References)
	}
	if f.Origin != "" {
		fmt.Printf("Origin: %s\n", f.Origin)
	}

	fmt.Println()

	// Print dimensions and audio parameters
	fmt.Printf("Number of Measurements: %d\n", f.M)
	fmt.Printf("Number of Receivers: %d\n", f.R)
	fmt.Printf("Number of Emitters: %d\n", f.E)
	fmt.Printf("Number of DataSamples: %d\n", f.N)
	if len(f.SamplingRate) > 0 {
		fmt.Printf("SampleRate: %g\n", f.SamplingRate[0])
	}
	if len(f.Delay) > 0 {
		fmt.Printf("Delay: %g\n", f.Delay[0])
	}
	if (f.DataType == "TF" || f.DataType == "TF-E") && len(f.Frequencies) > 0 {
		fmt.Printf("Frequencies: %d points, %g Hz – %g Hz\n",
			len(f.Frequencies),
			f.Frequencies[0],
			f.Frequencies[len(f.Frequencies)-1])
	}
	if f.DataType == "SOS" && f.N > 0 {
		fmt.Printf("Biquad sections per filter: %d (N=%d)\n", f.N/6, f.N)
	}
}
