package accesslog

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"unsafe"
)

const (
	startTag       = "${"
	endTag         = "}"
	paramSeparator = ":"
)

type Buffer interface {
	Len() int
	ReadFrom(r io.Reader) (int64, error)
	WriteTo(w io.Writer) (int64, error)
	Bytes() []byte
	Write(p []byte) (int, error)
	WriteByte(c byte) error
	WriteString(s string) (int, error)
	Set(p []byte)
	SetString(s string)
	String() string
}

// buildLogFuncChain analyzes the template and creates slices with the functions for execution and
// slices with the fixed parts of the template and the parameters
//
// fixParts contains the fixed parts of the template or parameters if a function is stored in the funcChain at this position
// funcChain contains for the parts which exist the functions for the dynamic parts
// funcChain and fixParts always have the same length and contain nil for the parts where no data is required in the chain,
// if a function exists for the part, a parameter for it can also exist in the fixParts slice
func buildLogFuncChain(cfg *options, tagFunctions map[string]LogFunc) ([][]byte, []LogFunc, error) {
	// process flow is copied from the fasttemplate flow https://github.com/valyala/fasttemplate/blob/2a2d1afadadf9715bfa19683cdaeac8347e5d9f9/template.go#L23-L62
	templateB := unsafeBytes(cfg.format)
	startTagB := unsafeBytes(startTag)
	endTagB := unsafeBytes(endTag)
	paramSeparatorB := unsafeBytes(paramSeparator)

	var fixParts [][]byte
	var funcChain []LogFunc

	for {
		currentPos := bytes.Index(templateB, startTagB)
		if currentPos < 0 {
			// no starting tag found in the existing template part
			break
		}
		// add fixed part
		funcChain = append(funcChain, nil)
		fixParts = append(fixParts, templateB[:currentPos])

		templateB = templateB[currentPos+len(startTagB):]
		currentPos = bytes.Index(templateB, endTagB)
		if currentPos < 0 {
			// cannot find end tag - just write it to the output.
			funcChain = append(funcChain, nil)
			fixParts = append(fixParts, startTagB)
			break
		}
		// ## function block ##
		// first check for tags with parameters
		if index := bytes.Index(templateB[:currentPos], paramSeparatorB); index != -1 {
			logFunc, ok := tagFunctions[unsafeString(templateB[:index+1])]
			if !ok {
				return nil, nil, errors.New("No parameter found in \"" + unsafeString(templateB[:currentPos]) + "\"")
			}
			funcChain = append(funcChain, logFunc)
			// add param to the fixParts
			fixParts = append(fixParts, templateB[index+1:currentPos])
		} else if logFunc, ok := tagFunctions[unsafeString(templateB[:currentPos])]; ok {
			// add functions without parameter
			funcChain = append(funcChain, logFunc)
			fixParts = append(fixParts, nil)
		}
		// ## function block end ##

		// reduce the template string
		templateB = templateB[currentPos+len(endTagB):]
	}
	// set the rest
	funcChain = append(funcChain, nil)
	fixParts = append(fixParts, templateB)

	return fixParts, funcChain, nil
}

const MaxStringLen = 0x7fff0000 // Maximum string length for UnsafeBytes. (decimal: 2147418112)

func unsafeBytes(s string) []byte {
	if s == "" {
		return nil
	}

	return (*[MaxStringLen]byte)(unsafe.Pointer(
		(*reflect.StringHeader)(unsafe.Pointer(&s)).Data),
	)[:len(s):len(s)]
}

func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}
