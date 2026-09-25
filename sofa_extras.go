package sofa

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// Attribute is a netCDF attribute go-sofa does not map to a File field.
// Value is a string, a numeric scalar (int8 … uint64, float32, float64) or
// a non-empty slice of one numeric type. Open reads empty (null) attributes
// as "".
type Attribute struct {
	Name  string
	Value any
}

// Variable is a netCDF variable go-sofa does not interpret, such as
// SourceView, RoomCornerA or a char array like ReceiverDescriptions.
//
// Shape gives the size of each axis and Dims their netCDF dimension names
// (nil when the file does not name them; Save then writes the variable
// without dimensions, which netCDF reads as phony ones). Exactly one of
// Values and Chars holds the data, row-major: Values for numeric variables,
// which Open reads and Save writes as float64, and Chars for char arrays,
// one byte per element (0 for an unset character).
type Variable struct {
	Name       string
	Dims       []string
	Shape      []int
	Values     []float64
	Chars      []byte
	Attributes []Attribute
}

// attrUnits is the attribute naming a variable's units.
const attrUnits = "Units"

// plumbingAttribute reports whether a variable attribute belongs to the
// HDF5 dimension-scale or netCDF-4 machinery, which Save writes itself.
func plumbingAttribute(name string) bool {
	switch name {
	case "CLASS", "NAME", "DIMENSION_LIST", "REFERENCE_LIST":
		return true
	}
	return strings.HasPrefix(name, "_Netcdf4")
}

// netcdfAttribute reports whether a global attribute is netCDF's own, which
// Save writes itself.
func netcdfAttribute(name string) bool {
	return name == "_NCProperties" || strings.HasPrefix(name, "_Netcdf4")
}

// attributeString converts a decoded string attribute to a Go string. go-hdf5
// decodes an attribute with a null dataspace (the SOFA Toolbox's empty
// string) as an empty []interface{}; that and nil read as "".
func attributeString(val any) string {
	switch v := val.(type) {
	case nil:
		return ""
	case string:
		return v
	case []interface{}:
		if len(v) == 0 {
			return ""
		}
	}
	return fmt.Sprintf("%v", val)
}

// attributeValue returns a decoded attribute value in the form Attribute
// keeps, or false when Save could not write it back.
func attributeValue(val any) (any, bool) {
	if v, ok := val.([]interface{}); val == nil || ok && len(v) == 0 {
		return "", true
	}
	return val, supportedAttributeValue(val)
}

// supportedAttributeValue reports whether Save can write v as an attribute.
func supportedAttributeValue(v any) bool {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return false
	}
	if rv.Kind() == reflect.String {
		return true
	}
	if rv.Kind() == reflect.Slice {
		return rv.Len() > 0 && numericKind(rv.Type().Elem().Kind())
	}
	return numericKind(rv.Kind())
}

func numericKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// attributeOptions turns attributes into dataset options for Save.
func attributeOptions(attrs []Attribute) []hdf5.DatasetOption {
	opts := make([]hdf5.DatasetOption, len(attrs))
	for i, a := range attrs {
		opts[i] = hdf5.WithAttribute(a.Name, a.Value)
	}
	return opts
}

// keepGlobalAttribute appends a global attribute go-sofa does not map to
// f.Attributes, or records in f.Dropped why it cannot.
func (f *File) keepGlobalAttribute(a globalAttribute) {
	val, err := a.read()
	if err != nil {
		f.Dropped = append(f.Dropped, fmt.Sprintf("global attribute %s: %v", a.name, err))
		return
	}
	v, ok := attributeValue(val)
	if !ok {
		f.Dropped = append(f.Dropped, fmt.Sprintf("global attribute %s: unsupported type %T", a.name, val))
		return
	}
	f.Attributes = append(f.Attributes, Attribute{Name: a.name, Value: v})
}

// writtenVariables returns the variables Save writes for f, dimension
// scales included, each with the attributes Save sets on it itself (the
// dimension-scale plumbing aside).
func (f *File) writtenVariables() map[string][]string {
	units := []string{attrUnits}
	typeUnits := []string{"Type", attrUnits}
	written := map[string][]string{
		datasetListenerView: typeUnits,
		datasetListenerUp:   typeUnits,
	}
	for _, d := range sofaDimensions {
		written[d] = nil
	}
	for _, p := range []struct {
		name    string
		written bool
	}{
		{datasetListenerPosition, len(f.ListenerPositions) > 0},
		{datasetReceiverPosition, len(f.ReceiverPositions) > 0 || len(f.ReceiverPositionsM) > 0},
		{datasetSourcePosition, len(f.SourcePositions) > 0},
		{datasetEmitterPosition, len(f.EmitterPositions) > 0 || len(f.EmitterPositionsM) > 0},
	} {
		if p.written {
			written[p.name] = typeUnits
		}
	}
	if f.RoomVolume != 0 {
		written[datasetRoomVolume] = units
	}
	if f.RoomTemperature != 0 {
		written[datasetRoomTemperature] = units
	}
	switch f.DataType {
	case DataTypeFIR, DataTypeSOS:
		data := "Data.IR"
		if f.DataType == DataTypeSOS {
			data = "Data.SOS"
		}
		written[data] = nil
		written["Data.SamplingRate"] = units
		written["Data.Delay"] = nil
	case DataTypeTF, DataTypeTFE:
		written["Data.Real"] = nil
		written["Data.Imag"] = nil
		written[dimN] = []string{"LongName", attrUnits} // the frequency coordinate variable
	}
	return written
}

// readExtras keeps what Open did not interpret: the attributes of the
// variables and dimension scales Save writes beyond the ones it sets, and
// every other variable that is not a dimension scale. Whatever cannot be
// kept, including attributes of further dimension scales, is listed in
// f.Dropped; nothing here fails Open.
func (f *File) readExtras(datasets map[string]*hdf5.Dataset) {
	var scales []string
	for name, ds := range datasets {
		if slices.Contains(sofaDimensions, name) || isDimensionScale(ds) {
			scales = append(scales, name)
		}
	}
	labels := dimensionLabels(datasets, scales)

	names := make([]string, 0, len(datasets))
	for name := range datasets {
		names = append(names, name)
	}
	sort.Strings(names)
	written := f.writtenVariables()
	for _, name := range names {
		ds := datasets[name]
		if owned, ok := written[name]; ok {
			if attrs := f.readVariableAttributes(name, ds, owned); len(attrs) > 0 {
				if f.VariableAttributes == nil {
					f.VariableAttributes = map[string][]Attribute{}
				}
				f.VariableAttributes[name] = attrs
			}
			continue
		}
		if slices.Contains(scales, name) {
			if attrs := f.readVariableAttributes(name, ds, nil); len(attrs) > 0 {
				f.Dropped = append(f.Dropped, fmt.Sprintf("attributes of dimension %s: not preserved", name))
			}
			continue
		}
		v, err := readVariable(name, ds, labels[name])
		if err != nil {
			f.Dropped = append(f.Dropped, fmt.Sprintf("variable %s: %v", name, err))
			continue
		}
		v.Attributes = f.readVariableAttributes(name, ds, nil)
		f.Variables = append(f.Variables, v)
	}
}

// isDimensionScale reports whether ds is an HDF5 dimension scale.
func isDimensionScale(ds *hdf5.Dataset) bool {
	class, err := readStringAttribute(ds, "CLASS")
	return err == nil && class == "DIMENSION_SCALE"
}

// readVariable reads a variable go-sofa does not interpret: a numeric one
// as float64 values, a char array (1-byte strings) as bytes.
func readVariable(name string, ds *hdf5.Dataset, dims []string) (Variable, error) {
	info, err := ds.Info()
	if err != nil {
		return Variable{}, err
	}
	shape, ok := parseDataspaceShape(info)
	if !ok {
		return Variable{}, fmt.Errorf("cannot determine the shape")
	}
	if len(shape) == 0 {
		return Variable{}, fmt.Errorf("scalar variables are not supported")
	}
	if n, _ := parseDataspaceElements(info); n > maxDataElements {
		return Variable{}, fmt.Errorf("element count exceeds limit %d", maxDataElements)
	}
	v := Variable{Name: name, Shape: make([]int, len(shape))}
	for i, d := range shape {
		v.Shape[i] = int(d) //nolint:gosec // bounded by maxDataElements above
	}
	if len(dims) == len(shape) {
		v.Dims = dims
	}
	n, err := dimProduct(v.Shape...)
	if err != nil {
		return Variable{}, err
	}

	if strings.HasPrefix(info, "Dataset: string") {
		if !strings.HasPrefix(info, "Dataset: string (size=1 bytes)") {
			return Variable{}, fmt.Errorf("only 1-byte (char) strings are supported")
		}
		strs, err := ds.ReadStrings()
		if err != nil {
			return Variable{}, err
		}
		v.Chars = make([]byte, len(strs))
		for i, s := range strs {
			if s != "" {
				v.Chars[i] = s[0]
			}
		}
		if len(v.Chars) != n {
			return Variable{}, fmt.Errorf("read %d characters, want %d", len(v.Chars), n)
		}
		return v, nil
	}
	if v.Values, err = ds.Read(); err != nil {
		return Variable{}, err
	}
	if len(v.Values) != n {
		return Variable{}, fmt.Errorf("read %d values, want %d", len(v.Values), n)
	}
	return v, nil
}

// readVariableAttributes returns the attributes of variable name except the
// dimension-scale plumbing and the ones in owned, which Save writes itself.
// Attributes that cannot be kept are listed in f.Dropped.
func (f *File) readVariableAttributes(name string, ds *hdf5.Dataset, owned []string) []Attribute {
	all, err := ds.Attributes()
	if err != nil {
		f.Dropped = append(f.Dropped, fmt.Sprintf("attributes of %s: %v", name, err))
		return nil
	}
	var attrs []Attribute
	for _, a := range all {
		if plumbingAttribute(a.Name) || slices.Contains(owned, a.Name) {
			continue
		}
		val, err := a.ReadValue()
		if err != nil {
			f.Dropped = append(f.Dropped, fmt.Sprintf("attribute %s:%s: %v", name, a.Name, err))
			continue
		}
		v, ok := attributeValue(val)
		if !ok {
			f.Dropped = append(f.Dropped, fmt.Sprintf("attribute %s:%s: unsupported type %T", name, a.Name, val))
			continue
		}
		attrs = append(attrs, Attribute{Name: a.Name, Value: v})
	}
	return attrs
}

// writeExtraVariables writes f.Variables after everything go-sofa
// interprets, attached to their dimension scales.
func (nc *netcdfDimensions) writeExtraVariables(vars []Variable) error {
	for _, v := range vars {
		if err := nc.writeExtraVariable(v); err != nil {
			return fmt.Errorf("write %s: %w", v.Name, err)
		}
	}
	return nil
}

func (nc *netcdfDimensions) writeExtraVariable(v Variable) error {
	shape := make([]uint64, len(v.Shape))
	for i, s := range v.Shape {
		shape[i] = uint64(s) //nolint:gosec // sizes > 0 by validateExtras
	}
	opts := attributeOptions(v.Attributes)
	var ds *hdf5.DatasetWriter
	var err error
	if v.Chars != nil {
		ds, err = nc.fw.CreateDataset("/"+v.Name, hdf5.String, shape, append(opts, hdf5.WithStringSize(1))...)
		if err != nil {
			return fmt.Errorf("create dataset: %w", err)
		}
		chars := make([]string, len(v.Chars))
		for i, c := range v.Chars {
			if c != 0 {
				chars[i] = string([]byte{c})
			}
		}
		err = ds.Write(chars)
	} else {
		ds, err = nc.fw.CreateDataset("/"+v.Name, hdf5.Float64, shape, opts...)
		if err != nil {
			return fmt.Errorf("create dataset: %w", err)
		}
		err = ds.Write(v.Values)
	}
	if err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	for i, d := range v.Dims {
		if err := ds.AttachDimensionScale(i, nc.scales[d]); err != nil {
			return fmt.Errorf("attach dimension %s: %w", d, err)
		}
	}
	return nil
}

// validateExtras checks Attributes, VariableAttributes and Variables: names
// must not clash with what Save writes itself, values must be writable, and
// variable shapes must agree with their data and with the file's
// dimensions.
func (f *File) validateExtras() error {
	mapped := f.globalFields()
	if err := checkAttributes("global attribute", f.Attributes, func(name string) bool {
		_, ok := mapped[name]
		return ok || netcdfAttribute(name)
	}); err != nil {
		return err
	}

	written := f.writtenVariables()
	names := make([]string, 0, len(f.VariableAttributes))
	for name := range f.VariableAttributes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		owned, ok := written[name]
		if !ok {
			return fmt.Errorf("variable attributes for %s, which Save does not write", name)
		}
		if err := checkAttributes("attribute of "+name, f.VariableAttributes[name], func(a string) bool {
			return plumbingAttribute(a) || slices.Contains(owned, a)
		}); err != nil {
			return err
		}
	}

	sizes := map[string]int{dimM: f.M, dimR: f.R, dimE: f.E, dimN: f.N, dimC: 3, dimI: 1}
	seen := map[string]bool{}
	for _, v := range f.Variables {
		if err := v.validate(sizes); err != nil {
			return fmt.Errorf("variable %s: %w", v.Name, err)
		}
		if _, ok := written[v.Name]; ok {
			return fmt.Errorf("variable %s: Save already writes a variable of that name", v.Name)
		}
		if seen[v.Name] {
			return fmt.Errorf("duplicate variable %s", v.Name)
		}
		seen[v.Name] = true
	}
	for d := range sizes {
		if seen[d] {
			return fmt.Errorf("variable %s: name taken by dimension %s", d, d)
		}
		if _, ok := written[d]; ok && !slices.Contains(sofaDimensions, d) {
			return fmt.Errorf("dimension %s: Save already writes a variable of that name", d)
		}
	}
	return nil
}

// validate checks one extra variable and records the size of each named
// dimension in sizes, which must agree across variables.
func (v Variable) validate(sizes map[string]int) error {
	if v.Name == "" || strings.Contains(v.Name, "/") {
		return fmt.Errorf("invalid variable name %q", v.Name)
	}
	if (v.Values == nil) == (v.Chars == nil) {
		return fmt.Errorf("set exactly one of Values and Chars")
	}
	if len(v.Shape) == 0 {
		return fmt.Errorf("empty Shape")
	}
	n, err := dimProduct(v.Shape...)
	if err != nil {
		return fmt.Errorf("shape %v: %w", v.Shape, err)
	}
	if got := len(v.Values) + len(v.Chars); got != n {
		return fmt.Errorf("%d elements for Shape %v", got, v.Shape)
	}
	if v.Dims != nil && len(v.Dims) != len(v.Shape) {
		return fmt.Errorf("%d Dims for Shape %v", len(v.Dims), v.Shape)
	}
	for i, d := range v.Dims {
		if d == "" || strings.Contains(d, "/") {
			return fmt.Errorf("invalid dimension name %q", d)
		}
		if size, ok := sizes[d]; ok && size != v.Shape[i] {
			return fmt.Errorf("dimension %s has size %d, want %d", d, v.Shape[i], size)
		}
		sizes[d] = v.Shape[i]
	}
	return checkAttributes("attribute", v.Attributes, plumbingAttribute)
}

// checkAttributes checks that attrs have distinct, non-empty names that are
// not reserved, and values Save can write.
func checkAttributes(what string, attrs []Attribute, reserved func(string) bool) error {
	seen := map[string]bool{}
	for _, a := range attrs {
		switch {
		case a.Name == "":
			return fmt.Errorf("%s with an empty name", what)
		case reserved(a.Name):
			return fmt.Errorf("%s %s is written by Save itself", what, a.Name)
		case seen[a.Name]:
			return fmt.Errorf("duplicate %s %s", what, a.Name)
		case !supportedAttributeValue(a.Value):
			return fmt.Errorf("%s %s: unsupported value type %T", what, a.Name, a.Value)
		}
		seen[a.Name] = true
	}
	return nil
}
