package typeutil

import (
	"reflect"
	"testing"
)

type sampleStruct struct{}

func namedFunctionForTest() {}

func TestParseTypeNameForStructAndPointer(t *testing.T) {
	if got := ParseTypeName(reflect.ValueOf(sampleStruct{})); got != "sampleStruct" {
		t.Fatalf("ParseTypeName(struct) = %q, want sampleStruct", got)
	}
	if got := ParseTypeName(reflect.ValueOf(&sampleStruct{})); got != "sampleStruct" {
		t.Fatalf("ParseTypeName(pointer) = %q, want sampleStruct", got)
	}
}

func TestParseTypeNameForNamedAndAnonymousFunc(t *testing.T) {
	if got := ParseTypeName(reflect.ValueOf(namedFunctionForTest)); got != "namedFunctionForTest" {
		t.Fatalf("ParseTypeName(named func) = %q, want namedFunctionForTest", got)
	}
	if got := ParseTypeName(reflect.ValueOf(func() {})); got != "" {
		t.Fatalf("ParseTypeName(anonymous func) = %q, want empty", got)
	}
}
