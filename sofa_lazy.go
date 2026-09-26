package sofa

// OpenLazy opens a SOFA file like Open but leaves the audio data in the
// file: ImpulseResponses, TFReal/TFImag, TFRealE/TFImagE and
// SOSCoefficients stay empty, while the metadata, positions, sampling rate,
// delay and extras are read as by Open. The layout of the audio datasets is
// checked all the same, so a file OpenLazy accepts is one Open accepts.
//
// The returned File holds the file open until Close. Read FIR measurements
// with ReadMeasurement or RangeMeasurements; IRAt, IRPeakdB and Save need
// the audio in memory, so they fail on a lazy File.
func OpenLazy(path string) (*File, error) {
	return open(path, true)
}
