package parser

import (
	"reflect"
	"testing"
)

func TestKMZParser_SupportedExtensions(t *testing.T) {
	p := NewKMZParser()
	exts := p.SupportedExtensions()
	expected := []string{".kmz"}
	if !reflect.DeepEqual(exts, expected) {
		t.Errorf("SupportedExtensions() = %v, want %v", exts, expected)
	}
}
