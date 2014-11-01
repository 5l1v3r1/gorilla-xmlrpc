// Copyright 2013 Ivan Danyliuk
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xml

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

// Types used for unmarshalling
type Response struct {
	Name   xml.Name    `xml:"methodResponse"`
	Params []Param     `xml:"params>param"`
	Fault  FaultValue  `xml:"fault,omitempty"`
}

type Param struct {
	Value Value `xml:"value"`
}

type Value struct {
	Array    []Value  `xml:"array>data>value"`
	Struct   []Member `xml:"struct>member"`
	String   string   `xml:"string"`
	Int      string   `xml:"int"`
	Int4     string   `xml:"i4"`
	Double   string   `xml:"double"`
	Boolean  string   `xml:"boolean"`
	DateTime string   `xml:"dateTime.iso8601"`
	Base64   string   `xml:"base64"`
	Raw      string   `xml:",innerxml"` // the value can be defualt string
}

type Member struct {
	Name  string `xml:"name"`
	Value Value  `xml:"value"`
}

func XML2RPC(xmlraw string, rpc interface{}) (err error) {
	// Unmarshal raw XML into the temporal structure
	var ret Response
	decoder := xml.NewDecoder(bytes.NewReader([]byte(xmlraw)))
	decoder.CharsetReader = charset.NewReader
	err = decoder.Decode(&ret)
	if err != nil {
		return
	}

	if !ret.Fault.IsEmpty() {
		fault, err := getFaultResponse(ret.Fault)
		if err != nil {
			return err
		}
		return fault
	}

	// Structures should have equal number of fields
	if reflect.TypeOf(rpc).Elem().NumField() != len(ret.Params) {
		return errors.New("Wrong number of arguments")
	}

	// Now, convert temporal structure into the
	// passed rpc variable, according to it's structure
	for i, param := range ret.Params {
		field := reflect.ValueOf(rpc).Elem().Field(i)
		err = Value2Field(param.Value, &field)
		if err != nil {
			return
		}
	}

	return
}

// getFaultResponse converts FaultValue to Fault.
func getFaultResponse(fault FaultValue) (Fault, error) {
	var (
		code int
		str string
		err error
	)

	for _, field := range fault.Value.Struct {
		if field.Name == "faultCode" {
			code, err = strconv.Atoi(field.Value.Int)
		} else if field.Name == "faultString" {
			str = field.Value.String
			if str == "" {
				str = field.Value.Raw
			}
		}
	}

	return Fault{Code: code, String: str}, err
}

func Value2Field(value Value, field *reflect.Value) error {
	if !field.CanSet() {
		return errors.New("Something wrong, unsettable rpc field/item passed")
	}

	var (
		err error
		val interface{}
	)

	switch {
	case value.Int != "":
		val, _ = strconv.Atoi(value.Int)
	case value.Int4 != "":
		val, _ = strconv.Atoi(value.Int4)
	case value.Double != "":
		val, _ = strconv.ParseFloat(value.Double, 64)
	case value.String != "":
		val = value.String
	case value.Boolean != "":
		val = XML2Bool(value.Boolean)
	case value.DateTime != "":
		val, err = XML2DateTime(value.DateTime)
	case value.Base64 != "":
		val, err = XML2Base64(value.Base64)
	case len(value.Struct) != 0:
		if field.Kind() != reflect.Struct {
			return fmt.Errorf("Structure fields mismatch: %s != %s", field.Kind(), reflect.Struct.String())
		}
		s := value.Struct
		for i := 0; i < len(s); i++ {
			// Uppercase first letter for field name to deal with
			// methods in lowercase, which cannot be used
			field_name := uppercaseFirst(s[i].Name)
			f := field.FieldByName(field_name)
			err = Value2Field(s[i].Value, &f)
		}
	case len(value.Array) != 0:
		a := value.Array
		f := *field
		slice := reflect.MakeSlice(reflect.TypeOf(f.Interface()),
			len(a), len(a))
		for i := 0; i < len(a); i++ {
			item := slice.Index(i)
			err = Value2Field(a[i], &item)
		}
		f = reflect.AppendSlice(f, slice)
		val = f.Interface()

	default:
		// value field is default to string, see http://en.wikipedia.org/wiki/XML-RPC#Data_types
		// also can be <nil/>
		if value.Raw != "<nil/>" {
			val = value.Raw
		}
	}

	if val != nil {
		if reflect.TypeOf(val) != reflect.TypeOf(field.Interface()) {
			return errors.New(fmt.Sprintf("Fields type mismatch: %s != %s",
				reflect.TypeOf(val),
				reflect.TypeOf(field.Interface())))
		}

		field.Set(reflect.ValueOf(val))
	}

	return err
}

func XML2Bool(value string) bool {
	var b bool
	switch value {
	case "1", "true", "TRUE", "True":
		b = true
	case "0", "false", "FALSE", "False":
		b = false
	}
	return b
}

func XML2DateTime(value string) (time.Time, error) {
	var (
		year, month, day     int
		hour, minute, second int
	)
	_, err := fmt.Sscanf(value, "%04d%02d%02dT%02d:%02d:%02d",
		&year, &month, &day,
		&hour, &minute, &second)
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
	return t, err
}

func XML2Base64(value string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(value)
}

func uppercaseFirst(in string) (out string) {
	r, n := utf8.DecodeRuneInString(in)
	return string(unicode.ToUpper(r)) + in[n:]
}
