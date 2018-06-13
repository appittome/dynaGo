// Copyright 2016 Appittome. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dynaGo

import (
	"errors"
	"os"
	"reflect"
	"sync"

	"github.com/aws/aws-sdk-go/service/dynamodb"
)

// Marshal returns a dynamodb.PutItemInput representitive of i
// Any struct to be interpreted by this method must provide a
// Partition Key, marked by the field tag: "HASH", and may
// optionally select a Sort Key using the field tag "RANGE"
// Field tags are modeled after the encoding/json package as
// follows:  A field may have a different name as a dynamoDB
// attribute.  This name can be specified with the field tag
//   `dynaGo:"[alt-name]"`
// Any options in the field tag (such as HASH, or RANGE) must
// be specified after a comma. If the attribute name remains
// the same, then the tage must begin with a leading comma to
// indicate the presence of options:
//   `dynaGo:",HASH"`
//   `dynaGo:"[alt-name],HASH"
// for more examples see https://golang.org/pkg/encoding/json/
//
// Table names will simply be composed of the struct name plus
// the letter s.  For instance if there is a
//   type Packet struct {...}
// the associatedd dynamoDB table will be named "Packets" (for now?)
//
// Immediately this method only recognizes struct types that are
// composed of exculsively int, string, and structs or slices and
// pointers to any of those types. Any further unexpected type
// will trigger a panic. Additional types should be trivial to add
// following the given pattern.
func Marshal(i interface{}) *dynamodb.PutItemInput {
	e := &valueEncoderState{make(map[string]*dynamodb.AttributeValue)}
	encode(e, i)
	tn := TableName(reflect.TypeOf(i))
	return &dynamodb.PutItemInput{Item: e.item, TableName: &tn}
}

var (
	prefix string
	once   sync.Once
)

const (
	dynaGoPrefix = "DYNAGO_PREFIX"
)

func tablePrefix() string {
	once.Do(func() {
		// if the prefex isn't set, just have a tantrum
		if _, ok := os.LookupEnv(dynaGoPrefix); !ok {
			panic("env DNYAGO_PREFIX not set - no valid table prefix provided in environment")
		}
		//fetch the value in ENVIRONMENT - whatever that ended up being.
		prefix = os.Getenv(dynaGoPrefix) + "_"
	})
	return prefix
}

func TableName(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return tablePrefix() + t.Name() + "s"
}

// Try to create a table if it doesn't already exist
// If it does exist or cannot be created, return error
//
// Tables are created from structs only, and will panic on any other type
//
// Table name will be [structName] + s (ie type Doc struct {...} => table "Docs")
func CreateTable(svc *dynamodb.DynamoDB, v interface{}, w int64, r int64) error {
	tn := TableName(reflect.TypeOf(v))
	if err := tableExists(svc, tn); err != nil {
		return err
	}
	e := &tableEncoderState{
		keySchema:            make([]*dynamodb.KeySchemaElement, 0),
		attributeDefinitions: make([]*dynamodb.AttributeDefinition, 0),
	}
	encode(e, v)
	params := &dynamodb.CreateTableInput{
		TableName:            &tn,
		KeySchema:            e.keySchema,
		AttributeDefinitions: e.attributeDefinitions,
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  &r,
			WriteCapacityUnits: &w,
		},
	}
	if _, err := svc.CreateTable(params); err != nil {
		return err
	}
	return nil
}

type encoderState interface{}
type fieldTransform func(fs reflect.StructField, v reflect.Value) bool

// Concerned with encoding structs to 2 types:
// dynamoDB Tables, and dynamoDB Values by way of
// tableEncoderState and valueEncoderState respectively
func encode(e encoderState, i interface{}) {
	foundPKey := false
	v := reflect.ValueOf(i)
	t := v.Type()
	et := reflect.TypeOf(e)

	//allow one possible level of indirection
	if t.Kind() == reflect.Ptr {
		if v.IsNil() {
			panic(errors.New("Cannot encode nil ptr."))
		}
		t, v = t.Elem(), v.Elem()
	}

	if t.Kind() != reflect.Struct {
		panic(&OnlyStructsSupportedError{t.Kind()})
	}
	var ftr fieldTransform
	switch es := e.(type) {
	case *tableEncoderState:
		ftr = func(fs reflect.StructField, fv reflect.Value) bool {
			str := tableEncoder(fs.Type)(es, fs, fv)
			return str == dynamodb.KeyTypeHash
		}
	case *valueEncoderState:
		ftr = func(fs reflect.StructField, fv reflect.Value) bool {
			fn := getAttrName(fs)
			valueEncoder(fs.Type)(es, fn, fv)
			return true
		}
	default:
		panic(&InvalidEncoderStateType{et})
	}
	for n := 0; n < t.NumField(); n++ {
		fs, fv := t.Field(n), v.Field(n)
		// expect to find a primary key
		foundPKey = ftr(fs, fv) || foundPKey
	}
	if !foundPKey {
		panic(&MissingKeyError{t, dynamodb.KeyTypeHash})
	}
}

//-- UTIL --//
// could be cached
func tableExists(svc *dynamodb.DynamoDB, tn string) error {
	params := &dynamodb.ListTablesInput{}
	resp, err := svc.ListTables(params)
	if err != nil {
		return err
	}
	for _, name := range resp.TableNames {
		if *name == tn {
			return TableExistsError{tn}
		}
	}
	return nil
}

// The dynamoDB attribute name is determined by:
// if the field tags contains a name use that name
// if not, just use the native GoLang field name
// THIS METHOD PANICS IF the tags name the field
// "HASH", or "RANGE" as this is assumed to be a
// mistake (missing leading comma in field tag)
func getAttrName(s reflect.StructField) string {
	fn, _ := parseTag(s.Tag.Get("dynaGo"))
	if fn == dynamodb.KeyTypeHash || fn == dynamodb.KeyTypeRange {
		panic(&FieldNameCannotBeError{fn})
	}
	if fn == "" {
		fn = s.Name
	}
	return fn
}

// Determine if this field is a dynamoDB key
// if it is return the type from the below set
//   - dynamodb.KeyTypeHash
//   - dynamoDB.KeyTypeRange
// if it is not, return "" and an error
func getKeyType(s reflect.StructField, v reflect.Value) (string, error) {
	_, o := parseTag(s.Tag.Get("dynaGo"))
	if o.Contains(dynamodb.KeyTypeHash) {
		return dynamodb.KeyTypeHash, nil
	}
	if o.Contains(dynamodb.KeyTypeRange) {
		return dynamodb.KeyTypeRange, nil
	}
	return "", &KeyTypeNotFoundError{v.Type()}
}
