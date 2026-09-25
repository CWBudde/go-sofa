package sofa

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzOpen feeds arbitrary bytes to Open and exercises the accessors on
// any file that opens. It must never panic. Seeds are small valid files
// of every supported DataType, produced by Save at test time.
//
// Run with e.g.:
//
//	GOMEMLIMIT=2GiB go test -run '^$' -fuzz FuzzOpen -fuzztime 60s
func FuzzOpen(f *testing.F) {
	dir := f.TempDir()
	for _, sf := range saveFixtures() {
		path := filepath.Join(dir, "seed.sofa")
		if err := sf.Save(path); err != nil {
			f.Fatalf("Save %s seed: %v", sf.DataType, err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "in.sofa")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		sf, err := Open(path)
		if err != nil {
			return
		}
		defer sf.Close()

		_ = sf.SamplingRateScalar()
		_ = sf.Duration()
		for _, idx := range [][2]int{{0, 0}, {sf.M - 1, sf.R - 1}, {sf.M, 0}, {-1, 0}} {
			_ = sf.IRAt(idx[0], idx[1])
			_ = sf.IRPeakdB(idx[0], idx[1])
		}
		_, _ = sf.SHOrder()
		_ = sf.SHCoefficientCount()
		_ = sf.SHWarnings()

		// Shapes must match the declared dimensions for the DataType.
		switch sf.DataType {
		case dataTypeTF:
			if err := check3D("TFReal", sf.TFReal, sf.M, sf.R, sf.N); err != nil {
				t.Fatal(err)
			}
		case dataTypeTFE:
			if err := check4D("TFRealE", sf.TFRealE, sf.M, sf.R, sf.E, sf.N); err != nil {
				t.Fatal(err)
			}
		case dataTypeSOS:
			if err := check3D("SOSCoefficients", sf.SOSCoefficients, sf.M, sf.R, sf.N); err != nil {
				t.Fatal(err)
			}
		default:
			if err := check3D("ImpulseResponses", sf.ImpulseResponses, sf.M, sf.R, sf.N); err != nil {
				t.Fatal(err)
			}
		}
	})
}
